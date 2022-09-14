package consul

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/avast/retry-go"
	consulapi "github.com/hashicorp/consul/api"
	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/projects/gloo/constants"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	glooConsul "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/consul"
	"github.com/solo-io/gloo/projects/gloo/pkg/upstreams/consul"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/go-utils/errutils"
	"github.com/solo-io/go-utils/hashutils"
	"github.com/solo-io/k8s-utils/kubeutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"golang.org/x/sync/errgroup"
	"k8s.io/apimachinery/pkg/util/sets"
)

type epWatchTuple struct {
	// the last seen endpoints for this svc in this datacenter
	endpoints []*consulapi.CatalogService
	// a cancel function we can call when we no longer care about this watch (call before removal from map)
	cancel context.CancelFunc
}

// map of service name to endpoints tuple
type svcEndpointWatches map[string]*epWatchTuple

// map of datacenter to svcEndpointWatches
type dataCenterEndpointWatches map[string]svcEndpointWatches

type dataCenterServiceEndpointsTuple struct {
	dataCenter, service string
	endpoints           []*consulapi.CatalogService
}

// Starts a watch on the Consul service metadata endpoint for all the services associated with the tracked upstreams.
// Whenever it detects an update to said services, it fetches the complete specs for the tracked services,
// converts them to endpoints, and sends the result on the returned channel.
func (p *plugin) WatchEndpoints(writeNamespace string, upstreamsToTrack v1.UpstreamList, opts clients.WatchOpts) (<-chan v1.EndpointList, <-chan error, error) {

	// Filter out non-consul upstreams
	trackedServiceToUpstreams := make(map[string][]*v1.Upstream)
	for _, us := range upstreamsToTrack {
		if consulUsSpec := us.GetConsul(); consulUsSpec != nil {
			// discovery generates one upstream for every Consul service name;
			// this should only happen if users define duplicate upstreams for a consul service name.
			trackedServiceToUpstreams[consulUsSpec.GetServiceName()] = append(trackedServiceToUpstreams[consulUsSpec.GetServiceName()], us)
		}
	}

	dataCenters, err := p.client.DataCenters()
	if err != nil {
		return nil, nil, err
	}

	serviceMetaChan, servicesWatchErrChan := p.client.WatchServices(
		opts.Ctx,
		dataCenters,
		p.consulUpstreamDiscoverySettings.GetConsistencyMode(),
		p.consulUpstreamDiscoverySettings.GetQueryOptions(),
	)

	errChan := make(chan error)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		errutils.AggregateErrs(opts.Ctx, errChan, servicesWatchErrChan, "consul eds")
	}()

	allEndpointsListChan := make(chan v1.EndpointList)
	wg.Add(1)
	go func() {
		defer close(allEndpointsListChan)
		defer wg.Done()

		timer := time.NewTicker(p.dnsPollingInterval)
		defer timer.Stop()

		var previousSpecs []*consulapi.CatalogService
		var previousHash uint64

		publishEndpoints := func(endpoints v1.EndpointList) bool {
			if opts.Ctx.Err() != nil {
				return false
			}
			allEndpointsListChan <- endpoints
			return true
		}

		dcEndpointWatches := dataCenterEndpointWatches{}

		var (
			eg               errgroup.Group
			allEndpointsChan = make(chan *dataCenterServiceEndpointsTuple)
		)

		wg.Add(1)
		defer func() {
			_ = eg.Wait() // will never error
			close(allEndpointsChan)
			wg.Done() // delay closing errChan until all goroutines sending to it are done (eg.Wait() above)
		}()

		edsBlockingQueries := false // defaults to false because caching defaults to true; in testing I only saw cache hits when lastIndex was 0
		if bq := p.settings.GetConsulDiscovery().GetEdsBlockingQueries(); bq != nil {
			edsBlockingQueries = bq.GetValue()
		}

		logger := contextutils.LoggerFrom(opts.Ctx)

		for {
			select {
			case serviceMeta, ok := <-serviceMetaChan:
				if !ok {
					return
				}

				// non-blocking; more cache hits but more network calls if caching disabled (or cache misses per consul install settings)
				if !edsBlockingQueries {
					// the correctness of this implementation depending on updates on `serviceMetaChan` whenever a single catalog
					// service is updated is tested in "fires service watch even if catalog service is the only update"
					//
					// i.e., even an update to a single catalog service will cause a full refresh of all services
					specs := refreshSpecs(opts.Ctx, p.client, serviceMeta, errChan, trackedServiceToUpstreams)
					previousSpecs = specs

					// Build new endpoints from specs and publish if ctx is not cancelled
					endpoints := buildEndpointsFromSpecs(opts.Ctx, writeNamespace, p.resolver, specs, trackedServiceToUpstreams)
					currentHash := hashutils.MustHash(endpoints)
					if previousHash == currentHash {
						continue
					}
					previousHash = currentHash
					if !publishEndpoints(endpoints) {
						return
					}
					continue
				}

				// blocking; fewer network calls but fewer cache hits
				// in testing, I only saw cache hits here if last index was zero (first call during blocking)

				// construct a set of the present services by datacenter
				dcToCurrentSvcs := map[string]sets.String{}
				for _, meta := range serviceMeta {
					for _, dc := range meta.DataCenters {
						// add to set of datacenter/svc pairs present
						if _, ok := dcToCurrentSvcs[dc]; !ok {
							dcToCurrentSvcs[dc] = sets.NewString()
						}
						dcToCurrentSvcs[dc].Insert(meta.Name)

						// additionally, if not already a watch for this, create a watch
						if _, ok := dcEndpointWatches[dc]; !ok {
							dcEndpointWatches[dc] = svcEndpointWatches{}
						}
						if _, ok := dcEndpointWatches[dc][meta.Name]; ok {
							// watch already exists, don't recreate
							continue
						}
						// watch does not exist, create it
						ctx, newCancel := context.WithCancel(opts.Ctx)
						dcEndpointWatches[dc][meta.Name] = &epWatchTuple{
							endpoints: nil, // intentionally nil until we get the first update
							cancel:    newCancel,
						}

						// Copy before passing to goroutines!
						dcName := dc
						svcName := meta.Name

						endpointsChan, epErrChan := p.watchEndpointsInDataCenter(ctx, dcName, svcName, p.consulUpstreamDiscoverySettings.GetConsistencyMode(), p.consulUpstreamDiscoverySettings.GetQueryOptions())

						// Collect endpoints
						eg.Go(func() error {
							aggregateEndpoints(ctx, allEndpointsChan, endpointsChan)
							return nil
						})

						// Collect errors
						eg.Go(func() error {
							errutils.AggregateErrs(ctx, errChan, epErrChan, fmt.Sprintf("data center: %s, service: %s", dcName, svcName))
							return nil
						})
					}
				}

				// create a set of the dc / svc combos we will delete (do not delete from the map that we iterate over)
				dcToSvcsToCancel := map[string]sets.String{}
				for dc, svcWatchesMap := range dcEndpointWatches {
					for svcName := range svcWatchesMap {
						if _, ok := dcToSvcsToCancel[dc]; !ok {
							dcToSvcsToCancel[dc] = sets.NewString()
						}
						// if dc/svc combo is present in our watch map, but not in parsed current state of the world, cancel the watch
						if _, ok := dcToCurrentSvcs[dc]; !ok {
							// dc not current, we should delete the watch
							dcToSvcsToCancel[dc].Insert(svcName)
						} else if _, ok := dcToCurrentSvcs[dc][svcName]; !ok {
							// svc not current, we should delete
							dcToSvcsToCancel[dc].Insert(svcName)
						}
					}
				}

				// cancel the watches we need to cancel
				for dc, svcsMap := range dcToSvcsToCancel {
					for svc := range svcsMap {
						if _, ok := dcEndpointWatches[dc]; !ok {
							// developer logic error, skip to prevent panic
							logger.DPanicf("tried to cancel watch for endpoints in unknown data center: %s", dc)
							continue
						}
						if _, ok := dcEndpointWatches[dc][svc]; !ok {
							// developer logic error, skip to prevent panic
							logger.DPanicf("tried to cancel watch for endpoints for unknown service: %s", svc)
							continue
						}
						dcEndpointWatches[dc][svc].cancel()
						delete(dcEndpointWatches[dc], svc)
					}
				}

			case eps, ok := <-allEndpointsChan:
				if !ok {
					return
				}
				if _, ok := dcEndpointWatches[eps.dataCenter]; !ok {
					// developer logic error, skip to prevent panic
					logger.DPanicf("received endpoints in unknown data center: %s", eps.dataCenter)
					continue
				}
				if _, ok := dcEndpointWatches[eps.dataCenter][eps.service]; !ok {
					// developer logic error, skip to prevent panic
					logger.DPanicf("received endpoints for unknown service: %s", eps.dataCenter)
					continue
				}
				dcEndpointWatches[eps.dataCenter][eps.service].endpoints = eps.endpoints
				collector := newSpecCollector()
				for _, svcTuple := range dcEndpointWatches {
					for _, svc := range svcTuple {
						if svc.endpoints == nil {
							// no update received yet, skip
							continue
						}
						collector.Add(svc.endpoints)
					}
				}
				specs := collector.Get()
				previousSpecs = specs

				// Build new endpoints from specs and publish if ctx is not cancelled
				endpoints := buildEndpointsFromSpecs(opts.Ctx, writeNamespace, p.resolver, specs, trackedServiceToUpstreams)
				currentHash := hashutils.MustHash(endpoints)
				if previousHash == currentHash {
					continue
				}
				previousHash = currentHash
				if !publishEndpoints(endpoints) {
					return
				}

			case <-timer.C:
				// ensure we have at least one spec to check against; otherwise we risk marking EDS as ready
				// (by sending endpoints, even an empty list) too early
				if len(previousSpecs) == 0 {
					continue
				}

				// Poll to ensure any DNS updates get picked up in endpoints for EDS
				endpoints := buildEndpointsFromSpecs(opts.Ctx, writeNamespace, p.resolver, previousSpecs, trackedServiceToUpstreams)
				currentHash := hashutils.MustHash(endpoints)
				if previousHash == currentHash {
					continue
				}
				previousHash = currentHash
				if !publishEndpoints(endpoints) {
					return
				}
			case <-opts.Ctx.Done():
				return
			}
		}
	}()

	go func() {
		wg.Wait()
		close(errChan)
	}()
	return allEndpointsListChan, errChan, nil
}

// Honors the contract of Watch functions to open with an initial read.
func (p *plugin) watchEndpointsInDataCenter(ctx context.Context, dataCenter, svcName string, cm glooConsul.ConsulConsistencyModes, queryOpts *glooConsul.QueryOptions) (<-chan *dataCenterServiceEndpointsTuple, <-chan error) {
	endpointsChan := make(chan *dataCenterServiceEndpointsTuple)
	errsChan := make(chan error)

	go func(dataCenter string) {
		defer close(endpointsChan)
		defer close(errsChan)

		lastIndex := uint64(0)

		for {
			select {
			case <-ctx.Done():
				return
			default:

				var (
					endpoints []*consulapi.CatalogService
					queryMeta *consulapi.QueryMeta
				)

				// This is a blocking query (see [here](https://www.consul.io/api/features/blocking.html) for more info)
				// The first invocation (with lastIndex equal to zero) will return immediately
				queryOpts := consul.NewConsulCatalogServiceQueryOptions(dataCenter, cm, queryOpts)
				queryOpts.WaitIndex = lastIndex

				ctxDead := false

				// Use a back-off retry strategy to avoid flooding the error channel
				err := retry.Do(
					func() error {
						var err error
						if ctx.Err() != nil {
							// intentionally return early if context is already done
							// this is a backoff loop; by the time we get here ctx may be done
							ctxDead = true
							return nil
						}

						endpoints, queryMeta, err = p.client.Service(svcName, "", queryOpts.WithContext(ctx))
						return err
					},
					retry.Attempts(6),
					//  Last delay is 2^6 * 100ms = 3.2s
					retry.Delay(100*time.Millisecond),
					retry.DelayType(retry.BackOffDelay),
				)

				if ctxDead {
					return
				}

				if err != nil {
					errsChan <- err
					continue
				}

				// If index is the same, there have been no changes since last query
				if queryMeta.LastIndex == lastIndex {
					continue
				}

				tuple := &dataCenterServiceEndpointsTuple{
					dataCenter: dataCenter,
					service:    svcName,
					endpoints:  endpoints,
				}

				// Update the last index
				if queryMeta.LastIndex < lastIndex {
					// update if index goes backwards per consul blocking query docs
					// this can happen e.g. KV list operations where item with highest index is deleted
					// for more, see https://www.consul.io/api-docs/features/blocking#implementation-details
					lastIndex = 0
				} else {
					lastIndex = queryMeta.LastIndex
				}

				endpointsChan <- tuple
			}
		}
	}(dataCenter)

	return endpointsChan, errsChan
}

func aggregateEndpoints(ctx context.Context, dest chan *dataCenterServiceEndpointsTuple, src <-chan *dataCenterServiceEndpointsTuple) {
	for {
		select {
		case services, ok := <-src:
			if !ok {
				return
			}
			select {
			case <-ctx.Done():
				return
			case dest <- services:
			}
		case <-ctx.Done():
			return
		}
	}
}

// For each service AND data center combination, return a CatalogService that contains a list of all service instances
// belonging to that service within that datacenter.
func refreshSpecs(ctx context.Context, client consul.ConsulWatcher, serviceMeta []*consul.ServiceMeta, errChan chan error, serviceToUpstream map[string][]*v1.Upstream) []*consulapi.CatalogService {
	logger := contextutils.LoggerFrom(contextutils.WithLogger(ctx, "consul_eds"))
	specs := newThreadSafeSpecCollector()

	// Get complete service information for every dataCenter:service tuple in separate goroutines
	var eg errgroup.Group
	for _, service := range serviceMeta {
		var cm glooConsul.ConsulConsistencyModes
		var queryOptions *glooConsul.QueryOptions
		if upstreams, ok := serviceToUpstream[service.Name]; len(upstreams) > 0 && ok {
			cm = upstreams[0].GetConsul().GetConsistencyMode()
			queryOptions = upstreams[0].GetConsul().GetQueryOptions()
		}
		// we take the most consistent mode found on any upstream for a service for correctness
		for _, consulUpstream := range serviceToUpstream[service.Name] {
			// prefer earlier more restrictive query type (i.e. consistent > default > stale)
			switch consulUpstream.GetConsul().GetConsistencyMode() {
			case glooConsul.ConsulConsistencyModes_ConsistentMode:
				cm = glooConsul.ConsulConsistencyModes_ConsistentMode
			case glooConsul.ConsulConsistencyModes_DefaultMode:
				if cm != glooConsul.ConsulConsistencyModes_ConsistentMode {
					cm = glooConsul.ConsulConsistencyModes_DefaultMode
				}
			case glooConsul.ConsulConsistencyModes_StaleMode:
				if cm != glooConsul.ConsulConsistencyModes_ConsistentMode && cm != glooConsul.ConsulConsistencyModes_DefaultMode {
					cm = glooConsul.ConsulConsistencyModes_StaleMode
				}
			}
			if queryOptions := consulUpstream.GetConsul().GetQueryOptions(); queryOptions != nil {
				// if any upstream can't use cache, disable for all
				if useCache := queryOptions.GetUseCache(); useCache != nil && !useCache.GetValue() {
					queryOptions.UseCache = useCache
				}
			}
		}
		for _, dataCenter := range service.DataCenters {
			// Copy iterator variables before passing them to goroutines!
			svc := service
			dcName := dataCenter

			// Get complete spec for each service in parallel
			eg.Go(func() error {
				queryOpts := consul.NewConsulCatalogServiceQueryOptions(dcName, cm, queryOptions)
				if ctx.Err() != nil {
					// intentionally return early if context is already done
					// we create a lot of requests; by the time we get here ctx may be done
					return ctx.Err()
				}
				services, _, err := client.Service(svc.Name, "", queryOpts.WithContext(ctx))
				if err != nil {
					return err
				}
				specs.Add(services)

				return nil
			})
		}
	}

	// Wait for all requests to complete, an error to occur, or for the underlying context to be cancelled.
	//
	// Don't return if an error occurred. We still want to propagate the endpoints for the requests that
	// succeeded. Any inconsistencies will be caught by the Gloo translator.
	if err := eg.Wait(); err != nil {
		select {
		case errChan <- err:
		default:
			logger.Errorf("write error channel is full! could not propagate err: %v", err)
		}
	}
	return specs.Get()
}

// build gloo endpoints out of consul catalog services and gloo upstreams
// trackedServiceToUpstreams is a map from consul service names to a list of gloo upstreams associated with it.
// Each spec is a grouping of serviceInstances (aka endpoints) associated with a single consul service on one datacenter.
// This means that for each consul service/datacenter combo, we produce as many endpoints for each associated IP we find
// using getIpAddresses(), each of which will be labeled to reflect which of its tags/datacenters are associated with that endpoint.
// This awkward labeling is needed because our constructed endpoints are made on a per datacenter basis, but gloo
// upstreams are not divided this way, so we have to divide them ourselves with metadata.
func buildEndpointsFromSpecs(
	ctx context.Context,
	writeNamespace string,
	resolver DnsResolver,
	specs []*consulapi.CatalogService,
	trackedServiceToUpstreams map[string][]*v1.Upstream,
) v1.EndpointList {
	var endpoints v1.EndpointList
	for _, spec := range specs {
		if upstreams, ok := trackedServiceToUpstreams[spec.ServiceName]; ok {
			if eps, err := buildEndpoints(ctx, writeNamespace, resolver, spec, upstreams); err != nil {
				contextutils.LoggerFrom(ctx).Warnf("consul eds plugin encountered error resolving DNS for consul service %v", spec, err)
			} else {
				endpoints = append(endpoints, eps...)
			}
		}
	}

	// Sort by name in ascending order for idempotency
	sort.SliceStable(endpoints, func(i, j int) bool {
		return endpoints[i].GetMetadata().GetName() < endpoints[j].GetMetadata().GetName()
	})
	return endpoints
}

// The ServiceTags on the Consul Upstream(s) represent all tags for Consul services with the given ServiceName across
// data centers. We create an endpoint label for each tag from the gloo upstreams,
// where the label key is the name of the tag and the label value is "1" if the current service contains the same tag,
// and "0" otherwise.
func BuildTagMetadata(tags []string, upstreams []*v1.Upstream) map[string]string {

	// Build maps for quick lookup
	svcTags := make(map[string]bool)
	for _, tag := range tags {
		svcTags[tag] = true
	}

	labels := make(map[string]string)
	for _, usTag := range getUniqueUpstreamTags(upstreams) {

		// Prepend prefix
		tagKey := constants.ConsulTagKeyPrefix + usTag

		if _, ok := svcTags[usTag]; ok {
			labels[tagKey] = constants.ConsulEndpointMetadataMatchTrue
		} else {
			labels[tagKey] = constants.ConsulEndpointMetadataMatchFalse
		}
	}

	return labels
}

// Similarly to what we do with tags, create a label for each data center and set it to "1" if the service instance
// is running in that data center.
func BuildDataCenterMetadata(dataCenters []string, upstreams []*v1.Upstream) map[string]string {

	// Build maps for quick lookup
	svcDataCenters := make(map[string]bool)
	for _, dc := range dataCenters {
		svcDataCenters[dc] = true
	}

	labels := make(map[string]string)
	for _, dc := range getUniqueUpstreamDataCenters(upstreams) {

		// Prepend prefix
		dcKey := constants.ConsulDataCenterKeyPrefix + dc

		if _, ok := svcDataCenters[dc]; ok {
			labels[dcKey] = constants.ConsulEndpointMetadataMatchTrue
		} else {
			labels[dcKey] = constants.ConsulEndpointMetadataMatchFalse
		}
	}
	return labels
}

// Construct gloo endpoints for one consul CatalogService and a bunch of gloo upstreams.
// Produces 1 endpoint for each ip address discovered by the DNS resolver.
// Each resulting endpoint has labels representing all tags/datacenters on the upstreams, with flags for
// which of those is or isn't in the current catalog service (see BuildTagMetadata function)
func buildEndpoints(
	ctx context.Context,
	namespace string,
	resolver DnsResolver,
	service *consulapi.CatalogService,
	upstreams []*v1.Upstream,
) ([]*v1.Endpoint, error) {

	// Address is the IP address of the Consul node on which the service is registered.
	// ServiceAddress is the IP address of the service host — if empty, node address should be used
	address := service.ServiceAddress
	if address == "" {
		address = service.Address
	}

	ipAddresses, err := getIpAddresses(ctx, address, resolver)
	if err != nil {
		return nil, err
	}

	var endpoints []*v1.Endpoint
	for _, ipAddr := range ipAddresses {
		endpoints = append(endpoints, buildEndpoint(namespace, address, ipAddr, service, upstreams))
	}
	return endpoints, nil
}

// only returns an error if the consul service address is a hostname and we can't resolve it
func getIpAddresses(ctx context.Context, address string, resolver DnsResolver) ([]string, error) {
	addr := net.ParseIP(address)
	if addr != nil {
		// the consul service address is an IP address, no need to resolve it!
		return []string{address}, nil
	}

	// we're assuming the consul service returned a hostname instead of an IP
	// we need to resolve this here so EDS can be given IPs (EDS can't resolve hostnames)
	if resolver == nil {
		return nil, eris.Errorf("Consul service returned an address that couldn't be parsed as an IP (%s), "+
			"would have resolved as a hostname but the configured Consul DNS resolver was nil", address)
	}
	ipAddrs, err := resolver.Resolve(ctx, address)
	if err != nil {
		return nil, err
	}

	var ipAddresses []string
	for _, ipAddr := range ipAddrs {
		ipAddresses = append(ipAddresses, ipAddr.String())
	}
	return ipAddresses, nil
}

func buildEndpoint(namespace, address, ipAddress string, service *consulapi.CatalogService, upstreams []*v1.Upstream) *v1.Endpoint {
	hostname := ""
	var healthCheckConfig *v1.HealthCheckConfig
	if address != ipAddress {
		// we don't want to override the hostname if we didn't resolve the address
		hostname = address
		healthCheckConfig = &v1.HealthCheckConfig{
			Hostname: hostname,
		}
	}
	return &v1.Endpoint{
		Metadata: &core.Metadata{
			Namespace:       namespace,
			Name:            buildEndpointName(ipAddress, service),
			Labels:          buildLabels(service.ServiceTags, []string{service.Datacenter}, upstreams),
			ResourceVersion: strconv.FormatUint(service.ModifyIndex, 10),
		},
		Upstreams:   toResourceRefs(upstreams, service.ServiceTags),
		Address:     ipAddress,
		Port:        uint32(service.ServicePort),
		Hostname:    hostname,
		HealthCheck: healthCheckConfig,
	}
}

func buildEndpointName(address string, service *consulapi.CatalogService) string {
	parts := []string{address, service.ServiceName}
	if service.ServiceID != "" {
		parts = append(parts, service.ServiceID, strconv.Itoa(service.ServicePort))
	}
	unsanitizedName := strings.Join(parts, "-")
	unsanitizedName = strings.ReplaceAll(unsanitizedName, "_", "")
	return kubeutils.SanitizeNameV2(unsanitizedName)
}

// The labels will be used to match the endpoint to the subsets of the cluster represented by the upstream.
// This is a union of BuildTagMetadata and BuildDataCenterMetadata,
// which means that the labels map contains all of the upstreams' tags and datacenters as keys,
// with a 1 as the value if that tag/datacenter in the associated catalogService, and a 0 otherwise.
// Both the tags and dataCenters inputs come from the catalog service.
func buildLabels(tags, dataCenters []string, upstreams []*v1.Upstream) map[string]string {
	labels := BuildTagMetadata(tags, upstreams)
	for dcLabelKey, dcLabelValue := range BuildDataCenterMetadata(dataCenters, upstreams) {
		labels[dcLabelKey] = dcLabelValue
	}
	return labels
}

// endpointTags come from a consul catalog service
func toResourceRefs(upstreams []*v1.Upstream, endpointTags []string) (out []*core.ResourceRef) {
	for _, us := range upstreams {
		upstreamTags := us.GetConsul().GetInstanceTags()
		if shouldAddToUpstream(endpointTags, upstreamTags) {
			out = append(out, us.GetMetadata().Ref())
		}
	}
	return
}

// are there no upstream tags = return true.
// Otherwise, check if upstream tags is a subset of endpoint tags
func shouldAddToUpstream(endpointTags, upstreamTags []string) bool {
	if len(upstreamTags) == 0 {
		return true
	}

	containsTag := func(tag string) bool {
		for _, etag := range endpointTags {
			if tag == etag {
				return true
			}
		}
		return false
	}

	// check if upstream tags is a subset of endpoint tags
	for _, tag := range upstreamTags {
		if !containsTag(tag) {
			return false
		}
	}

	return true
}

func getUniqueUpstreamTags(upstreams []*v1.Upstream) (tags []string) {
	tagMap := make(map[string]bool)
	for _, us := range upstreams {
		if len(us.GetConsul().GetSubsetTags()) != 0 {
			for _, tag := range us.GetConsul().GetSubsetTags() {
				tagMap[tag] = true
			}
		} else {
			for _, tag := range us.GetConsul().GetServiceTags() {
				tagMap[tag] = true
			}
		}
	}
	for tag := range tagMap {
		tags = append(tags, tag)
	}
	return
}

func getUniqueUpstreamDataCenters(upstreams []*v1.Upstream) (dataCenters []string) {
	dcMap := make(map[string]bool)
	for _, us := range upstreams {
		for _, dc := range us.GetConsul().GetDataCenters() {
			dcMap[dc] = true
		}
	}
	for dc := range dcMap {
		dataCenters = append(dataCenters, dc)
	}
	return
}

type specCollector interface {
	Add([]*consulapi.CatalogService)
	Get() []*consulapi.CatalogService
}

func newThreadSafeSpecCollector() specCollector {
	return &threadSafeCollector{}
}

type threadSafeCollector struct {
	mutex sync.RWMutex
	specs []*consulapi.CatalogService
}

func (c *threadSafeCollector) Add(specs []*consulapi.CatalogService) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.specs = append(c.specs, specs...)
}

func (c *threadSafeCollector) Get() []*consulapi.CatalogService {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.specs
}

func newSpecCollector() specCollector {
	return &collector{}
}

type collector struct {
	specs []*consulapi.CatalogService
}

func (c *collector) Add(specs []*consulapi.CatalogService) {
	c.specs = append(c.specs, specs...)
}

func (c *collector) Get() []*consulapi.CatalogService {
	return c.specs
}

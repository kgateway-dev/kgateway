package consul

import (
	"context"
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
)

type svcNameToEndpoints struct {
	svcNameToSpecs map[string][]*consulapi.CatalogService
}

type endpointsTuple struct {
	dcToSvcTuple map[string]*svcNameToEndpoints
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
		p.consulUpstreamDiscoverySettings.GetServiceFilter().GetValue(),
		p.consulUpstreamDiscoverySettings.GetConsistencyMode(),
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

		timer := time.NewTicker(DefaultDnsPollingInterval)
		defer timer.Stop()

		var previousSpecs []*consulapi.CatalogService
		var previousHash uint64

		publishEndpoints := func(endpoints v1.EndpointList) bool {
			if opts.Ctx.Err() != nil {
				return false
			}
			select {
			case <-opts.Ctx.Done():
				return false
			case allEndpointsListChan <- endpoints:
			}
			return true
		}

		epsTuple := &endpointsTuple{
			dcToSvcTuple: make(map[string]*svcNameToEndpoints),
		}

		var (
			eg               errgroup.Group
			allEndpointsChan = make(chan []*consulapi.CatalogService)
		)

		defer close(allEndpointsChan)

		// Create a new context for each loop, cancel it before each loop
		var cancel context.CancelFunc = func() {}
		// Use closure to allow cancel function to be updated as context changes
		defer func() { cancel() }()

		for {
			select {

			case serviceMeta, ok := <-serviceMetaChan:
				if !ok {
					return
				}

				// Cancel any running requests from previous iteration and set new context/cancel
				cancel()
				ctx, newCancel := context.WithCancel(opts.Ctx)
				cancel = newCancel

				epsTuple = &endpointsTuple{
					dcToSvcTuple: make(map[string]*svcNameToEndpoints),
				}

				for _, meta := range serviceMeta {
					for _, dc := range meta.DataCenters {
						// Copy before passing to goroutines!
						dcName := dc
						svcName := meta.Name

						endpointsChan, epErrChan := p.watchEndpointsInDataCenter(ctx, dcName, svcName)

						// Collect endpoints
						eg.Go(func() error {
							aggregateEndpoints(ctx, allEndpointsChan, endpointsChan)
							return nil
						})

						// Collect errors
						eg.Go(func() error {
							errutils.AggregateErrs(ctx, errChan, epErrChan, "data center: "+dcName)
							return nil
						})
					}
				}

			case eps, ok := <-allEndpointsChan:
				if !ok {
					return
				}

				// add ep to our persisted struct
				for _, ep := range eps {
					if epsTuple.dcToSvcTuple == nil {
						epsTuple.dcToSvcTuple = make(map[string]*svcNameToEndpoints)
					}
					if epsTuple.dcToSvcTuple[ep.Datacenter] == nil {
						epsTuple.dcToSvcTuple[ep.Datacenter] = &svcNameToEndpoints{}
					}
					if epsTuple.dcToSvcTuple[ep.Datacenter].svcNameToSpecs == nil {
						epsTuple.dcToSvcTuple[ep.Datacenter].svcNameToSpecs = make(map[string][]*consulapi.CatalogService)
					}
					if epsTuple.dcToSvcTuple[ep.Datacenter].svcNameToSpecs[ep.ServiceName] == nil {
						epsTuple.dcToSvcTuple[ep.Datacenter].svcNameToSpecs[ep.ServiceName] = []*consulapi.CatalogService{}
					}
					// TODO(kdorosh) reason about removing stale eps... don't just always append
					epsTuple.dcToSvcTuple[ep.Datacenter].svcNameToSpecs[ep.ServiceName] = append(epsTuple.dcToSvcTuple[ep.Datacenter].svcNameToSpecs[ep.ServiceName], ep)
				}

				collector := newSpecCollector()
				for _, svcTuple := range epsTuple.dcToSvcTuple {
					for _, svc := range svcTuple.svcNameToSpecs {
						collector.Add(svc)
					}
				}
				specs := collector.Get() // TODO(kdorosh) this does not need to be synchronized anymore!

				endpoints := buildEndpointsFromSpecs(opts.Ctx, writeNamespace, p.resolver, specs, trackedServiceToUpstreams)
				previousHash = hashutils.MustHash(endpoints)
				previousSpecs = specs

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
func (p *plugin) watchEndpointsInDataCenter(ctx context.Context, dataCenter, svcName string) (<-chan []*consulapi.CatalogService, <-chan error) {
	endpointsChan := make(chan []*consulapi.CatalogService)
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
				queryOpts := consul.NewConsulCatalogServiceQueryOptions(dataCenter, glooConsul.ConsulConsistencyModes_ConsistentMode) // TODO(kdorosh) make configurable
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
						// TODO(kdorosh) provide tag?
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

				// Update the last index
				if queryMeta.LastIndex < lastIndex {
					// update if index goes backwards per consul blocking query docs
					// this can happen e.g. KV list operations where item with highest index is deleted
					// for more, see https://www.consul.io/api-docs/features/blocking#implementation-details
					lastIndex = 0
				} else {
					lastIndex = queryMeta.LastIndex
				}
				endpointsChan <- endpoints
			}
		}
	}(dataCenter)

	return endpointsChan, errsChan
}

// For each service AND data center combination, return a CatalogService that contains a list of all service instances
// belonging to that service within that datacenter.
func refreshSpecs(ctx context.Context, client consul.ConsulWatcher, serviceMeta []*consul.ServiceMeta, errChan chan error, serviceToUpstream map[string][]*v1.Upstream) []*consulapi.CatalogService {
	logger := contextutils.LoggerFrom(contextutils.WithLogger(ctx, "consul_eds"))
	specs := newSpecCollector()

	// Get complete service information for every dataCenter:service tuple in separate goroutines
	var eg errgroup.Group
	for _, service := range serviceMeta {
		var cm glooConsul.ConsulConsistencyModes
		if upstreams, ok := serviceToUpstream[service.Name]; len(upstreams) > 0 && ok {
			cm = upstreams[0].GetConsul().GetConsistencyMode()
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
		}
		for _, dataCenter := range service.DataCenters {
			// Copy iterator variables before passing them to goroutines!
			svc := service
			dcName := dataCenter

			// Get complete spec for each service in parallel
			eg.Go(func() error {
				queryOpts := consul.NewConsulCatalogServiceQueryOptions(dcName, cm)
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

func newSpecCollector() specCollector {
	return &collector{}
}

type specCollector interface {
	Add([]*consulapi.CatalogService)
	Get() []*consulapi.CatalogService
}

type collector struct {
	mutex sync.RWMutex
	specs []*consulapi.CatalogService
}

func (c *collector) Add(specs []*consulapi.CatalogService) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.specs = append(c.specs, specs...)
}

func (c *collector) Get() []*consulapi.CatalogService {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.specs
}

func aggregateEndpoints(ctx context.Context, dest chan []*consulapi.CatalogService, src <-chan []*consulapi.CatalogService) {
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

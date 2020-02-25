package consul

import (
	"context"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/solo-io/gloo/projects/gloo/pkg/upstreams/consul"

	"github.com/rotisserie/eris"
	"github.com/solo-io/go-utils/hashutils"

	"github.com/solo-io/go-utils/kubeutils"

	"github.com/solo-io/gloo/projects/gloo/constants"

	"github.com/solo-io/go-utils/contextutils"

	"github.com/solo-io/gloo/pkg/utils"

	consulapi "github.com/hashicorp/consul/api"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/errutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"golang.org/x/sync/errgroup"
)

// Starts a watch on the Consul service metadata endpoint for all the services associated with the tracked upstreams.
// Whenever it detects an update to said services, it fetches the complete specs for the tracked services,
// converts them to endpoints, and sends the result on the returned channel.
func (p *plugin) WatchEndpoints(writeNamespace string, upstreamsToTrack v1.UpstreamList, opts clients.WatchOpts) (<-chan v1.EndpointList, <-chan error, error) {
	endpointsChan := make(chan v1.EndpointList)
	errChan := make(chan error)

	// Filter out non-consul upstreams
	trackedServiceToUpstreams := make(map[string][]*v1.Upstream)
	var previousSpecs []*consulapi.CatalogService
	var previousHash uint64
	for _, us := range upstreamsToTrack {
		if consulUsSpec := us.GetConsul(); consulUsSpec != nil {
			// We generate one upstream for every Consul service name, so this should never happen.
			trackedServiceToUpstreams[consulUsSpec.ServiceName] = append(trackedServiceToUpstreams[consulUsSpec.ServiceName], us)
		}
	}

	dataCenters, err := p.client.DataCenters()
	if err != nil {
		return nil, nil, err
	}

	serviceMetaChan, servicesWatchErrChan := p.client.WatchServices(opts.Ctx, dataCenters)

	var errAggregator sync.WaitGroup
	errAggregator.Add(1)
	go func() {
		defer errAggregator.Done()
		errutils.AggregateErrs(opts.Ctx, errChan, servicesWatchErrChan, "consul eds")
	}()

	go func() {

		// Create a new context for each loop, cancel it before each loop
		var cancel context.CancelFunc = func() {}
		// Use closure to allow cancel function to be updated as context changes
		defer func() { cancel() }()

		timer := time.NewTicker(DefaultDnsPollingInterval)

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

				specs := refreshSpecs(ctx, p.client, serviceMeta, errChan)
				endpoints := buildEndpointsFromSpecs(opts.Ctx, writeNamespace, p.resolver, specs, trackedServiceToUpstreams)

				previousHash = hashutils.MustHash(endpoints)
				previousSpecs = specs

				endpointsChan <- endpoints

			case <-timer.C:
				// Poll to ensure any DNS updates get picked up in endpoints for EDS
				endpoints := buildEndpointsFromSpecs(opts.Ctx, writeNamespace, p.resolver, previousSpecs, trackedServiceToUpstreams)

				currentHash := hashutils.MustHash(endpoints)
				if previousHash == currentHash {
					continue
				}

				previousHash = currentHash
				endpointsChan <- endpoints

			case <-opts.Ctx.Done():
				close(endpointsChan)

				// Wait for error aggregation routing to complete to avoid writing to closed errChan
				errAggregator.Wait()
				close(errChan)
				return

			}
		}
	}()

	return endpointsChan, errChan, nil
}

func refreshSpecs(ctx context.Context, client consul.ConsulWatcher, serviceMeta []*consul.ServiceMeta, errChan chan error) []*consulapi.CatalogService {
	logger := contextutils.LoggerFrom(contextutils.WithLogger(ctx, "consul_eds"))

	specs := newSpecCollector()

	// Get complete service information for every dataCenter:service tuple in separate goroutines
	var eg errgroup.Group
	for _, service := range serviceMeta {
		for _, dataCenter := range service.DataCenters {

			// Copy iterator variables before passing them to goroutines!
			svc := service
			dcName := dataCenter

			// Get complete spec for each service in parallel
			eg.Go(func() error {
				queryOpts := &consulapi.QueryOptions{Datacenter: dcName, RequireConsistent: true}

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

func buildEndpointsFromSpecs(ctx context.Context, writeNamespace string, resolver DnsResolver, specs []*consulapi.CatalogService, trackedServiceToUpstreams map[string][]*v1.Upstream) v1.EndpointList {
	var endpoints v1.EndpointList
	for _, spec := range specs {
		if upstreams, ok := trackedServiceToUpstreams[spec.ServiceName]; ok {
			if eps, err := buildEndpoints(writeNamespace, resolver, spec, upstreams); err != nil {
				contextutils.LoggerFrom(ctx).Warnf("consul eds plugin encountered error resolving DNS for consul service %v", spec, err)
			} else {
				endpoints = append(endpoints, eps...)
			}
		}
	}

	// Sort by name in ascending order for idempotency
	sort.SliceStable(endpoints, func(i, j int) bool {
		return endpoints[i].Metadata.Name < endpoints[j].Metadata.Name
	})
	return endpoints
}

// The ServiceTags on the Consul Upstream(s) represent all tags for Consul services with the given ServiceName across
// data centers. We create an endpoint label for each of these tags, where the label key is the name of the tag and
// the label value is "1" if the current service contains the same tag, else "0".
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

func buildEndpoints(namespace string, resolver DnsResolver, service *consulapi.CatalogService, upstreams []*v1.Upstream) ([]*v1.Endpoint, error) {

	// Address is the IP address of the Consul node on which the service is registered.
	// ServiceAddress is the IP address of the service host — if empty, node address should be used
	address := service.ServiceAddress
	if address == "" {
		address = service.Address
	}

	ipAddresses, err := getIpAddresses(address, resolver)
	if err != nil {
		return nil, err
	}

	var endpoints []*v1.Endpoint
	for _, ipAddr := range ipAddresses {
		endpoints = append(endpoints, buildEndpoint(namespace, ipAddr, service, upstreams))
	}
	return endpoints, nil
}

// only returns an error if the consul service address is a hostname and we can't resolve it
func getIpAddresses(address string, resolver DnsResolver) ([]string, error) {
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

	ipAddrs, err := resolver.Resolve(address)
	if err != nil {
		return nil, err
	}

	var ipAddresses []string
	for _, ipAddr := range ipAddrs {
		ipAddresses = append(ipAddresses, ipAddr.String())
	}
	return ipAddresses, nil
}

func buildEndpoint(namespace, address string, service *consulapi.CatalogService, upstreams []*v1.Upstream) *v1.Endpoint {
	return &v1.Endpoint{
		Metadata: core.Metadata{
			Namespace:       namespace,
			Name:            buildEndpointName(service),
			Labels:          buildLabels(service.ServiceTags, []string{service.Datacenter}, upstreams),
			ResourceVersion: strconv.FormatUint(service.ModifyIndex, 10),
		},
		Upstreams: toResourceRefs(upstreams),
		Address:   address,
		Port:      uint32(service.ServicePort),
	}
}

func buildEndpointName(service *consulapi.CatalogService) string {
	parts := []string{service.ServiceName}
	if service.ServiceID != "" {
		parts = append(parts, service.ServiceID)
	}
	unsanitizedName := strings.Join(parts, "-")
	unsanitizedName = strings.ReplaceAll(unsanitizedName, "_", "")
	return kubeutils.SanitizeNameV2(unsanitizedName)
}

// The labels will be used by to match the endpoint to the subsets of the cluster represented by the upstream.
func buildLabels(tags, dataCenters []string, upstreams []*v1.Upstream) map[string]string {
	labels := BuildTagMetadata(tags, upstreams)
	for dcLabelKey, dcLabelValue := range BuildDataCenterMetadata(dataCenters, upstreams) {
		labels[dcLabelKey] = dcLabelValue
	}
	return labels
}

func toResourceRefs(upstreams []*v1.Upstream) (out []*core.ResourceRef) {
	for _, us := range upstreams {
		out = append(out, utils.ResourceRefPtr(us.Metadata.Ref()))
	}
	return
}

func getUniqueUpstreamTags(upstreams []*v1.Upstream) (tags []string) {
	tagMap := make(map[string]bool)
	for _, us := range upstreams {
		for _, tag := range us.GetConsul().ServiceTags {
			tagMap[tag] = true
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
		for _, dc := range us.GetConsul().DataCenters {
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
	mutex sync.Mutex
	specs servicesByNode
}

type servicesByNode map[string]servicesByServiceId

type servicesByServiceId map[string]*consulapi.CatalogService

// implementation of a Set
func (c *collector) Add(specs []*consulapi.CatalogService) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if c.specs == nil {
		c.specs = make(servicesByNode)
	}
	for _, spec := range specs {
		if c.specs[spec.Node] == nil {
			c.specs[spec.Node] = make(servicesByServiceId)
		}
		// Consul enforces that service IDs must be unique per node
		c.specs[spec.Node][spec.ServiceID] = spec
	}
}

// implementation of a Set
func (c *collector) Get() []*consulapi.CatalogService {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	var services []*consulapi.CatalogService
	for _, nodes := range c.specs {
		for _, svc := range nodes {
			services = append(services, svc)
		}
	}
	return services
}

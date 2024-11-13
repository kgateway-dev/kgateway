package consul

import (
	consulapi "github.com/hashicorp/consul/api"
	"github.com/rotisserie/eris"
	glooConsul "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/consul"
)

//go:generate mockgen -destination=./mocks/mock_consul_client.go -source consul_client.go

var ForbiddenDataCenterErr = func(dataCenter string) error {
	return eris.Errorf("not allowed to query data center [%s]. "+
		"Use the settings to configure the data centers Gloo is allowed to query", dataCenter)
}

// TODO(marco): consider adding ctx to signatures instead on relying on caller to set it
// Wrap the Consul API in an interface to allow mocking
type ClientWrapper interface {
	// DataCenters is used to query for all the known data centers.
	// Results will be filtered based on the data center whitelist provided in the Gloo settings.
	DataCenters() ([]string, error)
	// Services is used to query for all known services
	Services(q *consulapi.QueryOptions) (map[string][]string, *consulapi.QueryMeta, error)
	// Service is used to query catalog entries for a given service
	Service(service, tag string, q *consulapi.QueryOptions) ([]*consulapi.CatalogService, *consulapi.QueryMeta, error)
	// Connect is used to query catalog entries for a given Connect-enabled service
	Connect(service, tag string, q *consulapi.QueryOptions) ([]*consulapi.CatalogService, *consulapi.QueryMeta, error)
}

type clientWrapper struct {
	api *consulapi.Client
}

// NewConsulClientWrapper wraps the original consul client to allow for access in testing + simplification of calls
func NewConsulClientWrapper(consulClient *consulapi.Client) ClientWrapper {
	return &clientWrapper{consulClient}
}

func (c *clientWrapper) DataCenters() ([]string, error) {
	return c.api.Catalog().Datacenters()
}

func (c *clientWrapper) Services(q *consulapi.QueryOptions) (map[string][]string, *consulapi.QueryMeta, error) {
	return c.api.Catalog().Services(q)
}

func (c *clientWrapper) Service(service, tag string, q *consulapi.QueryOptions) ([]*consulapi.CatalogService, *consulapi.QueryMeta, error) {
	return c.api.Catalog().Service(service, tag, q)
}

func (c *clientWrapper) Connect(service, tag string, q *consulapi.QueryOptions) ([]*consulapi.CatalogService, *consulapi.QueryMeta, error) {
	return c.api.Catalog().Connect(service, tag, q)
}

// NewFilteredConsulClient is used to create a new client for filtered consul requests.
// We have a wrapper around the consul api client *consulapi.Client - so that we can filter requests
func NewFilteredConsulClient(client ClientWrapper, dataCenters []string, serviceTagsAllowlist []string) (ClientWrapper, error) {
	dcMap := make(map[string]struct{})
	tagsMap := make(map[string]struct{})
	for _, dc := range dataCenters {
		dcMap[dc] = struct{}{}
	}

	for _, tag := range serviceTagsAllowlist {
		tagsMap[tag] = struct{}{}
	}

	return &consul{
		api:                  client,
		dataCenters:          dcMap,
		serviceTagsAllowlist: tagsMap,
	}, nil
}

type consul struct {
	api ClientWrapper
	// allowlist of data centers to consider when querying the agent - If empty, all are allowed
	dataCenters map[string]struct{}
	// allowlist of serviceTags to consider when querying the agent - If emtpy, all are allowed
	serviceTagsAllowlist map[string]struct{}
}

func (c *consul) DataCenters() ([]string, error) {
	dc, err := c.api.DataCenters()
	if err != nil {
		return nil, err
	}
	return c.filterDataCenters(dc), nil
}

func (c *consul) Services(q *consulapi.QueryOptions) (map[string][]string, *consulapi.QueryMeta, error) {
	if err := c.validateDataCenter(q.Datacenter); err != nil {
		return nil, nil, err
	}
	services, queryMeta, err := c.api.Services(q)
	services = c.filterServices(services)
	return services, queryMeta, err
}

func (c *consul) Service(service, tag string, q *consulapi.QueryOptions) ([]*consulapi.CatalogService, *consulapi.QueryMeta, error) {
	if err := c.validateDataCenter(q.Datacenter); err != nil {
		return nil, nil, err
	}
	return c.api.Service(service, tag, q)
}

func (c *consul) Connect(service, tag string, q *consulapi.QueryOptions) ([]*consulapi.CatalogService, *consulapi.QueryMeta, error) {
	if err := c.validateDataCenter(q.Datacenter); err != nil {
		return nil, nil, err
	}
	return c.api.Connect(service, tag, q)
}

// Filters out the data centers not listed in the config
func (c *consul) filterDataCenters(dataCenters []string) []string {

	// If empty, all are allowed
	if len(c.dataCenters) == 0 {
		return dataCenters
	}

	var filtered []string
	for _, dc := range dataCenters {
		if _, ok := c.dataCenters[dc]; ok {
			filtered = append(filtered, dc)
		}
	}
	return filtered
}

// Filters out the services that do not have matching tags from the service_tags_allowlist
// input from services is a map of service name to slice of tags
func (c *consul) filterServices(services map[string][]string) map[string][]string {
	//if there is no allowlist, allow for all services
	if len(c.serviceTagsAllowlist) == 0 {
		return services
	}

	//Filter services by tags
	filteredServices := make(map[string][]string)
	for serviceName, sTags := range services {
		for _, tag := range sTags {
			if _, found := c.serviceTagsAllowlist[tag]; found {
				filteredServices[serviceName] = sTags
				break
			}
		}
	}
	return filteredServices
}

// Checks whether we are allowed to query the given data center
func (c *consul) validateDataCenter(dataCenter string) error {

	// If empty, all are allowed
	if len(c.dataCenters) == 0 {
		return nil
	}

	// If empty, the Consul client will use the default agent data center, which we should allow
	if _, ok := c.dataCenters[dataCenter]; dataCenter != "" && ok {
		return ForbiddenDataCenterErr(dataCenter)
	}
	return nil
}

// NewConsulServicesQueryOptions returns a QueryOptions configuration that's used for Consul queries to /catalog/services
func NewConsulServicesQueryOptions(dataCenter string, cm glooConsul.ConsulConsistencyModes, _ *glooConsul.QueryOptions) *consulapi.QueryOptions {
	return internalConsulQueryOptions(dataCenter, cm, false) // caching not supported by endpoint
}

// NewConsulCatalogServiceQueryOptions returns a QueryOptions configuration that's used for Consul queries to /catalog/service/:servicename
func NewConsulCatalogServiceQueryOptions(dataCenter string, cm glooConsul.ConsulConsistencyModes, queryOptions *glooConsul.QueryOptions) *consulapi.QueryOptions {
	useCache := true
	if cache := queryOptions.GetUseCache(); cache != nil {
		useCache = cache.GetValue()
	}
	return internalConsulQueryOptions(dataCenter, cm, useCache)
}

func internalConsulQueryOptions(dataCenter string, cm glooConsul.ConsulConsistencyModes, useCache bool) *consulapi.QueryOptions {
	// it can either be requireConsistent or allowStale or neither
	// choosing the Default Mode will clear both fields
	requireConsistent := cm == glooConsul.ConsulConsistencyModes_ConsistentMode
	allowStale := cm == glooConsul.ConsulConsistencyModes_StaleMode
	return &consulapi.QueryOptions{
		Datacenter:        dataCenter,
		AllowStale:        allowStale,
		RequireConsistent: requireConsistent,
		UseCache:          useCache,
	}
}

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
type ConsulClient interface {
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

func NewConsulClient(client *consulapi.Client, dataCenters []string) (ConsulClient, error) {
	dcMap := make(map[string]struct{})
	for _, dc := range dataCenters {
		dcMap[dc] = struct{}{}
	}

	return &consul{
		api:         client,
		dataCenters: dcMap,
	}, nil
}

type consul struct {
	api *consulapi.Client
	// Whitelist of data centers to consider when querying the agent
	dataCenters map[string]struct{}
}

func (c *consul) DataCenters() ([]string, error) {
	dc, err := c.api.Catalog().Datacenters()
	if err != nil {
		return nil, err
	}
	return c.filter(dc), nil
}

func (c *consul) Services(q *consulapi.QueryOptions) (map[string][]string, *consulapi.QueryMeta, error) {
	if err := c.validateDataCenter(q.Datacenter); err != nil {
		return nil, nil, err
	}
	return c.api.Catalog().Services(q)
}

func (c *consul) Service(service, tag string, q *consulapi.QueryOptions) ([]*consulapi.CatalogService, *consulapi.QueryMeta, error) {
	if err := c.validateDataCenter(q.Datacenter); err != nil {
		return nil, nil, err
	}
	return c.api.Catalog().Service(service, tag, q)
}

func (c *consul) Connect(service, tag string, q *consulapi.QueryOptions) ([]*consulapi.CatalogService, *consulapi.QueryMeta, error) {
	if err := c.validateDataCenter(q.Datacenter); err != nil {
		return nil, nil, err
	}
	return c.api.Catalog().Connect(service, tag, q)
}

// Filters out the data centers not listed in the config
func (c *consul) filter(dataCenters []string) []string {

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
func NewConsulServicesQueryOptions(dataCenter string, cm glooConsul.ConsulConsistencyModes, filter string, _ *glooConsul.QueryOptions) *consulapi.QueryOptions {
	return internalConsulQueryOptions(dataCenter, cm, filter, false) // caching not supported by endpoint
}

// NewConsulCatalogServiceQueryOptions returns a QueryOptions configuration that's used for Consul queries to /catalog/services/:servicename
func NewConsulCatalogServiceQueryOptions(dataCenter string, cm glooConsul.ConsulConsistencyModes, queryOptions *glooConsul.QueryOptions) *consulapi.QueryOptions {
	useCache := true
	if use := queryOptions.GetUseCache(); use != nil {
		useCache = use.GetValue()
	}
	return internalConsulQueryOptions(dataCenter, cm, "", useCache)
}

func internalConsulQueryOptions(dataCenter string, cm glooConsul.ConsulConsistencyModes, filter string, useCache bool) *consulapi.QueryOptions {
	// it can either be requireConsistent or allowStale or neither
	// choosing the Default Mode will clear both fields
	requireConsistent := cm == glooConsul.ConsulConsistencyModes_ConsistentMode
	allowStale := cm == glooConsul.ConsulConsistencyModes_StaleMode
	return &consulapi.QueryOptions{
		Datacenter:        dataCenter,
		AllowStale:        allowStale,
		RequireConsistent: requireConsistent,
		Filter:            filter,
		UseCache:          useCache,
	}
}

package upstreams

import (
	consulapi "github.com/hashicorp/consul/api"
)

//go:generate mockgen -destination=./mock_consul_client.go -source consul_client.go -package upstreams

// Wrap the Consul API in an interface to allow mocking
type ConsulClient interface {
	DataCenters() ([]string, error)
	Services(q *consulapi.QueryOptions) (map[string][]string, *consulapi.QueryMeta, error)
}

func NewConsulClient() (ConsulClient, error) {
	client, err := consulapi.NewClient(consulapi.DefaultConfig())
	if err != nil {
		return nil, err
	}
	return &consul{api: client}, nil
}

type consul struct {
	api *consulapi.Client
}

func (c *consul) DataCenters() ([]string, error) {
	return c.api.Catalog().Datacenters()
}

func (c *consul) Services(q *consulapi.QueryOptions) (map[string][]string, *consulapi.QueryMeta, error) {
	return c.api.Catalog().Services(q)
}

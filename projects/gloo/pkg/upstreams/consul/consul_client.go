package consul

import (
	"github.com/gogo/protobuf/types"
	consulapi "github.com/hashicorp/consul/api"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/errors"
)

//go:generate mockgen -destination=./mock_consul_client.go -source consul_client.go -package consul

// Wrap the Consul API in an interface to allow mocking
type ConsulClient interface {
	// Returns false if no connection to the Consul agent can be established
	CanConnect() bool
	DataCenters() ([]string, error)
	Services(q *consulapi.QueryOptions) (map[string][]string, *consulapi.QueryMeta, error)
}

func NewConsulClient(settings *v1.Settings) (ConsulClient, error) {
	consulConfig := consulapi.DefaultConfig()

	// If present, apply custom config
	var dataCenters map[string]bool
	if settings != nil && settings.Consul != nil {
		var err error
		consulConfig, dataCenters, err = apply(settings.Consul, consulConfig)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to configure Consul client")
		}
	}

	client, err := consulapi.NewClient(consulConfig)
	if err != nil {
		return nil, err
	}
	return &consul{
		api:         client,
		dataCenters: dataCenters,
	}, nil
}

type consul struct {
	api *consulapi.Client
	// Whitelist of data centers to consider when querying the agent
	dataCenters map[string]bool
}

func (c *consul) CanConnect() bool {
	_, err := c.api.Catalog().Datacenters()
	return err == nil
}

func (c *consul) DataCenters() ([]string, error) {
	dc, err := c.api.Catalog().Datacenters()
	if err != nil {
		return nil, err
	}
	return c.filter(dc), nil
}

func (c *consul) Services(q *consulapi.QueryOptions) (map[string][]string, *consulapi.QueryMeta, error) {
	return c.api.Catalog().Services(q)
}

// Applies the given settings to the default configuration
func apply(cfg *v1.Settings_ConsulConfiguration, defaultCfg *consulapi.Config) (*consulapi.Config, map[string]bool, error) {

	if cfg.Address != nil {
		defaultCfg.Address = cfg.Address.Value
	}

	if cfg.WaitTime != nil {
		duration, err := types.DurationFromProto(cfg.WaitTime)
		if err != nil {
			return nil, nil, err
		}
		defaultCfg.WaitTime = duration
	}

	dataCenters := make(map[string]bool)
	for _, dc := range cfg.DataCenters {
		dataCenters[dc] = true
	}

	return defaultCfg, dataCenters, nil
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

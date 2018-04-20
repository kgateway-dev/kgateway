package helpers

import (
	"fmt"

	"github.com/hashicorp/consul/api"
	"github.com/pkg/errors"
)

// uses default config
func ConsulServiceAddress(service, tag string) (string, error) {
	cfg := api.DefaultConfig()
	cli, err := api.NewClient(cfg)
	if err != nil {
		return "", err
	}
	instances, _, err := cli.Catalog().Service(service, tag, nil)
	if err != nil {
		return "", err
	}
	if len(instances) < 1 {
		return "", errors.New("no instances found for " + service + " : " + tag)
	}
	return fmt.Sprintf("%v:%v", instances[0].ServiceAddress, instances[0].ServicePort), nil
}

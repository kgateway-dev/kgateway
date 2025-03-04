package settings

import (
	"github.com/kelseyhightower/envconfig"
)

type Settings struct {
	// Controls the DnsLookupFamily for all static clusters created via Backend resources.
	// If not set, the Envoy default of AUTO will be used, which is effectively "V6_PREFERRED".
	// Supported values are: "V4_ONLY", "V6_ONLY", "V4_PREFERRED", "ALL".
	// Details on the behavior of these options are available on the Envoy documentation:
	// https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/cluster/v3/cluster.proto#enum-config-cluster-v3-cluster-dnslookupfamily
	DnsLookupFamily string `split_words:"true"`

	EnableIstioIntegration bool   `split_words:"true"`
	EnableAutoMtls         bool   `split_words:"true"`
	StsClusterName         string `split_words:"true"`
	StsUri                 string `split_words:"true"`

	// XdsServiceName is the name of the Kubernetes Service that serves xDS config.
	// It it assumed to be in the kgateway install namespace.
	XdsServiceName string `split_words:"true" default:"kgateway"`

	// XdsServicePort is the port of the Kubernetes Service that serves xDS config.
	// This corresponds to the value of the `grpc-xds` port in the service.
	XdsServicePort uint32 `split_words:"true" default:"9977"`
}

// BuildSettings returns a zero-valued Settings obj if error is encountered when parsing env
func BuildSettings() (*Settings, error) {
	settings := &Settings{}
	if err := envconfig.Process("KGW", settings); err != nil {
		return settings, err
	}
	return settings, nil
}

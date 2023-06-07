package services

import (
	"github.com/solo-io/gloo/test/services/envoy"
)

type EnvoyInstance = envoy.Instance
type EnvoyFactory = envoy.Factory

const DefaultProxyName = envoy.DefaultProxyName

func NextBindPort() uint32 {
	return envoy.NextBindPort()
}

func MustEnvoyFactory() envoy.Factory {
	return envoy.NewFactory()
}

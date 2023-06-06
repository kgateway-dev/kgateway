package services

import (
	"sync/atomic"

	"github.com/solo-io/gloo/test/ginkgo/parallel"
	"github.com/solo-io/gloo/test/services/envoy"
)

type EnvoyInstance = envoy.Instance
type EnvoyFactory = envoy.Factory
type EnvoyBootstrapBuilder = envoy.BootstrapBuilder
type DockerOptions = envoy.DockerOptions

const DefaultProxyName = envoy.DefaultProxyName

var bindPort = uint32(10080)

func NextBindPort() uint32 {
	return AdvanceBindPort(&bindPort)
}

func AdvanceBindPort(p *uint32) uint32 {
	return atomic.AddUint32(p, 1) + uint32(parallel.GetPortOffset())
}

func MustEnvoyFactory() envoy.Factory {
	return envoy.MustEnvoyFactory()
}

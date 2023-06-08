package envoy

import (
	"sync/atomic"

	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"

	"github.com/solo-io/gloo/test/ginkgo/parallel"
)

var (
	baseAdminPort     = defaults.EnvoyAdminPort
	baseHttpPort      = defaults.HttpPort
	baseHttpsPort     = defaults.HttpsPort
	baseTcpPort       = defaults.TcpPort
	baseHybridPort    = defaults.HybridPort
	baseAccessLogPort = uint32(10080)
)

func NextAccessLogPort() uint32 {
	return advancePort(&baseAccessLogPort)
}

func advanceRequestPorts() {
	defaults.HttpPort = advancePort(&baseHttpPort)
	defaults.HttpsPort = advancePort(&baseHttpsPort)
	defaults.HybridPort = advancePort(&baseHybridPort)
	defaults.TcpPort = advancePort(&baseTcpPort)
	defaults.EnvoyAdminPort = advancePort(&baseAdminPort)
}

func advancePort(p *uint32) uint32 {
	return atomic.AddUint32(p, 1) + uint32(parallel.GetPortOffset())
}

package envoy

import (
	"sync/atomic"

	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"

	"github.com/solo-io/gloo/test/ginkgo/parallel"
)

var (
	bindPort = uint32(10080)
)

func NextBindPort() uint32 {
	return advancePort(&bindPort)
}

func advanceRequestPorts() {
	defaults.EnvoyAdminPort = advancePort(&defaults.EnvoyAdminPort)
	defaults.HttpPort = advancePort(&defaults.HttpPort)
	defaults.HttpsPort = advancePort(&defaults.HttpsPort)
	defaults.TcpPort = advancePort(&defaults.TcpPort)
	defaults.HybridPort = advancePort(&defaults.HybridPort)
}

func advancePort(p *uint32) uint32 {
	return atomic.AddUint32(p, 1) + uint32(parallel.GetPortOffset())
}

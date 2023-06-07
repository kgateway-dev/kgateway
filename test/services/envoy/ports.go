package envoy

import (
	"sync/atomic"

	"github.com/solo-io/gloo/test/ginkgo/parallel"
)

var (
	adminPort = uint32(20000)
	bindPort  = uint32(10080)
)

func NextBindPort() uint32 {
	return AdvancePort(&bindPort)
}

func NextAdminPort() uint32 {
	return AdvancePort(&adminPort)
}

func AdvancePort(p *uint32) uint32 {
	return atomic.AddUint32(p, 1) + uint32(parallel.GetPortOffset())
}

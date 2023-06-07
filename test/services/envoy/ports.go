package envoy

import (
	"sync/atomic"

	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"

	"github.com/solo-io/gloo/test/ginkgo/parallel"
)

var (
	adminPort = uint32(20000)
	bindPort  = uint32(10080)

	HttpPort   = defaults.HttpPort
	HttpsPort  = defaults.HttpsPort
	TcpPort    = defaults.TcpPort
	HybridPort = defaults.HybridPort
)

func NextBindPort() uint32 {
	return AdvancePort(&bindPort)
}

func NextAdminPort() uint32 {
	return AdvancePort(&adminPort)
}

func AdvanceRequestPorts() {
	HttpPort = AdvancePort(&HttpPort)
	HttpsPort = AdvancePort(&HttpsPort)
	TcpPort = AdvancePort(&TcpPort)
	HybridPort = AdvancePort(&HybridPort)

	// NOTE TO DEVELOPERS:
	// This file contains definitions for port values that the test suite will use
	// Ideally these ports would be owned exclusively by the envoy package.
	// However, the challenge is that we have some default resources, which are created using the defaults package.
	// Therefore, we need to keep the defaults package ports in sync with the envoy ports

	defaults.HttpPort = HttpPort
	defaults.HttpsPort = HttpsPort
	defaults.HybridPort = HybridPort
	defaults.TcpPort = TcpPort
}

func AdvancePort(p *uint32) uint32 {
	return atomic.AddUint32(p, 1) + uint32(parallel.GetPortOffset())
}

package envoy

import (
	"sync/atomic"

	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"

	"github.com/solo-io/gloo/test/ginkgo/parallel"
)

var (
	bindPort = uint32(10080)

	AdminPort  = defaults.EnvoyAdminPort
	HttpPort   = defaults.HttpPort
	HttpsPort  = defaults.HttpsPort
	TcpPort    = defaults.TcpPort
	HybridPort = defaults.HybridPort
)

func NextBindPort() uint32 {
	return advancePort(&bindPort)
}

func advanceRequestPorts() {
	HttpPort = advancePort(&HttpPort)
	HttpsPort = advancePort(&HttpsPort)
	TcpPort = advancePort(&TcpPort)
	HybridPort = advancePort(&HybridPort)
	AdminPort = advancePort(&AdminPort)

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

func advancePort(p *uint32) uint32 {
	return atomic.AddUint32(p, 1) + uint32(parallel.GetPortOffset())
}

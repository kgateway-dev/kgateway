//go:build ignore

package envoy

import (
	"github.com/kgateway-dev/kgateway/v2/internal/gloo/pkg/defaults"

	"github.com/kgateway-dev/kgateway/v2/test/ginkgo/parallel"
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

func advancePort(port *uint32) uint32 {
	return parallel.AdvancePortSafeListen(port)
}

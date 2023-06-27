package envoy

import (
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
	return parallel.AdvancePort(&baseAccessLogPort)
}

func advanceRequestPorts() {
	defaults.HttpPort = parallel.AdvancePort(&baseHttpPort)
	defaults.HttpsPort = parallel.AdvancePort(&baseHttpsPort)
	defaults.HybridPort = parallel.AdvancePort(&baseHybridPort)
	defaults.TcpPort = parallel.AdvancePort(&baseTcpPort)
	defaults.EnvoyAdminPort = parallel.AdvancePort(&baseAdminPort)
}

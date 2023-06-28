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
	return parallel.AdvancePortSafeDenylist(&baseAccessLogPort)
}

func advanceRequestPorts() {
	defaults.HttpPort = parallel.AdvancePortSafeDenylist(&baseHttpPort)
	defaults.HttpsPort = parallel.AdvancePortSafeDenylist(&baseHttpsPort)
	defaults.HybridPort = parallel.AdvancePortSafeDenylist(&baseHybridPort)
	defaults.TcpPort = parallel.AdvancePortSafeDenylist(&baseTcpPort)
	defaults.EnvoyAdminPort = parallel.AdvancePortSafeDenylist(&baseAdminPort)
}

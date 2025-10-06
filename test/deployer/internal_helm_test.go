package deployer

import (
	"testing"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/schemes"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
)

func TestRenderHelmChart(t *testing.T) {
	tests := []HelmTestCase{
		{
			Name:      "basic gateway with default gatewayclass and no gwparams",
			InputFile: "base-gateway",
		},
		{
			Name:      "gwparams with omitDefaultSecurityContext",
			InputFile: "omit-default-security-context",
		},
		{
			Name:      "agentgateway",
			InputFile: "agentgateway",
		},
		{
			Name:      "agentgateway OmitDefaultSecurityContext true GWP via GWC",
			InputFile: "agentgateway-omitdefaultsecuritycontext",
		},
		{
			Name:      "agentgateway OmitDefaultSecurityContext true GWP via GW",
			InputFile: "agentgateway-omitdefaultsecuritycontext-ref-gwp-on-gw",
		},
	}

	tester := DeployerTester{
		ControllerName:    wellknown.DefaultGatewayControllerName,
		AgwControllerName: wellknown.DefaultAgwControllerName,
		ClassName:         wellknown.DefaultGatewayClassName,
		WaypointClassName: wellknown.DefaultWaypointClassName,
		AgwClassName:      wellknown.DefaultAgwClassName,
	}

	dir := fsutils.MustGetThisDir()
	scheme := schemes.GatewayScheme()
	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			tester.RunHelmChartTest(t, tt, scheme, dir, nil)
		})
	}
}

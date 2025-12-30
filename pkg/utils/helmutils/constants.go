package helmutils

const (
	ChartName                = "kgateway"
	CRDChartName             = "kgateway-crds"
	AgentgatewayChartName    = "agentgateway"
	AgentgatewayCRDChartName = "agentgateway-crds"

	DefaultChartUri                = "oci://ghcr.io/kgateway-dev/charts/kgateway"
	DefaultCRDChartUri             = "oci://ghcr.io/kgateway-dev/charts/kgateway-crds"
	DefaultagentGatewayChartUri    = "cr.agentgateway.dev/charts/agentgateway"
	DefaultagentGatewayCRDChartUri = "cr.agentgateway.dev/charts/agentgateway-crds"
)

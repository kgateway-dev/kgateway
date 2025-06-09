package trafficpolicy

import "github.com/kgateway-dev/kgateway/v2/internal/kgateway/plugins"

type ProviderNeededMap struct {
	// map filterhcain name -> providername -> provider
	Providers map[string]map[string]*TrafficPolicyGatewayExtensionIR
}

func (p *ProviderNeededMap) Add(fcn, providerName string, provider *TrafficPolicyGatewayExtensionIR) {
	if p.Providers == nil {
		p.Providers = make(map[string]map[string]*TrafficPolicyGatewayExtensionIR)
	}
	if p.Providers[fcn] == nil {
		p.Providers[fcn] = make(map[string]*TrafficPolicyGatewayExtensionIR)
	}
	p.Providers[fcn][providerName] = provider
}

func AddDisableFilterIfNeeded(filters []plugins.StagedHttpFilter) []plugins.StagedHttpFilter {
	for _, f := range filters {
		if f.Filter.GetName() == extAuthGlobalDisableFilterName {
			return filters
		}
	}

	f := plugins.MustNewStagedFilter(extAuthGlobalDisableFilterName,
		setMetadataConfig,
		plugins.BeforeStage(plugins.FaultStage))
	f.Filter.Disabled = true
	filters = append(filters, f)
	return filters
}

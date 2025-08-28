package trafficpolicy

import (
	set_metadata "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/set_metadata/v3"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/plugins"
)

type ProviderNeededMap struct {
	// map filter_chain name -> providers
	Providers map[string][]Provider
}

type Provider struct {
	Name      string
	Extension *TrafficPolicyGatewayExtensionIR
}

func (p *ProviderNeededMap) Add(filterChain, providerName string, provider *TrafficPolicyGatewayExtensionIR) {
	if p.Providers == nil {
		p.Providers = make(map[string][]Provider)
	}
	p.Providers[filterChain] = append(p.Providers[filterChain], Provider{
		Name:      providerName,
		Extension: provider,
	})
}

func AddDisableFilterIfNeeded(
	filters []plugins.StagedHttpFilter,
	disableFilterName string,
	disableFilterMetadataNamespace string,
) []plugins.StagedHttpFilter {
	for _, f := range filters {
		if f.Filter.GetName() == disableFilterName {
			return filters
		}
	}

	f := plugins.MustNewStagedFilter(
		disableFilterName, newSetMetadataConfig(disableFilterMetadataNamespace), plugins.BeforeStage(plugins.FaultStage))
	f.Filter.Disabled = true
	filters = append(filters, f)
	return filters
}

func newSetMetadataConfig(metadataNamespace string) *set_metadata.Config {
	return &set_metadata.Config{
		Metadata: []*set_metadata.Metadata{
			{
				MetadataNamespace: metadataNamespace,
				Value: &structpb.Struct{Fields: map[string]*structpb.Value{
					globalFilterDisableMetadataKey: structpb.NewBoolValue(true),
				}},
			},
		},
	}
}

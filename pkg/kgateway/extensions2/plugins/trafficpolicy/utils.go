package trafficpolicy

import (
	set_metadata "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/set_metadata/v3"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/filters"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
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
	stagedFilters []filters.StagedHttpFilter,
	disableFilterName string,
	disableFilterMetadataNamespace string,
) []filters.StagedHttpFilter {
	for _, f := range stagedFilters {
		if f.Filter.GetName() == disableFilterName {
			return stagedFilters
		}
	}

	f := filters.MustNewStagedFilter(
		disableFilterName,
		newSetMetadataConfig(disableFilterMetadataNamespace),
		filters.BeforeStage(filters.FaultStage),
	)
	f.Filter.Disabled = true
	stagedFilters = append(stagedFilters, f)
	return stagedFilters
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

func AddAuthEnabledFilterIfNeeded(
	stagedFilters []filters.StagedHttpFilter,
	filterName string,
	enableAuthMetadata bool,
) []filters.StagedHttpFilter {
	if !enableAuthMetadata {
		return stagedFilters
	}
	for _, f := range stagedFilters {
		if f.Filter.GetName() == filterName {
			return stagedFilters
		}
	}

	f := filters.MustNewStagedFilter(filterName,
		GenerateBlankTransformationConfig(),
		filters.AfterStage(filters.AuthNStage),
	)
	f.Filter.Disabled = true
	stagedFilters = append(stagedFilters, f)
	return stagedFilters
}

// AddAuthMetadataIfNeeded sets the `dev.kgateway.auth_policy:auth_succeeded=true` dynamic metadata
// via the transformation filter when enableAuthMetadata is true
func AddAuthMetadataIfNeeded(perFilterConfig *ir.TypedFilterConfigMap, filterName string, enableAuthMetadata bool) {
	if !enableAuthMetadata {
		return
	}
	perFilterConfig.AddTypedConfig(filterName, generateDynamicMetadata(AuthPolicyMetadataNamespace, map[string]kgateway.InjaTemplate{
		AuthSucceededMetadataKey: kgateway.InjaTemplate("true"),
	}))
}

// AddBlankTransformationIfNeeded sets a blank (no-op) per-route transformation on the named filter when
// enableAuthMetadata is true. This prevents auth metadata from being set on routes
// where auth is explicitly disabled or not configured.
func AddBlankTransformationIfNeeded(perFilterConfig *ir.TypedFilterConfigMap, filterName string, enableAuthMetadata bool) {
	if !enableAuthMetadata {
		return
	}
	perFilterConfig.AddTypedConfig(filterName, GenerateBlankTransformationConfigPerRoute())
}

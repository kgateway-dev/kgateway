package trafficpolicy

import (
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	gzipcompressorv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/compression/gzip/compressor/v3"
	gzipdecompressorv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/compression/gzip/decompressor/v3"
	compressorv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/compressor/v3"
	decompressorv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/decompressor/v3"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/filters"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

const (
	compressorFilterName   = "envoy.filters.http.compressor"
	decompressorFilterName = "envoy.filters.http.decompressor"
)

type compressionIR struct {
	enable bool
}

type decompressionIR struct {
	enable bool
}

var (
	_ PolicySubIR = &compressionIR{}
	_ PolicySubIR = &decompressionIR{}
)

func (c *compressionIR) Equals(other PolicySubIR) bool {
	oc, ok := other.(*compressionIR)
	if !ok {
		return false
	}
	if c == nil || other == nil {
		return c == nil && oc == nil
	}
	return c.enable == oc.enable
}

func (c *compressionIR) Validate() error { return nil }

func (d *decompressionIR) Equals(other PolicySubIR) bool {
	od, ok := other.(*decompressionIR)
	if !ok {
		return false
	}
	if d == nil || other == nil {
		return d == nil && od == nil
	}
	return d.enable == od.enable
}

func (d *decompressionIR) Validate() error { return nil }

// constructCompression builds IR for response compression (per-route) and decompression (listener enable toggle).
func constructCompression(spec kgateway.TrafficPolicySpec, out *trafficPolicySpecIr) {
	if spec.Compression == nil {
		return
	}

	// Enable response compression if not disabled
	if rc := spec.Compression.ResponseCompression; rc != nil {
		// Note: we intentionally rely on Envoy defaults for algorithm (gzip at listener) and content types.
		out.compression = &compressionIR{enable: (rc.Disable == nil)}
	}

	// Enable request decompression if not disabled
	if dc := spec.Compression.RequestDecompression; dc != nil {
		out.decompression = &decompressionIR{enable: (dc.Disable == nil)}
	}
}

func (p *trafficPolicyPluginGwPass) handleCompression(fcn string, pCtxTypedFilterConfig *ir.TypedFilterConfigMap, comp *compressionIR) {
	if comp == nil {
		return
	}

	// Set per-route typed config to enable or disable compression on routes.
	if comp.enable {
		pCtxTypedFilterConfig.AddTypedConfig(compressorFilterName, EnableFilterPerRoute())
	} else {
		pCtxTypedFilterConfig.AddTypedConfig(compressorFilterName, DisableFilterPerRoute())
		return
	}

	// Ensure a disabled baseline compressor filter is present in the listener chain.
	if p.compressorInChain == nil {
		p.compressorInChain = make(map[string]*compressorv3.Compressor)
	}
	if _, ok := p.compressorInChain[fcn]; !ok {
		// Build gzip compressor library with Envoy defaults.
		gzipAny, _ := utils.MessageToAny(&gzipcompressorv3.Gzip{})
		p.compressorInChain[fcn] = &compressorv3.Compressor{
			RequestDirectionConfig: &compressorv3.Compressor_RequestDirectionConfig{
				CommonConfig: &compressorv3.Compressor_CommonDirectionConfig{
					Enabled: &envoycorev3.RuntimeFeatureFlag{
						DefaultValue: wrapperspb.Bool(false),
					},
				},
			},
			CompressorLibrary: &envoycorev3.TypedExtensionConfig{
				Name:        "envoy.compression.gzip.compressor",
				TypedConfig: gzipAny,
			},
		}
	}
}

func (p *trafficPolicyPluginGwPass) handleDecompression(fcn string, pCtxTypedFilterConfig *ir.TypedFilterConfigMap, decomp *decompressionIR) {
	if decomp == nil {
		return
	}
	if decomp.enable {
		pCtxTypedFilterConfig.AddTypedConfig(decompressorFilterName, EnableFilterPerRoute())
	} else {
		pCtxTypedFilterConfig.AddTypedConfig(decompressorFilterName, DisableFilterPerRoute())
		return
	}
	if p.decompressorInChain == nil {
		p.decompressorInChain = make(map[string]*decompressorv3.Decompressor)
	}
	if _, ok := p.decompressorInChain[fcn]; !ok {
		gzipAny, _ := utils.MessageToAny(&gzipdecompressorv3.Gzip{})
		p.decompressorInChain[fcn] = &decompressorv3.Decompressor{
			ResponseDirectionConfig: &decompressorv3.Decompressor_ResponseDirectionConfig{
				CommonConfig: &decompressorv3.Decompressor_CommonDirectionConfig{
					Enabled: &envoycorev3.RuntimeFeatureFlag{
						DefaultValue: wrapperspb.Bool(false),
					},
				},
			},
			DecompressorLibrary: &envoycorev3.TypedExtensionConfig{
				Name:        "envoy.compression.gzip.decompressor",
				TypedConfig: gzipAny,
			},
		}
	}
}

// HttpFilters wiring is in traffic_policy_plugin.go
func addCompressionFiltersIfNeeded(staged []filters.StagedHttpFilter, p *trafficPolicyPluginGwPass, fcn string) []filters.StagedHttpFilter {
	if c := p.compressorInChain[fcn]; c != nil {
		filter := filters.MustNewStagedFilter(
			compressorFilterName,
			c,
			filters.AfterStage(filters.WellKnownFilterStage(filters.CorsStage)),
		)
		filter.Filter.Disabled = true
		staged = append(staged, filter)
	}
	if d := p.decompressorInChain[fcn]; d != nil {
		filter := filters.MustNewStagedFilter(
			decompressorFilterName,
			d,
			filters.AfterStage(filters.WellKnownFilterStage(filters.CorsStage)),
		)
		filter.Filter.Disabled = true
		staged = append(staged, filter)
	}
	return staged
}

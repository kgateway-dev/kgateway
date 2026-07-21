package trafficpolicy

import (
	"slices"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	brotlicompressorv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/compression/brotli/compressor/v3"
	brotlidecompressorv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/compression/brotli/decompressor/v3"
	gzipcompressorv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/compression/gzip/compressor/v3"
	gzipdecompressorv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/compression/gzip/decompressor/v3"
	zstdcompressorv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/compression/zstd/compressor/v3"
	zstddecompressorv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/compression/zstd/decompressor/v3"
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
	// libraries are the response compression codecs to offer, in preference order.
	// Only meaningful when enable is true.
	libraries []kgateway.CompressionLibrary
}

type decompressionIR struct {
	enable bool
	// codecs to accept, meaningful only when enabled.
	libraries []kgateway.CompressionLibrary
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
	if c == nil || oc == nil {
		return c == nil && oc == nil
	}
	if c.enable != oc.enable {
		return false
	}
	// When disabled, all codecs are turned off regardless of libraries.
	if !c.enable {
		return true
	}
	if len(c.libraries) != len(oc.libraries) {
		return false
	}
	for i := range c.libraries {
		if c.libraries[i] != oc.libraries[i] {
			return false
		}
	}
	return true
}

func (c *compressionIR) Validate() error { return nil }

func (d *decompressionIR) Equals(other PolicySubIR) bool {
	od, ok := other.(*decompressionIR)
	if !ok {
		return false
	}
	if d == nil || od == nil {
		return d == nil && od == nil
	}
	if d.enable != od.enable {
		return false
	}
	if !d.enable {
		return true
	}
	return slices.Equal(d.libraries, od.libraries)
}

func (d *decompressionIR) Validate() error { return nil }

// constructCompression builds IR for response compression (per-route) and decompression (listener enable toggle).
func constructCompression(spec kgateway.TrafficPolicySpec, out *trafficPolicySpecIr) {
	if spec.Compression == nil {
		return
	}

	// Enable response compression if not disabled
	if rc := spec.Compression.ResponseCompression; rc != nil {
		// Default to gzip for backward compatibility when no codec is selected.
		// Note: we intentionally rely on Envoy defaults for the codec config and content types.
		libraries := rc.Libraries
		if len(libraries) == 0 {
			libraries = []kgateway.CompressionLibrary{kgateway.CompressionGzip}
		}
		out.compression = &compressionIR{enable: (rc.Disable == nil), libraries: libraries}
	}

	// Enable request decompression if not disabled
	if dc := spec.Compression.RequestDecompression; dc != nil {
		libraries := dc.Libraries
		if len(libraries) == 0 {
			libraries = []kgateway.CompressionLibrary{kgateway.CompressionGzip}
		} else {
			// Envoy selects the decompressor by Content-Encoding, so order is not significant. Sort
			// to a canonical form so reordered lists produce equal IR and stable config.
			libraries = slices.Clone(libraries)
			slices.Sort(libraries)
		}
		out.decompression = &decompressionIR{enable: (dc.Disable == nil), libraries: libraries}
	}
}

// compressorEntry is a single codec's compressor filter installed in a filter chain.
type compressorEntry struct {
	// filterName is the unique HTTP filter name for this compressor.
	filterName string
	compressor *compressorv3.Compressor
}

// allCompressionLibraries lists every supported response compression codec. Used on the
// disable path to turn off every codec a higher-level policy might have enabled.
var allCompressionLibraries = []kgateway.CompressionLibrary{
	kgateway.CompressionGzip,
	kgateway.CompressionBrotli,
	kgateway.CompressionZstd,
}

func (p *trafficPolicyPluginGwPass) handleCompression(fcn string, pCtxTypedFilterConfig *ir.TypedFilterConfigMap, comp *compressionIR) {
	if comp == nil {
		return
	}

	// Disable path: the route turns compression off. The codecs enabled by a higher-level
	// policy are not known here, so disable every codec's compressor filter and mark the
	// per-route config optional so Envoy ignores codecs absent from the chain.
	if !comp.enable {
		for _, library := range allCompressionLibraries {
			pCtxTypedFilterConfig.AddTypedConfig(compressorFilterNameFor(library), DisableFilterPerRouteOptional())
		}
		return
	}

	if p.compressorInChain == nil {
		p.compressorInChain = make(map[string][]compressorEntry)
	}
	// One compressor filter per codec, shared across routes. Each route enables the subset it
	// offers and Envoy picks the winner from the client's Accept-Encoding header, so no
	// server-side preference is set.
	for _, library := range comp.libraries {
		filterName := compressorFilterNameFor(library)
		pCtxTypedFilterConfig.AddTypedConfig(filterName, EnableFilterPerRoute())

		if !hasCompressorNamed(p.compressorInChain[fcn], filterName) {
			p.compressorInChain[fcn] = append(p.compressorInChain[fcn], compressorEntry{
				filterName: filterName,
				compressor: newCompressor(library),
			})
		}
	}
}

// compressorFilterNameFor returns the per-codec filter name. Gzip uses the bare compressor name
// so existing single-codec configs stay byte-identical.
func compressorFilterNameFor(library kgateway.CompressionLibrary) string {
	switch library {
	case kgateway.CompressionBrotli:
		return compressorFilterName + ".brotli"
	case kgateway.CompressionZstd:
		return compressorFilterName + ".zstd"
	default: // CompressionGzip
		return compressorFilterName
	}
}

func hasCompressorNamed(entries []compressorEntry, filterName string) bool {
	for i := range entries {
		if entries[i].filterName == filterName {
			return true
		}
	}
	return false
}

// newCompressor builds a disabled baseline compressor filter for the given codec, using
// Envoy defaults for the codec config (no quality/level knobs).
func newCompressor(library kgateway.CompressionLibrary) *compressorv3.Compressor {
	return &compressorv3.Compressor{
		RequestDirectionConfig: &compressorv3.Compressor_RequestDirectionConfig{
			CommonConfig: &compressorv3.Compressor_CommonDirectionConfig{
				Enabled: &envoycorev3.RuntimeFeatureFlag{
					DefaultValue: wrapperspb.Bool(false),
				},
			},
		},
		CompressorLibrary: compressorLibraryFor(library),
	}
}

// compressorLibraryFor returns the Envoy compressor library extension config for the given
// codec. The typed config is left at Envoy defaults (no quality/level knobs).
func compressorLibraryFor(library kgateway.CompressionLibrary) *envoycorev3.TypedExtensionConfig {
	switch library {
	case kgateway.CompressionBrotli:
		brotliAny, _ := utils.MessageToAny(&brotlicompressorv3.Brotli{})
		return &envoycorev3.TypedExtensionConfig{
			Name:        "envoy.compression.brotli.compressor",
			TypedConfig: brotliAny,
		}
	case kgateway.CompressionZstd:
		zstdAny, _ := utils.MessageToAny(&zstdcompressorv3.Zstd{})
		return &envoycorev3.TypedExtensionConfig{
			Name:        "envoy.compression.zstd.compressor",
			TypedConfig: zstdAny,
		}
	default: // CompressionGzip and unset both map to gzip for backward compatibility.
		gzipAny, _ := utils.MessageToAny(&gzipcompressorv3.Gzip{})
		return &envoycorev3.TypedExtensionConfig{
			Name:        "envoy.compression.gzip.compressor",
			TypedConfig: gzipAny,
		}
	}
}

func (p *trafficPolicyPluginGwPass) handleDecompression(fcn string, pCtxTypedFilterConfig *ir.TypedFilterConfigMap, decomp *decompressionIR) {
	if decomp == nil {
		return
	}

	// Disable path: turn off every codec's decompressor filter. Names are fixed per codec (no
	// settings), so a route-level disable always covers whatever a broader policy enabled. The
	// per-route config is optional so Envoy ignores codecs absent from the chain.
	if !decomp.enable {
		for _, library := range allCompressionLibraries {
			pCtxTypedFilterConfig.AddTypedConfig(decompressorFilterNameFor(library), DisableFilterPerRouteOptional())
		}
		return
	}

	if p.decompressorInChain == nil {
		p.decompressorInChain = make(map[string][]decompressorEntry)
	}
	// One decompressor filter per codec, shared across routes. Each route enables the subset it
	// accepts and Envoy picks the decompressor by the request's Content-Encoding.
	for _, library := range decomp.libraries {
		filterName := decompressorFilterNameFor(library)
		pCtxTypedFilterConfig.AddTypedConfig(filterName, EnableFilterPerRoute())

		if !hasDecompressorNamed(p.decompressorInChain[fcn], filterName) {
			p.decompressorInChain[fcn] = append(p.decompressorInChain[fcn], decompressorEntry{
				filterName:   filterName,
				decompressor: newDecompressor(library),
			})
		}
	}
}

// decompressorEntry is a single codec's decompressor filter installed in a filter chain.
type decompressorEntry struct {
	filterName   string
	decompressor *decompressorv3.Decompressor
}

// decompressorFilterNameFor returns the per-codec decompressor filter name. Gzip keeps the bare
// decompressor name so existing single-codec configs stay byte-identical.
func decompressorFilterNameFor(library kgateway.CompressionLibrary) string {
	switch library {
	case kgateway.CompressionBrotli:
		return decompressorFilterName + ".brotli"
	case kgateway.CompressionZstd:
		return decompressorFilterName + ".zstd"
	default: // CompressionGzip
		return decompressorFilterName
	}
}

func hasDecompressorNamed(entries []decompressorEntry, filterName string) bool {
	for i := range entries {
		if entries[i].filterName == filterName {
			return true
		}
	}
	return false
}

// newDecompressor builds a decompressor filter for the given codec. Response-direction
// decompression is disabled, so only request bodies are decompressed.
func newDecompressor(library kgateway.CompressionLibrary) *decompressorv3.Decompressor {
	return &decompressorv3.Decompressor{
		ResponseDirectionConfig: &decompressorv3.Decompressor_ResponseDirectionConfig{
			CommonConfig: &decompressorv3.Decompressor_CommonDirectionConfig{
				Enabled: &envoycorev3.RuntimeFeatureFlag{
					DefaultValue: wrapperspb.Bool(false),
				},
			},
		},
		DecompressorLibrary: decompressorLibraryFor(library),
	}
}

// decompressorLibraryFor returns the Envoy decompressor library extension config for the codec.
func decompressorLibraryFor(library kgateway.CompressionLibrary) *envoycorev3.TypedExtensionConfig {
	switch library {
	case kgateway.CompressionBrotli:
		brotliAny, _ := utils.MessageToAny(&brotlidecompressorv3.Brotli{})
		return &envoycorev3.TypedExtensionConfig{
			Name:        "envoy.compression.brotli.decompressor",
			TypedConfig: brotliAny,
		}
	case kgateway.CompressionZstd:
		zstdAny, _ := utils.MessageToAny(&zstddecompressorv3.Zstd{})
		return &envoycorev3.TypedExtensionConfig{
			Name:        "envoy.compression.zstd.decompressor",
			TypedConfig: zstdAny,
		}
	default: // CompressionGzip and unset both map to gzip for backward compatibility.
		gzipAny, _ := utils.MessageToAny(&gzipdecompressorv3.Gzip{})
		return &envoycorev3.TypedExtensionConfig{
			Name:        "envoy.compression.gzip.decompressor",
			TypedConfig: gzipAny,
		}
	}
}

// HttpFilters wiring is in traffic_policy_plugin.go
func addCompressionFiltersIfNeeded(staged []filters.StagedHttpFilter, p *trafficPolicyPluginGwPass, fcn string) []filters.StagedHttpFilter {
	// One disabled-by-default compressor filter per codec. Order is not significant to
	// negotiation, so the weights only give deterministic output.
	for i, entry := range p.compressorInChain[fcn] {
		filter := filters.MustNewStagedFilter(
			entry.filterName,
			entry.compressor,
			filters.RelativeToStage(filters.WellKnownFilterStage(filters.CorsStage), 1+i),
		)
		filter.Filter.Disabled = true
		staged = append(staged, filter)
	}
	// One disabled-by-default decompressor filter per codec. Order is not significant (Envoy
	// selects by Content-Encoding), and same-stage filters sort deterministically by name.
	for _, entry := range p.decompressorInChain[fcn] {
		filter := filters.MustNewStagedFilter(
			entry.filterName,
			entry.decompressor,
			filters.AfterStage(filters.WellKnownFilterStage(filters.CorsStage)),
		)
		filter.Filter.Disabled = true
		staged = append(staged, filter)
	}
	return staged
}

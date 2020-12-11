package wasm

//go:generate mockgen -destination mocks/mock_cache.go  github.com/solo-io/wasm/tools/wasme/pkg/cache Cache

import (
	"context"
	"net/http"
	"strings"
	"sync"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_extensions_filters_http_wasm_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/wasm/v3"
	envoy_extensions_wasm_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/wasm/v3"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/golang/protobuf/ptypes/duration"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/wasm"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/wasm/tools/wasme/pkg/defaults"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

const (
	FilterName       = "envoy.filters.http.wasm"
	V8Runtime        = "envoy.wasm.runtime.v8"
	WavmRuntime      = "envoy.wasm.runtime.wavm"
	VmId             = "gloo-vm-id"
	WasmCacheCluster = "wasm-cache"
)

var (
	once       sync.Once
	imageCache = defaults.NewDefaultCache()

	defaultPluginPredicate = plugins.AcceptedStage
	defaultPluginStage     = plugins.BeforeStage(defaultPluginPredicate)
)

// Compile-time assertion
var _ plugins.Plugin = &Plugin{}
var _ plugins.HttpFilterPlugin = &Plugin{}

type Plugin struct{}

func NewPlugin() *Plugin {
	once.Do(func() {
		// TODO(EItanya): move this into a setup loop, rather than living in the filter
		// It makes sense that it should only start under certain circumstances, but starting
		// a web server from a plugin feels like an anti-pattern
		go http.ListenAndServe(":9979", imageCache)
	})
	return &Plugin{}
}

// TODO:not a string..
type Schema string

type CachedPlugin struct {
	Schema Schema
	Sha256 string
}

func (p *Plugin) Init(params plugins.InitParams) error {
	return nil
}

func (p *Plugin) ensureFilter(wasmFilter *wasm.WasmFilter) (*plugins.StagedHttpFilter, error) {

	cachedPlugin, err := p.ensurePluginInCache(wasmFilter)
	if err != nil {
		return nil, err
	}

	err = p.verifyConfiguration(cachedPlugin.Schema, wasmFilter.Config)
	if err != nil {
		return nil, err
	}

	var runtime string
	switch wasmFilter.GetVmType() {
	case wasm.WasmFilter_V8:
		runtime = V8Runtime
	case wasm.WasmFilter_WAVM:
		runtime = WavmRuntime
	}

	filterCfg := &envoy_extensions_filters_http_wasm_v3.Wasm{
		Config: &envoy_extensions_wasm_v3.PluginConfig{
			Name:          wasmFilter.Name,
			RootId:        wasmFilter.RootId,
			Configuration: wasmFilter.Config,
			Vm: &envoy_extensions_wasm_v3.PluginConfig_VmConfig{
				VmConfig: &envoy_extensions_wasm_v3.VmConfig{
					VmId:                VmId,
					Runtime:             runtime,
					NackOnCodeCacheMiss: true,
					Code: &envoy_config_core_v3.AsyncDataSource{
						Specifier: &envoy_config_core_v3.AsyncDataSource_Remote{
							Remote: &envoy_config_core_v3.RemoteDataSource{
								HttpUri: &envoy_config_core_v3.HttpUri{
									Uri: "http://gloo/images/" + cachedPlugin.Sha256,
									HttpUpstreamType: &envoy_config_core_v3.HttpUri_Cluster{
										Cluster: WasmCacheCluster,
									},
									Timeout: &duration.Duration{
										Seconds: 5, // TODO: customize
									},
								},
								Sha256: cachedPlugin.Sha256,
							},
						},
					},
				},
			},
		},
	}

	pluginStage := TransformWasmFilterStage(wasmFilter.GetFilterStage())
	stagedFilter, err := plugins.NewStagedFilterWithConfig(FilterName, filterCfg, pluginStage)
	if err != nil {
		return nil, err
	}

	return &stagedFilter, nil
}

func (p *Plugin) ensurePluginInCache(filter *wasm.WasmFilter) (*CachedPlugin, error) {

	digest, err := imageCache.Add(context.TODO(), filter.Image)
	if err != nil {
		return nil, err
	}
	return &CachedPlugin{
		Sha256: strings.TrimPrefix(string(digest), "sha256:"),
	}, nil
}

func (p *Plugin) verifyConfiguration(schema Schema, config *any.Any) error {
	// everything goes now-a-days
	return nil
}

func (p *Plugin) HttpFilters(params plugins.Params, l *v1.HttpListener) ([]plugins.StagedHttpFilter, error) {
	wasm := l.GetOptions().GetWasm()
	if wasm != nil {
		var result []plugins.StagedHttpFilter
		for _, wasmFilter := range wasm.GetFilters() {
			stagedPlugin, err := p.ensureFilter(wasmFilter)
			if err != nil {
				return nil, err
			}
			result = append(result, *stagedPlugin)
		}
		return result, nil
	}
	return nil, nil
}

func TransformWasmFilterStage(filterStage *wasm.FilterStage) plugins.FilterStage {
	if filterStage == nil {
		return defaultPluginStage
	}

	var resultStage plugins.WellKnownFilterStage
	switch filterStage.GetStage() {
	case wasm.FilterStage_FaultStage:
		resultStage = plugins.FaultStage
	case wasm.FilterStage_CorsStage:
		resultStage = plugins.CorsStage
	case wasm.FilterStage_WafStage:
		resultStage = plugins.WafStage
	case wasm.FilterStage_AuthNStage:
		resultStage = plugins.AuthNStage
	case wasm.FilterStage_AuthZStage:
		resultStage = plugins.AuthZStage
	case wasm.FilterStage_RateLimitStage:
		resultStage = plugins.RateLimitStage
	case wasm.FilterStage_AcceptedStage:
		resultStage = plugins.AcceptedStage
	case wasm.FilterStage_OutAuthStage:
		resultStage = plugins.OutAuthStage
	case wasm.FilterStage_RouteStage:
		resultStage = plugins.RouteStage
	}

	var result plugins.FilterStage
	switch filterStage.GetPredicate() {
	case wasm.FilterStage_During:
		result = plugins.DuringStage(resultStage)
	case wasm.FilterStage_Before:
		result = plugins.BeforeStage(resultStage)
	case wasm.FilterStage_After:
		result = plugins.AfterStage(resultStage)
	}
	return result
}

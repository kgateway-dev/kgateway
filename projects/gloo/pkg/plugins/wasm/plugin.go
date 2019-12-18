package wasm

//go:generate mockgen -destination mocks/mock_cache.go  github.com/solo-io/wasme/pkg/cache Cache

import (
	"context"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/gogo/protobuf/types"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/api/v2/config"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/wasm"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/external/envoy/api/v2/core"
	"github.com/solo-io/wasme/pkg/defaults"
)

const (
	FilterName       = "envoy.filters.http.wasm"
	V8Runtime        = "envoy.wasm.runtime.v8"
	WavmRuntime      = "envoy.wasm.runtime.wavm"
	VmId             = "gloo-vm-id"
	WasmCacheCluster = "wasm-cache"

	WasmEnabled = "WASM_ENABLED"
)

var (
	once       sync.Once
	imageCache = defaults.NewDefaultCache()
)

type Plugin struct {
}

func NewPlugin() *Plugin {
	once.Do(func() {
		if os.Getenv(WasmEnabled) != "" {
			go http.ListenAndServe(":9979", imageCache)
		}
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

func (p *Plugin) plugin(pc *wasm.PluginSource) (*plugins.StagedHttpFilter, error) {

	cachedPlugin, err := p.ensurePluginInCache(pc)
	if err != nil {
		return nil, err
	}

	err = p.verifyConfiguration(cachedPlugin.Schema, pc.Config)
	if err != nil {
		return nil, err
	}

	var runtime string
	switch pc.GetVmType() {
	case wasm.PluginSource_V8:
		runtime = V8Runtime
	case wasm.PluginSource_WAVM:
		runtime = WavmRuntime
	}

	filterCfg := &config.WasmService{
		Config: &config.PluginConfig{
			Name:          pc.Name,
			RootId:        pc.RootId,
			Configuration: pc.Config,
			VmConfig: &config.VmConfig{
				VmId:    VmId,
				Runtime: runtime,
				Code: &core.AsyncDataSource{
					Specifier: &core.AsyncDataSource_Remote{
						Remote: &core.RemoteDataSource{
							HttpUri: &core.HttpUri{
								Uri: "http://gloo/images/" + cachedPlugin.Sha256,
								HttpUpstreamType: &core.HttpUri_Cluster{
									Cluster: WasmCacheCluster,
								},
								Timeout: &types.Duration{
									Seconds: 5, // TODO: customize
								},
							},
							Sha256: cachedPlugin.Sha256,
						},
					},
				},
			},
		},
	}

	// TODO: allow customizing the stage
	stagedFilter, err := plugins.NewStagedFilterWithConfig(FilterName, filterCfg, plugins.DuringStage(plugins.AcceptedStage))
	if err != nil {
		return nil, err
	}

	return &stagedFilter, nil
}

func (p *Plugin) ensurePluginInCache(pc *wasm.PluginSource) (*CachedPlugin, error) {

	digest, err := imageCache.Add(context.TODO(), pc.Image)
	if err != nil {
		return nil, err
	}
	return &CachedPlugin{
		Sha256: strings.TrimPrefix(string(digest), "sha256:"),
	}, nil
}

func (p *Plugin) verifyConfiguration(schema Schema, config string) error {
	// everything goes now-a-days
	return nil
}

func (p *Plugin) HttpFilters(params plugins.Params, l *v1.HttpListener) ([]plugins.StagedHttpFilter, error) {
	if os.Getenv(WasmEnabled) == "" {
		contextutils.LoggerFrom(params.Ctx).Debugf("%s was not set, therefore not creating wasm config", WasmEnabled)
		return nil, nil
	}
	wasm := l.GetOptions().GetWasm()
	if wasm != nil {
		stagedPlugin, err := p.plugin(wasm)
		if err != nil {
			return nil, err
		}
		return []plugins.StagedHttpFilter{*stagedPlugin}, nil
	}
	return nil, nil
}

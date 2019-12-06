package wasm

import (
	"context"
	"net/http"
	"strings"
	"sync"

	"github.com/gogo/protobuf/types"
	"github.com/solo-io/extend-envoy/pkg/cache"
	"github.com/solo-io/extend-envoy/pkg/defaults"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/api/v2/config"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/wasm"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/go-utils/protoutils"
	"github.com/solo-io/solo-kit/pkg/api/external/envoy/api/v2/core"
)

const (
	FilterName       = "envoy.filters.http.wasm"
	V8Runtime        = "envoy.wasm.runtime.v8"
	WavmRuntime      = "envoy.wasm.runtime.wavm"
	VmId             = "gloo-vm-id"
	WasmCacheCluster = "wasm-cache"
)

type Plugin struct {
	// listenerEnabled map[*v1.HttpListener]bool
}

func NewPlugin() *Plugin {
	return &Plugin{
		// listenerEnabled: make(map[*v1.HttpListener]bool),
	}
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

	filterCfg := &config.WasmService{
		Config: &config.PluginConfig{
			Name:          pc.Name,
			RootId:        pc.RootId,
			Configuration: pc.Config,
			VmConfig: &config.VmConfig{
				VmId:    VmId,
				Runtime: WavmRuntime,
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

	strct, err := protoutils.MarshalStruct(filterCfg)
	if err != nil {
		return nil, err
	}
	// TODO: allow customizing the stage
	stagedFilter, err := plugins.NewStagedFilterWithConfig(FilterName, strct, plugins.DuringStage(plugins.AcceptedStage))
	if err != nil {
		return nil, err
	}

	return &stagedFilter, nil
}

var (
	imageCache cache.Cache
	once       sync.Once
)

func init() {
	imageCache = defaults.NewDefaultCache()
	go http.ListenAndServe(":9979", imageCache)
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

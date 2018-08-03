package plugins

import (
	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoylistener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	"github.com/envoyproxy/go-control-plane/pkg/server"
	"github.com/gogo/protobuf/types"

	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/pkg/control-plane/filewatcher"
	"github.com/solo-io/gloo/pkg/endpointdiscovery"
	"github.com/solo-io/gloo/pkg/secretwatcher"
)

type Stage int

const (
	PreInAuth  Stage = iota
	InAuth
	PostInAuth
	PreOutAuth
	OutAuth
)

type EnvoyNameForUpstream func(upstreamName string) string

type CommonParams struct {
	Listener *v1.Listener
}

type Dependencies struct {
	SecretRefs []string
	FileRefs   []string
}

type TranslatorPlugin interface {
	Init(options bootstrap.Options) error
}

type PluginWithDependencies interface {
	TranslatorPlugin
	GetDependencies(cfg *v1.Config) *Dependencies
}

// Parameters for ProcessUpstream()
type UpstreamPluginParams struct {
	CommonParams
	EnvoyNameForUpstream EnvoyNameForUpstream
	Secrets              secretwatcher.SecretMap
	Files                filewatcher.Files
}

type UpstreamPlugin interface {
	TranslatorPlugin
	ProcessUpstream(params *UpstreamPluginParams, in *v1.Upstream, out *envoyapi.Cluster) error
}

type EndpointDiscoveryPlugin interface {
	UpstreamPlugin
	SetupEndpointDiscovery() (endpointdiscovery.Interface, error)
}

// Params for ParseFunctionSpec()
type FunctionPluginParams struct {
	CommonParams
	UpstreamType string
	ServiceType  string
}

type FunctionPlugin interface {
	UpstreamPlugin
	// if the FunctionSpec does not belong to this plugin, return nil, nil
	// if the FunctionSpec belongs to this plugin but is not valid, return nil, err
	// if the FunctionSpec belongs to this plugin and is valid, return *Struct, nil
	ParseFunctionSpec(params *FunctionPluginParams, in v1.FunctionSpec) (*types.Struct, error)
}

// Params for ProcessRoute()
type RoutePluginParams struct {
	CommonParams
	EnvoyNameForUpstream EnvoyNameForUpstream
	// some route plugins need to know about the upstream(s) they route to
	Upstreams []*v1.Upstream
}

type RoutePlugin interface {
	TranslatorPlugin
	ProcessRoute(params *RoutePluginParams, in *v1.Route, out *envoyroute.Route) error
}

// Params for HttpFilters()
type HttpFilterPluginParams struct {
	CommonParams
}

type StagedHttpFilter struct {
	HttpFilter *envoyhttp.HttpFilter
	Stage      Stage
}

type HttpFilterPlugin interface {
	TranslatorPlugin
	HttpFilters(params *HttpFilterPluginParams) ([]StagedHttpFilter, error)
}

// Params for ListenerFilters()
type ListenerFilterPluginParams struct {
	CommonParams
	EnvoyNameForUpstream EnvoyNameForUpstream
	Config               *v1.Config
}

type StagedListenerFilter struct {
	ListenerFilter envoylistener.Filter
	Stage          Stage
}

// Plugins for creating network filters for listeners
type ListenerFilterPlugin interface {
	TranslatorPlugin
	ListenerFilters(params *ListenerFilterPluginParams, in *v1.Listener) ([]StagedListenerFilter, error)
}

// Plugins that create additional resources
type ClusterGeneratorPlugin interface {
	TranslatorPlugin
	GeneratedClusters(params *ClusterGeneratorPluginParams) ([]*envoyapi.Cluster, error)
}

// Params for GeneratedClusters()
type ClusterGeneratorPluginParams struct{}

type XdsPlugin interface {
	Callbacks() server.Callbacks
}

type XdsCallbacks []server.Callbacks

func (ps XdsCallbacks) OnStreamOpen(a int64, b string) {
	for _, cb := range ps {
		cb.OnStreamOpen(a, b)
	}
}
func (ps XdsCallbacks) OnStreamClosed(a int64) {
	for _, cb := range ps {
		cb.OnStreamClosed(a)
	}
}
func (ps XdsCallbacks) OnStreamRequest(a int64, b *envoyapi.DiscoveryRequest) {
	for _, cb := range ps {
		cb.OnStreamRequest(a, b)
	}
}
func (ps XdsCallbacks) OnStreamResponse(a int64, b *envoyapi.DiscoveryRequest, c *envoyapi.DiscoveryResponse) {
	for _, cb := range ps {
		cb.OnStreamResponse(a, b, c)
	}
}
func (ps XdsCallbacks) OnFetchRequest(a *envoyapi.DiscoveryRequest) {
	for _, cb := range ps {
		cb.OnFetchRequest(a)
	}
}
func (ps XdsCallbacks) OnFetchResponse(a *envoyapi.DiscoveryRequest, b *envoyapi.DiscoveryResponse) {
	for _, cb := range ps {
		cb.OnFetchResponse(a, b)
	}
}

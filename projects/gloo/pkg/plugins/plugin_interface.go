package plugins

import (
	"context"
	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoylistener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

type InitParams struct {
	Ctx                context.Context
	ExtensionsSettings *v1.Extensions
	Settings           *v1.Settings
}

type Plugin interface {
	Init(params InitParams) error
}

type Params struct {
	Ctx      context.Context
	Snapshot *v1.ApiSnapshot
}

type RouteParams struct {
	Params
	VirtualHost *v1.VirtualHost
}

/*
	Upstream Plugins
*/

type UpstreamPlugin interface {
	Plugin
	ProcessUpstream(params Params, in *v1.Upstream, out *envoyapi.Cluster) error
}

/*
	Routing Plugins
*/

type RoutePlugin interface {
	Plugin
	ProcessRoute(params RouteParams, in *v1.Route, out *envoyroute.Route) error
}

type RouteActionPlugin interface {
	Plugin
	ProcessRouteAction(params RouteParams, inAction *v1.RouteAction, inPlugins map[string]*RoutePlugin, out *envoyroute.RouteAction) error
}

/*
	Listener Plugins
*/

type ListenerPlugin interface {
	Plugin
	ProcessListener(params Params, in *v1.Listener, out *envoyapi.Listener) error
}

type ListenerFilterPlugin interface {
	Plugin
	ProcessListenerFilter(params Params, in *v1.Listener) ([]StagedListenerFilter, error)
}

type StagedListenerFilter struct {
	ListenerFilter envoylistener.Filter
	Stage          FilterStage
}

type HttpFilterPlugin interface {
	Plugin
	HttpFilters(params Params, listener *v1.HttpListener) ([]StagedHttpFilter, error)
}

type VirtualHostPlugin interface {
	Plugin
	ProcessVirtualHost(params Params, in *v1.VirtualHost, out *envoyroute.VirtualHost) error
}

type StagedHttpFilter struct {
	HttpFilter *envoyhttp.HttpFilter
	Stage      FilterStage
}

type FilterStage int

const (
	FaultFilter FilterStage = iota
	PreInAuth
	InAuth
	PostInAuth
	PreOutAuth
	OutAuth
)

/*
	Generation plugins
*/
type ClusterGeneratorPlugin interface {
	Plugin
	GeneratedClusters(params Params) ([]*envoyapi.Cluster, error)
}

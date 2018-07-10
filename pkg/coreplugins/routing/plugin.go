package routing

import (
	"fmt"

	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"

	"github.com/gogo/protobuf/types"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/plugins"
)

//go:generate protoc -I=./ -I=${GOPATH}/src/github.com/gogo/protobuf/ -I=${GOPATH}/src/github.com/gogo/protobuf/protobuf/ --gogo_out=Mgoogle/protobuf/duration.proto=github.com/gogo/protobuf/types:${GOPATH}/src route_extensions.proto

const (
	// TODO: add more retry policies
	serverFailurePolicy     = "5xx"
	connectionFailurePolicy = "connect-failure"
	defaultRetryPolicy      = serverFailurePolicy

	filterName  = "envoy.cors"
	pluginStage = plugins.InAuth
)

type Plugin struct {
	corsFilterNeeded bool
}

func (p *Plugin) GetDependencies(_ *v1.Config) *plugins.Dependencies {
	return nil
}

func (p *Plugin) ProcessRoute(_ *plugins.RoutePluginParams, in *v1.Route, out *envoyroute.Route) error {
	if in.Extensions == nil {
		return nil
	}
	spec, err := DecodeRouteExtensions(in.Extensions)
	if err != nil {
		return err
	}
	routeAction, ok := out.Action.(*envoyroute.Route_Route)
	// not a compatible route type
	if !ok {
		return nil
	}
	if routeAction.Route == nil {
		routeAction.Route = &envoyroute.RouteAction{}
	}
	for _, addH := range spec.AddRequestHeaders {
		routeAction.Route.RequestHeadersToAdd = append(routeAction.Route.RequestHeadersToAdd, &envoycore.HeaderValueOption{
			Header: &envoycore.HeaderValue{
				Key:   addH.Key,
				Value: addH.Value,
			},
			Append: &types.BoolValue{Value: addH.Append},
		})
	}
	for _, addH := range spec.AddResponseHeaders {
		routeAction.Route.ResponseHeadersToAdd = append(routeAction.Route.ResponseHeadersToAdd, &envoycore.HeaderValueOption{
			Header: &envoycore.HeaderValue{
				Key:   addH.Key,
				Value: addH.Value,
			},
			Append: &types.BoolValue{Value: addH.Append},
		})
	}
	routeAction.Route.ResponseHeadersToRemove = append(routeAction.Route.ResponseHeadersToRemove, spec.RemoveResponseHeaders...)

	if spec.Timeout != nil {
		routeAction.Route.Timeout = spec.Timeout
	}
	if spec.HostRewrite != "" {
		routeAction.Route.HostRewriteSpecifier = &envoyroute.RouteAction_HostRewrite{
			HostRewrite: spec.HostRewrite,
		}
	}
	if spec.MaxRetries > 0 {
		routeAction.Route.RetryPolicy = &envoyroute.RouteAction_RetryPolicy{
			RetryOn:    defaultRetryPolicy,
			NumRetries: &types.UInt32Value{Value: spec.MaxRetries},
		}
	}
	if spec.Cors != nil {
		p.corsFilterNeeded = true
		routeAction.Route.Cors = &envoyroute.CorsPolicy{
			AllowOrigin:      spec.Cors.AllowOrigin,
			AllowHeaders:     spec.Cors.AllowHeaders,
			AllowMethods:     spec.Cors.AllowMethods,
			ExposeHeaders:    spec.Cors.ExposeHeaders,
			AllowCredentials: &types.BoolValue{Value: spec.Cors.AllowCredentials},
		}
		if spec.Cors.MaxAge != nil {
			maxAge := fmt.Sprintf("%.0f", spec.Cors.MaxAge.Seconds())
			routeAction.Route.Cors.MaxAge = maxAge
		}
	}
	return nil
}

func (p *Plugin) HttpFilters(params *plugins.HttpFilterPluginParams) []plugins.StagedHttpFilter {
	defer func() { p.corsFilterNeeded = false }()

	if p.corsFilterNeeded {
		return []plugins.StagedHttpFilter{{
			HttpFilter: &envoyhttp.HttpFilter{Name: filterName}, Stage: pluginStage,
		}}
	}
	return nil
}

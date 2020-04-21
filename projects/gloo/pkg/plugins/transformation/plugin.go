package transformation

import (
	"context"

	udpa_type_v1 "github.com/cncf/udpa/go/udpa/type/v1"
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	envoy_config_bootstrap_v3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	v35 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	v34 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_extensions_filters_network_http_connection_manager_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/envoyproxy/go-control-plane/pkg/conversion"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/golang/protobuf/ptypes/duration"
	"github.com/solo-io/gloo/pkg/utils/protoutils"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/transformation"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/pluginutils"
)

const (
	FilterName = "io.solo.transformation"
)

var pluginStage = plugins.AfterStage(plugins.AuthZStage)

type Plugin struct {
	RequireTransformationFilter bool
}

func NewPlugin() *Plugin {
	return &Plugin{}
}

func (p *Plugin) Init(params plugins.InitParams) error {
	p.RequireTransformationFilter = false
	return nil
}

// TODO(yuval-k): We need to figure out what\if to do in edge cases where there is cluster weight transform
func (p *Plugin) ProcessVirtualHost(params plugins.VirtualHostParams, in *v1.VirtualHost, out *envoyroute.VirtualHost) error {
	transformations := in.GetOptions().GetTransformations()
	if transformations == nil {
		return nil
	}

	err := validateTransformation(params.Ctx, transformations)
	if err != nil {
		return err
	}

	p.RequireTransformationFilter = true
	return pluginutils.SetVhostPerFilterConfig(out, FilterName, transformations)
}

func (p *Plugin) ProcessRoute(params plugins.RouteParams, in *v1.Route, out *envoyroute.Route) error {
	transformations := in.GetOptions().GetTransformations()
	if transformations == nil {
		return nil
	}

	err := validateTransformation(params.Ctx, transformations)
	if err != nil {
		return err
	}

	p.RequireTransformationFilter = true
	return pluginutils.SetRoutePerFilterConfig(out, FilterName, transformations)
}

func (p *Plugin) ProcessWeightedDestination(params plugins.RouteParams, in *v1.WeightedDestination, out *envoyroute.WeightedCluster_ClusterWeight) error {
	transformations := in.GetOptions().GetTransformations()
	if transformations == nil {
		return nil
	}

	err := validateTransformation(params.Ctx, transformations)
	if err != nil {
		return err
	}

	p.RequireTransformationFilter = true
	return pluginutils.SetWeightedClusterPerFilterConfig(out, FilterName, transformations)
}

func (p *Plugin) HttpFilters(params plugins.Params, listener *v1.HttpListener) ([]plugins.StagedHttpFilter, error) {
	return []plugins.StagedHttpFilter{
		plugins.NewStagedFilter(FilterName, pluginStage),
	}, nil
}

func validateTransformation(ctx context.Context, transformations *transformation.RouteTransformations) error {
	err := bootstrap.ValidateBootstrap(ctx, buildBootstrap(transformations))
	if err != nil {
		return err
	}
	return nil
}

func buildBootstrap(transformations *transformation.RouteTransformations) string {

	configStruct, err := conversion.MessageToStruct(transformations)
	if err != nil {
		panic(err)
	}
	tAny := pluginutils.MustMessageToAny(transformations) //is gogoproto, no idea how to marshal with goproto

	// create a typed struct so goproto any can handle
	ts := &udpa_type_v1.TypedStruct{Value: configStruct, TypeUrl: tAny.TypeUrl}

	tAny2 := pluginutils.MustMessageToAny(ts)
	goAny := &any.Any{Value: tAny2.Value, TypeUrl: tAny2.TypeUrl}

	vhosts := []*envoy_config_route_v3.VirtualHost{
		{
			Name:    "placeholder_host",
			Domains: []string{"*"},
			Routes: []*envoy_config_route_v3.Route{
				{
					Action: &envoy_config_route_v3.Route_Route{Route: &envoy_config_route_v3.RouteAction{ClusterSpecifier: &envoy_config_route_v3.RouteAction_Cluster{Cluster: "placeholder_cluster"}}},
					Match: &envoy_config_route_v3.RouteMatch{
						PathSpecifier: &envoy_config_route_v3.RouteMatch_Prefix{Prefix: "/"},
					},
					TypedPerFilterConfig: map[string]*any.Any{
						FilterName: goAny,
					},
				},
			},
		},
	}

	rc3 := &envoy_config_route_v3.RouteConfiguration{VirtualHosts: vhosts}

	hcm := &envoy_extensions_filters_network_http_connection_manager_v3.HttpConnectionManager{
		StatPrefix:     "placeholder",
		RouteSpecifier: &envoy_extensions_filters_network_http_connection_manager_v3.HttpConnectionManager_RouteConfig{RouteConfig: rc3},
	}

	hcmAny := pluginutils.MustMessageToAny(hcm)
	bootstrap := &envoy_config_bootstrap_v3.Bootstrap{
		Node: &v3.Node{
			Id:      "imspecial",
			Cluster: "doesntmatter",
		},
		StaticResources: &envoy_config_bootstrap_v3.Bootstrap_StaticResources{
			Listeners: []*v34.Listener{
				{
					Name: "placeholder_listener",
					Address: &v3.Address{
						Address: &v3.Address_SocketAddress{SocketAddress: &v3.SocketAddress{
							Address:       "0.0.0.0",
							PortSpecifier: &v3.SocketAddress_PortValue{PortValue: 8081},
						}},
					},
					FilterChains: []*v34.FilterChain{
						{
							Name: "placeholder_filter_chain",
							Filters: []*v34.Filter{
								{
									ConfigType: &v34.Filter_TypedConfig{
										TypedConfig: hcmAny,
									},
									Name: "envoy.http_connection_manager",
								},
							},
						},
					},
				},
			},
			Clusters: []*v35.Cluster{
				{
					Name:           "placeholder_cluster",
					ConnectTimeout: &duration.Duration{Seconds: 5},
				},
			},
		},
	}

	b, _ := protoutils.MarshalBytes(bootstrap)
	re := string(b)

	return re
}

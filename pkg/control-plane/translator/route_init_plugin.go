package translator

import (
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/gogo/protobuf/types"
	"github.com/pkg/errors"

	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/plugins"

	"github.com/solo-io/gloo/pkg/protoutil"
)

const (
	filterName = "io.solo.function_router"
)

//go:generate protoc -I=${GOPATH}/src/github.com/solo-io/gloo/build-envoy/envoy-common/ -I=${GOPATH}/src/github.com/gogo/protobuf/protobuf/ --gogo_out=. ${GOPATH}/src/github.com/solo-io/gloo/build-envoy/envoy-common/functional_base.proto

type routeInitializerPlugin struct{}

func newRouteInitializerPlugin() *routeInitializerPlugin {
	return &routeInitializerPlugin{}
}

func (p *routeInitializerPlugin) GetDependencies(_ *v1.Config) *plugins.Dependencies {
	return nil
}

func (p *routeInitializerPlugin) ProcessRoute(_ *plugins.RoutePluginParams, in *v1.Route, out *envoyroute.Route) error {
	switch getDestinationType(in) {
	case destinationTypeSingleUpstream:
		processSingleUpstreamRoute(in.SingleDestination.DestinationType.(*v1.Destination_Upstream).Upstream.Name, in.PrefixRewrite, out)
		return nil
	case destinationTypeSingleFunction:
		processSingleFunctionRoute(in.SingleDestination.DestinationType.(*v1.Destination_Function).Function, in.PrefixRewrite, out)
		return nil
	case destinationTypeMultiple:
		processMultipleDestinationRoute(in.MultipleDestinations, in.PrefixRewrite, out)
		return nil
	}
	return errors.Errorf("invalid destination for function %#v | %#v", in.MultipleDestinations, in.SingleDestination)
}

type destinationType string

const (
	destinationTypeSingleUpstream = "single upstream"
	destinationTypeSingleFunction = "single function"
	destinationTypeMultiple       = "multiple upstreams or functions"
	//destinationTypeMultiFunction  = "multiple functions"
)

func getDestinationType(route *v1.Route) destinationType {
	if len(route.MultipleDestinations) > 0 {
		return destinationTypeMultiple
	}
	// invalid case, single destination must be set
	if route.SingleDestination == nil {
		return ""
	}
	switch route.SingleDestination.DestinationType.(type) {
	case *v1.Destination_Upstream:
		return destinationTypeSingleUpstream
	case *v1.Destination_Function:
		return destinationTypeSingleFunction
	}
	return ""
}

func processSingleUpstreamRoute(upstreamName, prefixRewrite string, out *envoyroute.Route) {
	initRouteForUpstream(upstreamName, prefixRewrite, out)
}

func processSingleFunctionRoute(destination *v1.FunctionDestination, prefixRewrite string, out *envoyroute.Route) {
	upstreamName := destination.UpstreamName
	initRouteForUpstream(upstreamName, prefixRewrite, out)

	if out.PerFilterConfig == nil {
		out.PerFilterConfig = map[string]*types.Struct{}
	}

	routeFunc, err := protoutil.MarshalStruct(&FunctionalFilterRouteConfig{FunctionName: destination.FunctionName})
	if err != nil {
		panic(err)
	}
	out.PerFilterConfig[filterName] = routeFunc
}

func processMultipleDestinationRoute(destinations []*v1.WeightedDestination, prefixRewrite string, out *envoyroute.Route) {
	var totalWeight uint32

	for _, destination := range destinations {
		totalWeight += destination.Weight

		var upstreamName string
		var funcname string
		switch dest := destination.DestinationType.(type) {
		case *v1.Destination_Function:
			upstreamName = dest.Function.UpstreamName
			funcname = dest.Function.FunctionName
			// if functional, add it to the functional destination list
		case *v1.Destination_Upstream:
			upstreamName = dest.Upstream.Name
		default:
			panic("TODO: handle when this type assert fails")
		}
		addWeightedCluster(upstreamName, funcname, destination.Weight, out)
	}

	setPrefixRewrite(prefixRewrite, out)
	setTotalWeight(totalWeight, out)
}

func setPrefixRewrite(prefixRewrite string, out *envoyroute.Route) {
	out.Action.(*envoyroute.Route_Route).Route.PrefixRewrite = prefixRewrite
}

func setTotalWeight(totalWeight uint32, out *envoyroute.Route) {
	weightedClusters := out.Action.(*envoyroute.Route_Route).Route.ClusterSpecifier.(*envoyroute.RouteAction_WeightedClusters)
	weightedClusters.WeightedClusters.TotalWeight = &types.UInt32Value{Value: totalWeight}
}

func addWeightedCluster(upstreamName, funcname string, weight uint32, out *envoyroute.Route) {
	clusterName := clusterName(upstreamName)
	weights := getWeightedClusters(out)
	clusterWeight := &envoyroute.WeightedCluster_ClusterWeight{
		Name:   clusterName,
		Weight: &types.UInt32Value{Value: weight},
	}
	if funcname != "" {

		routeFunc, err := protoutil.MarshalStruct(&FunctionalFilterRouteConfig{FunctionName: funcname})
		if err != nil {
			panic(err)
		}
		clusterWeight.PerFilterConfig = map[string]*types.Struct{
			filterName: routeFunc,
		}

	}

	weights.WeightedClusters.Clusters = append(weights.WeightedClusters.Clusters, clusterWeight)
}

func getWeightedClusters(out *envoyroute.Route) *envoyroute.RouteAction_WeightedClusters {
	// if route action is nil, just initialize it here
	if out.Action == nil {
		out.Action = &envoyroute.Route_Route{
			Route: &envoyroute.RouteAction{
				ClusterSpecifier: &envoyroute.RouteAction_WeightedClusters{
					WeightedClusters: &envoyroute.WeightedCluster{},
				},
			},
		}
	}

	// TODO: assess a way to deal with possible panics here
	// eventually we will need to support *Route_DirectResponse
	route, ok := out.Action.(*envoyroute.Route_Route)
	if !ok {
		panic("function router plugin unable to handle route action other than *Route_Route")
	}
	if route.Route == nil {
		route.Route = &envoyroute.RouteAction{}
	}
	if route.Route.ClusterSpecifier == nil {
		route.Route.ClusterSpecifier = &envoyroute.RouteAction_WeightedClusters{
			WeightedClusters: &envoyroute.WeightedCluster{},
		}
	}
	clusterSpecifier, ok := route.Route.ClusterSpecifier.(*envoyroute.RouteAction_WeightedClusters)
	if !ok {
		panic("function router plugin unable to handle Cluster Specifier other than *RouteAction_WeightedClusters")
	}
	if clusterSpecifier.WeightedClusters == nil {
		clusterSpecifier.WeightedClusters = &envoyroute.WeightedCluster{}
	}
	return clusterSpecifier
}

func initRouteForUpstream(upstreamName, prefixRewrite string, out *envoyroute.Route) {
	out.Action = &envoyroute.Route_Route{
		Route: &envoyroute.RouteAction{
			ClusterSpecifier: &envoyroute.RouteAction_Cluster{
				Cluster: clusterName(upstreamName),
			},
			PrefixRewrite: prefixRewrite,
		},
	}
}

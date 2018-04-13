package translator

import (
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/gogo/protobuf/types"
	"github.com/pkg/errors"

	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/coreplugins/common"
	"github.com/solo-io/gloo/pkg/plugins"
)

const (
	filterName                        = "io.solo.function_router"
	multiFunctionDestinationKey       = "weighted_functions"
	multiFunctionListDestinationKey   = "functions"
	multiFunctionWeightDestinationKey = "total_weight"
	singleFunctionDestinationKey      = "function"
)

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
	clusterName := clusterName(upstreamName)
	if out.Metadata == nil {
		out.Metadata = &envoycore.Metadata{}
	}
	functionalFilterMetadata := getFunctionalFilterMetadata(clusterName, out.Metadata)
	functionalFilterMetadata.Fields[singleFunctionDestinationKey] = &types.Value{Kind: &types.Value_StringValue{StringValue: destination.FunctionName}}
}

func processMultipleDestinationRoute(destinations []*v1.WeightedDestination, prefixRewrite string, out *envoyroute.Route) {
	var (
		totalWeight                   uint32
		upstreamDestinationsWithFuncs = make(map[string][]*v1.WeightedDestination)
		clusterWeights                = make(map[string]uint32)
	)
	for _, destination := range destinations {
		totalWeight += destination.Weight

		var upstreamName string
		switch dest := destination.DestinationType.(type) {
		case *v1.Destination_Function:
			upstreamName = dest.Function.UpstreamName
			// if functional, add it to the functional destination list
			upstreamDestinationsWithFuncs[upstreamName] = append(upstreamDestinationsWithFuncs[upstreamName], destination)
		case *v1.Destination_Upstream:
			upstreamName = dest.Upstream.Name
		default:
			panic("TODO: handle when this type assert fails")
		}
		clusterWeights[clusterName(upstreamName)] += uint32(destination.Weight)
	}
	// set weights for function routes
	for upstreamName, functionalDestinations := range upstreamDestinationsWithFuncs {
		addClusterFuncsToMetadata(clusterName(upstreamName), functionalDestinations, out)
	}
	// set weights for clusters (functional or non)
	for clusterName, weight := range clusterWeights {
		addWeightedCluster(clusterName, weight, out)
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

func addClusterFuncsToMetadata(clusterName string, destinations []*v1.WeightedDestination, out *envoyroute.Route) {
	var clusterFuncWeights []*types.Value
	var functionsWeight uint32 = 0
	for _, dest := range destinations {
		functionsWeight += dest.Weight
		clusterFuncWeight := &types.Value{
			Kind: &types.Value_StructValue{
				StructValue: &types.Struct{
					Fields: map[string]*types.Value{
						"name":   {Kind: &types.Value_StringValue{StringValue: dest.GetFunction().FunctionName}},
						"weight": {Kind: &types.Value_NumberValue{NumberValue: float64(dest.Weight)}},
					},
				},
			},
		}
		clusterFuncWeights = append(clusterFuncWeights, clusterFuncWeight)
	}
	if out.Metadata == nil {
		out.Metadata = &envoycore.Metadata{}
	}
	routeClusterMetadata := getFunctionalFilterMetadata(clusterName, out.Metadata)
	if routeClusterMetadata.Fields[multiFunctionDestinationKey] == nil {
		routeClusterMetadata.Fields[multiFunctionDestinationKey] = &types.Value{}
	}

	routeClusterMetadata.Fields[multiFunctionDestinationKey].Kind = &types.Value_StructValue{
		StructValue: &types.Struct{
			Fields: map[string]*types.Value{
				multiFunctionWeightDestinationKey: {Kind: &types.Value_NumberValue{NumberValue: float64(functionsWeight)}},
				multiFunctionListDestinationKey: {Kind: &types.Value_ListValue{
					ListValue: &types.ListValue{Values: clusterFuncWeights}}},
			},
		},
	}

}

func addWeightedCluster(clusterName string, weight uint32, out *envoyroute.Route) {
	weights := getWeightedClusters(out)
	clusterWeight := &envoyroute.WeightedCluster_ClusterWeight{
		Name:   clusterName,
		Weight: &types.UInt32Value{Value: weight},
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

func getFunctionalFilterMetadata(key string, meta *envoycore.Metadata) *types.Struct {
	initFunctionalFilterMetadata(key, meta)
	return meta.FilterMetadata[filterName].Fields[key].Kind.(*types.Value_StructValue).StructValue
}

// sets anything that might be nil so we don't get a nil pointer / map somewhere
func initFunctionalFilterMetadata(key string, meta *envoycore.Metadata) {
	filterMetadata := common.InitFilterMetadataField(filterName, key, meta)
	if filterMetadata.Kind == nil {
		filterMetadata.Kind = &types.Value_StructValue{}
	}
	_, isStructValue := filterMetadata.Kind.(*types.Value_StructValue)
	if !isStructValue {
		filterMetadata.Kind = &types.Value_StructValue{}
	}
	if filterMetadata.Kind.(*types.Value_StructValue).StructValue == nil {
		filterMetadata.Kind.(*types.Value_StructValue).StructValue = &types.Struct{}
	}
	if filterMetadata.Kind.(*types.Value_StructValue).StructValue.Fields == nil {
		filterMetadata.Kind.(*types.Value_StructValue).StructValue.Fields = make(map[string]*types.Value)
	}
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

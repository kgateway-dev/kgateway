package linkerd

import (
	"fmt"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/pluginutils"

	usconversions "github.com/solo-io/gloo/projects/gloo/pkg/upstreams"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/kubernetes"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
)

var (
	_ plugins.Plugin      = new(plugin)
	_ plugins.RoutePlugin = new(plugin)
)

const (
	ExtensionName = "linkerd"
	HeaderKey     = "l5d-dst-override"
)

type plugin struct {
	enabled bool
}

func NewPlugin() *plugin {
	return &plugin{}
}

func (p *plugin) Name() string {
	return ExtensionName
}

func (p *plugin) Init(params plugins.InitParams) {
	if settings := params.Settings; settings != nil {
		p.enabled = params.Settings.GetLinkerd()
	}
}

func (p *plugin) ProcessRoute(params plugins.RouteParams, in *v1.Route, out *envoy_config_route_v3.Route) error {
	if !p.enabled {
		return nil
	}
	routeAction := in.GetRouteAction()
	if routeAction == nil {
		return nil
	}

	upstreams := params.Snapshot.Upstreams
	upstreamGroups := params.Snapshot.UpstreamGroups

	switch destType := routeAction.GetDestination().(type) {
	case *v1.RouteAction_Single:

		upstreamRef, err := usconversions.DestinationToUpstreamRef(destType.Single)
		if err != nil {
			return err
		}

		us, err := upstreams.Find(upstreamRef.GetNamespace(), upstreamRef.GetName())
		if err != nil {
			return nil
		}
		kubeUs := us.GetKube()
		if kubeUs == nil {
			return nil
		}

		header := createHeaderForUpstream(kubeUs)
		headers := out.GetRequestHeadersToAdd()
		headers = append(headers, header)
		out.RequestHeadersToAdd = headers

	case *v1.RouteAction_Multi:
		destinations := destType.Multi.GetDestinations()
		err := configForMultiDestination(destinations, upstreams, out)
		if err != nil {
			return err
		}
	case *v1.RouteAction_UpstreamGroup:
		usg, err := upstreamGroups.Find(destType.UpstreamGroup.GetNamespace(), destType.UpstreamGroup.GetName())
		if err != nil {
			return pluginutils.NewUpstreamGroupNotFoundErr(*destType.UpstreamGroup)
		}
		err = configForMultiDestination(usg.GetDestinations(), upstreams, out)
		if err != nil {
			return err
		}
	default:
		return nil
	}
	return nil
}

func configForMultiDestination(
	destinations []*v1.WeightedDestination,
	upstreams v1.UpstreamList,
	out *envoy_config_route_v3.Route,
) error {
	routeAction := out.GetRoute()
	if routeAction == nil {
		return nil
	}

	weightedCluster := routeAction.GetWeightedClusters()
	if weightedCluster == nil {
		return nil
	}

	processedClusters := make(map[string]bool)

	for _, dest := range destinations {

		upstreamRef, err := usconversions.DestinationToUpstreamRef(dest.GetDestination())
		if err != nil {
			return err
		}

		us, err := upstreams.Find(upstreamRef.GetNamespace(), upstreamRef.GetName())
		if err != nil {
			continue
		}
		kubeUs := us.GetKube()
		if kubeUs == nil {
			continue
		}
		header := createHeaderForUpstream(kubeUs)
		clusterName := translator.UpstreamToClusterName(us.GetMetadata().Ref())
		clusters := findClustersForName(clusterName, weightedCluster.GetClusters())
		for _, cluster := range clusters {
			if _, ok := processedClusters[cluster.GetName()]; ok {
				continue
			}
			processedClusters[cluster.GetName()] = true
			headers := out.GetRequestHeadersToAdd()
			headers = append(headers, header)
			cluster.RequestHeadersToAdd = headers
		}
	}

	return nil
}

func findClustersForName(
	clusterName string,
	weightedCluster []*envoy_config_route_v3.WeightedCluster_ClusterWeight,
) []*envoy_config_route_v3.WeightedCluster_ClusterWeight {
	var result []*envoy_config_route_v3.WeightedCluster_ClusterWeight
	for _, v := range weightedCluster {
		if v.GetName() == clusterName {
			result = append(result, v)
		}
	}
	return result
}

func createHeaderForUpstream(us *kubernetes.UpstreamSpec) *envoy_config_core_v3.HeaderValueOption {
	destination := fmt.Sprintf("%s.%s.svc.cluster.local:%v",
		us.GetServiceName(), us.GetServiceNamespace(), us.GetServicePort())
	header := &envoy_config_core_v3.HeaderValueOption{
		Append: &wrappers.BoolValue{
			Value: false,
		},
		Header: &envoy_config_core_v3.HeaderValue{
			Value: destination,
			Key:   HeaderKey,
		},
	}
	return header
}

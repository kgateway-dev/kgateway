package linkerd

import (
	"fmt"

	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/pluginutils"

	usconversions "github.com/solo-io/gloo/projects/gloo/pkg/upstreams"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/gogo/protobuf/types"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/kubernetes"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
)

const (
	HeaderKey = "l5d-dst-override"
)

type Plugin struct {
	enabled bool
}

var _ plugins.Plugin = &Plugin{}

func NewPlugin() *Plugin {
	return &Plugin{}
}

func (p *Plugin) Init(params plugins.InitParams) error {
	if settings := params.Settings; settings != nil {
		p.enabled = params.Settings.Linkerd
	}
	return nil
}

func (p *Plugin) ProcessRoute(params plugins.RouteParams, in *v1.Route, out *envoyroute.Route) error {
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

		us, err := upstreams.Find(upstreamRef.Namespace, upstreamRef.Name)
		if err != nil {
			return nil
		}
		kubeUs := us.GetUpstreamSpec().GetKube()
		if kubeUs == nil {
			return nil
		}

		header := createHeaderForUpstream(kubeUs)
		headers := out.GetRequestHeadersToAdd()
		headers = append(headers, header)
		out.RequestHeadersToAdd = headers

	case *v1.RouteAction_Multi:
		destinations := destType.Multi.Destinations
		err := configForMultiDestination(destinations, upstreams, out)
		if err != nil {
			return err
		}
	case *v1.RouteAction_UpstreamGroup:
		usg, err := upstreamGroups.Find(destType.UpstreamGroup.Namespace, destType.UpstreamGroup.Name)
		if err != nil {
			return pluginutils.NewUpstreamGroupNotFoundErr(*destType.UpstreamGroup)
		}
		err = configForMultiDestination(usg.Destinations, upstreams, out)
		if err != nil {
			return err
		}
	default:
		return nil
	}
	return nil
}

func configForMultiDestination(destinations []*v1.WeightedDestination, upstreams v1.UpstreamList, out *envoyroute.Route) error {
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

		upstreamRef, err := usconversions.DestinationToUpstreamRef(dest.Destination)
		if err != nil {
			return err
		}

		us, err := upstreams.Find(upstreamRef.Namespace, upstreamRef.Name)
		if err != nil {
			continue
		}
		kubeUs := us.GetUpstreamSpec().GetKube()
		if kubeUs == nil {
			continue
		}
		header := createHeaderForUpstream(kubeUs)
		clusterName := translator.UpstreamToClusterName(us.GetMetadata().Ref())
		clusters := findClustersForName(clusterName, weightedCluster.Clusters)
		for _, cluster := range clusters {
			if _, ok := processedClusters[cluster.Name]; ok {
				continue
			}
			processedClusters[cluster.Name] = true
			headers := out.GetRequestHeadersToAdd()
			headers = append(headers, header)
			cluster.RequestHeadersToAdd = headers
		}
	}

	return nil
}

func findClustersForName(clusterName string, weightedCluster []*envoyroute.WeightedCluster_ClusterWeight) []*envoyroute.WeightedCluster_ClusterWeight {
	var result []*envoyroute.WeightedCluster_ClusterWeight
	for _, v := range weightedCluster {
		if v.Name == clusterName {
			result = append(result, v)
		}
	}
	return result
}

func createHeaderForUpstream(us *kubernetes.UpstreamSpec) *envoycore.HeaderValueOption {
	destination := fmt.Sprintf("%s.%s.svc.cluster.local:%v",
		us.ServiceName, us.ServiceNamespace, us.ServicePort)
	header := &envoycore.HeaderValueOption{
		Append: &types.BoolValue{
			Value: false,
		},
		Header: &envoycore.HeaderValue{
			Value: destination,
			Key:   HeaderKey,
		},
	}
	return header
}

func (p *Plugin) ProcessUpstream(params plugins.Params, in *v1.Upstream, out *envoyapi.Cluster) error {
	return nil
}

package upstream

import (
	"context"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoyclusters "github.com/envoyproxy/go-control-plane/envoy/extensions/clusters/dynamic_forward_proxy/v3"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/kgateway-dev/kgateway/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/internal/gateway2/ir"
)

func (p *plugin2) processDFPRoute(ctx context.Context, pCtx *ir.RouteContext, outputRoute *envoy_config_route_v3.Route) {
	for _, b := range pCtx.In.Backends {
		if u := b.Backend.Upstream; u != nil {
			if ir, ok := u.ObjIr.(*UpstreamIr); ok {
				if ir.dfpFilterConfig != nil {
					filters := p.neededDfpFilter[pCtx.FilterChainName]
					if _, ok := filters[u.ClusterName()]; !ok {
						filters[u.ClusterName()] = ir.dfpFilterConfig
						p.neededDfpFilter[pCtx.FilterChainName] = filters
					}
				}
			}
		}
	}
}

func processDFPCluster(ctx context.Context, in *v1alpha1.DynamicForwardProxy, out *envoy_config_cluster_v3.Cluster) {
	out.LbPolicy = envoy_config_cluster_v3.Cluster_CLUSTER_PROVIDED
	c := &envoyclusters.ClusterConfig{
		ClusterImplementationSpecifier: &envoyclusters.ClusterConfig_SubClustersConfig{
			SubClustersConfig: &envoyclusters.SubClustersConfig{
				LbPolicy: envoy_config_cluster_v3.Cluster_LEAST_REQUEST,
			},
		},
	}
	var anyCluster anypb.Any
	err := anypb.MarshalFrom(&anyCluster, c, proto.MarshalOptions{Deterministic: true})
	// error should never happen here. panic?
	if err != nil {
		panic(err)
	}

	// the upstream has a DNS name. We need Envoy to resolve the DNS name
	// set the type to strict dns
	out.ClusterDiscoveryType = &envoy_config_cluster_v3.Cluster_ClusterType{
		ClusterType: &envoy_config_cluster_v3.Cluster_CustomClusterType{
			Name:        "envoy.clusters.dynamic_forward_proxy",
			TypedConfig: &anyCluster,
		},
	}

}

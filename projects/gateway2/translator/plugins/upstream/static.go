package upstream

import (
	"context"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"github.com/solo-io/gloo/projects/gateway2/api/v1alpha1"
	"github.com/solo-io/gloo/projects/gateway2/krtcollections"
)

func processStatic(ctx context.Context, in *v1alpha1.StaticUpstream, out *envoy_config_cluster_v3.Cluster) {
}

func processEndpointsStatic(in *v1alpha1.StaticUpstream) *krtcollections.EndpointsForUpstream {
	return nil
}

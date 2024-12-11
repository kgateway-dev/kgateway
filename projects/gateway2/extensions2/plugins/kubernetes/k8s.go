package kubernetes

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"knative.dev/pkg/network"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"github.com/solo-io/gloo/projects/gateway2/extensions2/common"
	extensionsplug "github.com/solo-io/gloo/projects/gateway2/extensions2/plugin"
	"github.com/solo-io/gloo/projects/gateway2/ir"
	"github.com/solo-io/gloo/projects/gateway2/krtcollections"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
)

func NewPlugin(ctx context.Context, commoncol common.CommonCollections) extensionsplug.Plugin {
	serviceClient := kclient.New[*corev1.Service](commoncol.Client)
	services := krt.WrapClient(serviceClient, commoncol.KrtOpts.ToOptions("Services")...)

	gk := schema.GroupKind{
		Group: corev1.GroupName,
		Kind:  "Service",
	}

	clusterDomain := network.GetClusterDomainName()
	k8sServiceUpstreams := krt.NewManyCollection[*corev1.Service](services, func(kctx krt.HandlerContext, svc *corev1.Service) []ir.Upstream {
		uss := []ir.Upstream{}
		for _, port := range svc.Spec.Ports {
			uss = append(uss, ir.Upstream{
				ObjectSource: ir.ObjectSource{
					Kind:      gk.Kind,
					Group:     gk.Group,
					Namespace: svc.Namespace,
					Name:      svc.Name,
				},
				Obj:               svc,
				Port:              port.Port,
				CanonicalHostname: fmt.Sprintf("%s.%s.svc.%s", svc.Name, svc.Namespace, clusterDomain),
			})
		}
		return uss
	}, commoncol.KrtOpts.ToOptions("KubernetesServiceUpstreams")...)

	inputs := krtcollections.NewGlooK8sEndpointInputs(commoncol.Settings, commoncol.Client, commoncol.KrtOpts, commoncol.Pods, k8sServiceUpstreams)
	k8sServiceEndpoints := krtcollections.NewGlooK8sEndpoints(ctx, inputs)

	return extensionsplug.Plugin{
		ContributesUpstreams: map[schema.GroupKind]extensionsplug.UpstreamPlugin{
			gk: {
				UpstreamInit: ir.UpstreamInit{
					InitUpstream: processUpstream,
				},
				Endpoints: k8sServiceEndpoints,
				Upstreams: k8sServiceUpstreams,
			},
		},
	}
}

func processUpstream(ctx context.Context, in ir.Upstream, out *envoy_config_cluster_v3.Cluster) {

}

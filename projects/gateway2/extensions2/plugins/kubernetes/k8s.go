package kubernetes

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"knative.dev/pkg/network"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"github.com/solo-io/gloo/projects/gateway2/extensions2/common"
	extensionsplug "github.com/solo-io/gloo/projects/gateway2/extensions2/plugin"
	"github.com/solo-io/gloo/projects/gateway2/ir"
	"github.com/solo-io/gloo/projects/gateway2/krtcollections"
	"github.com/solo-io/gloo/projects/gateway2/utils/krtutil"
	glookubev1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/kube/apis/gloo.solo.io/v1"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
)

func NewPlugin(ctx context.Context, commoncol *common.CommonCollections) extensionsplug.Plugin {
	serviceClient := kclient.New[*corev1.Service](commoncol.Client)
	services := krt.WrapClient(serviceClient, commoncol.KrtOpts.ToOptions("Services")...)
	epSliceClient := kclient.New[*discoveryv1.EndpointSlice](commoncol.Client)
	endpointSlices := krt.WrapClient(epSliceClient, commoncol.KrtOpts.ToOptions("EndpointSlices")...)
	return NewPluginFromCollections(ctx, commoncol.KrtOpts, commoncol.Settings, commoncol.Pods, services, endpointSlices)
}

func NewPluginFromCollections(ctx context.Context, krtOpts krtutil.KrtOptions,
	settings krt.Singleton[glookubev1.Settings],
	pods krt.Collection[krtcollections.LocalityPod],
	services krt.Collection[*corev1.Service], endpointSlices krt.Collection[*discoveryv1.EndpointSlice]) extensionsplug.Plugin {
	gk := schema.GroupKind{
		Group: corev1.GroupName,
		Kind:  "Service",
	}

	clusterDomain := network.GetClusterDomainName()
	k8sServiceUpstreams := krt.NewManyCollection(services, func(kctx krt.HandlerContext, svc *corev1.Service) []ir.Upstream {
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
				GvPrefix:          "kube",
				CanonicalHostname: fmt.Sprintf("%s.%s.svc.%s", svc.Name, svc.Namespace, clusterDomain),
			})
		}
		return uss
	}, krtOpts.ToOptions("KubernetesServiceUpstreams")...)

	inputs := krtcollections.NewGlooK8sEndpointInputs(settings, krtOpts, endpointSlices, pods, k8sServiceUpstreams)
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
	out.ClusterDiscoveryType = &envoy_config_cluster_v3.Cluster_Type{
		Type: envoy_config_cluster_v3.Cluster_EDS,
	}
	out.EdsClusterConfig = &envoy_config_cluster_v3.Cluster_EdsClusterConfig{
		EdsConfig: &envoy_config_core_v3.ConfigSource{
			ResourceApiVersion: envoy_config_core_v3.ApiVersion_V3,
			ConfigSourceSpecifier: &envoy_config_core_v3.ConfigSource_Ads{
				Ads: &envoy_config_core_v3.AggregatedConfigSource{},
			},
		},
	}
}

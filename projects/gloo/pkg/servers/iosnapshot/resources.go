package iosnapshot

import (
	"slices"

	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway2/api/v1alpha1"
	"github.com/solo-io/gloo/projects/gateway2/wellknown"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	wellknownkube "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/kube/wellknown"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	KubernetesCoreGVKs = []schema.GroupVersionKind{
		wellknownkube.SecretGVK,
		wellknownkube.ConfigMapGVK,
	}

	GlooGVKs = []schema.GroupVersionKind{
		gloov1.SettingsGVK,
		gloov1.UpstreamGVK,
		gloov1.UpstreamGroupGVK,
		gloov1.ProxyGVK,
	}

	// PolicyGVKs is the set of GVKs that are used by a classic Gloo Gateway installation.
	// This is the common set of GVKs that are available when Edge Gateway APIs are being
	// used. See KubernetesGatewayIntegrationPolicyGVKs for the set of GVKs that are added
	// when the Kubernetes Gateway API is enabled
	PolicyGVKs = []schema.GroupVersionKind{
		gatewayv1.VirtualHostOptionGVK,
		gatewayv1.RouteOptionGVK,
	}

	EdgeGatewayGVKs = []schema.GroupVersionKind{
		gatewayv1.GatewayGVK,
		gatewayv1.MatchableHttpGatewayGVK,
		gatewayv1.MatchableTcpGatewayGVK,
		gatewayv1.VirtualServiceGVK,
		gatewayv1.RouteTableGVK,
	}

	KubernetesGatewayGVKs = []schema.GroupVersionKind{
		wellknown.GatewayClassGVK,
		wellknown.GatewayGVK,
		wellknown.HTTPRouteGVK,
		wellknown.ReferenceGrantGVK,
	}

	KubernetesGatewayIntegrationPolicyGVKs = []schema.GroupVersionKind{
		v1alpha1.GatewayParametersGVK,

		// While these are in fact Policy APIs, they are only enabled if the Kubernetes Gateway Integration is turned on
		gatewayv1.ListenerOptionGVK,
		gatewayv1.HttpListenerOptionGVK,
	}

	EdgeOnlyInputSnapshotGVKs = slices.Concat(
		KubernetesCoreGVKs,
		GlooGVKs,
		PolicyGVKs,
		EdgeGatewayGVKs,
	)

	// CompleteInputSnapshotGVKs is the list of GVKs that will be returned by the InputSnapshot API
	CompleteInputSnapshotGVKs = slices.Concat(
		EdgeOnlyInputSnapshotGVKs,
		KubernetesGatewayGVKs,
		KubernetesGatewayIntegrationPolicyGVKs,
	)
)

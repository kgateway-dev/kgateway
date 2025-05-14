package admin

import (
	"slices"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
)

// TODO: these need to be updated
var (
	KubernetesCoreGVKs = []schema.GroupVersionKind{
		wellknown.SecretGVK,
		wellknown.ConfigMapGVK,
	}

	// PolicyGVKs is the set of GVKs that are used by a classic Gloo Gateway installation.
	// This is the common set of GVKs that are available when Edge Gateway APIs are being
	// used. See KubernetesGatewayIntegrationPolicyGVKs for the set of GVKs that are added
	// when the Kubernetes Gateway API is enabled
	// PolicyGVKs = []schema.GroupVersionKind{
	// 	gatewayv1.VirtualHostOptionGVK,
	// 	gatewayv1.RouteOptionGVK,
	// }

	KubernetesGatewayGVKs = []schema.GroupVersionKind{
		wellknown.GatewayClassGVK,
		wellknown.GatewayGVK,
		wellknown.HTTPRouteGVK,
		wellknown.GRPCRouteGVK,
		wellknown.ReferenceGrantGVK,
	}

	KubernetesGatewayIntegrationPolicyGVKs = []schema.GroupVersionKind{
		wellknown.GatewayParametersGVK,

		// While these are in fact Policy APIs, they are only enabled if the Kubernetes Gateway Integration is turned on
		// gatewayv1.ListenerOptionGVK,
		// gatewayv1.HttpListenerOptionGVK,
	}

	// CompleteInputSnapshotGVKs is the list of GVKs that will be returned by the InputSnapshot API
	CompleteInputSnapshotGVKs = slices.Concat(
		KubernetesCoreGVKs,
		KubernetesGatewayGVKs,
		KubernetesGatewayIntegrationPolicyGVKs,
	)
)

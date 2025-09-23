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

	KubernetesGatewayGVKs = []schema.GroupVersionKind{
		wellknown.GatewayClassGVK,
		wellknown.GatewayGVK,
		wellknown.HTTPRouteGVK,
		wellknown.GRPCRouteGVK,
		wellknown.ReferenceGrantGVK,
	}

	KubernetesGatewayIntegrationPolicyGVKs = []schema.GroupVersionKind{
		wellknown.GatewayParametersGVK,
	}

	// CompleteInputSnapshotGVKs is the list of GVKs that will be returned by the InputSnapshot API
	CompleteInputSnapshotGVKs = slices.Concat(
		KubernetesCoreGVKs,
		KubernetesGatewayGVKs,
		KubernetesGatewayIntegrationPolicyGVKs,
	)
)

package iosnapshot

import (
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway2/wellknown"
	ratelimitv1alpha1 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/solo/ratelimit"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	extauthv1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
	wellknownkube "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/kube/wellknown"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"slices"
)

var (
	KubernetesGatewayGVKs = []schema.GroupVersionKind{
		wellknown.GatewayClassListGVK,
		wellknown.GatewayListGVK,
		wellknown.HTTPRouteListGVK,
		wellknown.ReferenceGrantListGVK,
	}

	KubernetesCoreGVKs = []schema.GroupVersionKind{
		wellknownkube.SecretGVK,
		wellknownkube.ConfigMapGVK,
	}

	EdgeGatewayGVKs = []schema.GroupVersionKind{
		// todo: add other edge types

		gloov1.SettingsGVK,
		gloov1.UpstreamGVK,
	}

	PolicyGVKs = []schema.GroupVersionKind{
		// Routing policies
		gatewayv1.ListenerOptionGVK,
		gatewayv1.HttpListenerOptionGVK,
		gatewayv1.VirtualHostOptionGVK,
		gatewayv1.RouteOptionGVK,

		// Extension service policies
		extauthv1.AuthConfigGVK,
		ratelimitv1alpha1.RateLimitConfigGVK,
		gloov1.UpstreamGVK,
		wellknownkube.SecretGVK,
	}

	// InputSnapshotGVKs is the list of GVKs that will be returned by the InputSnapshot API
	InputSnapshotGVKs = slices.Concat(
		KubernetesGatewayGVKs,
		KubernetesCoreGVKs,
		PolicyGVKs,
		EdgeGatewayGVKs,
	)
)

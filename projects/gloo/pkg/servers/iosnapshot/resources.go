package iosnapshot

import (
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway2/api/v1alpha1"
	"github.com/solo-io/gloo/projects/gateway2/wellknown"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	wellknownkube "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/kube/wellknown"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"slices"
)

var (
	KubernetesCoreGVKs = []schema.GroupVersionKind{
		wellknownkube.SecretGVK,
		wellknownkube.ConfigMapGVK,
	}

	GlooGVKs = []schema.GroupVersionKind{
		// todo: add other edge types

		gloov1.SettingsGVK,
		gloov1.UpstreamGVK,
		gloov1.ProxyGVK,
	}

	PolicyGVKs = []schema.GroupVersionKind{
		// Routing policies
		gatewayv1.ListenerOptionGVK,
		gatewayv1.HttpListenerOptionGVK,
		gatewayv1.VirtualHostOptionGVK,
		gatewayv1.RouteOptionGVK,

		// Extension service policies
		// todo: these should be EE only
		// extauthv1.AuthConfigGVK,
		// ratelimitv1alpha1.RateLimitConfigGVK,
	}

	KubernetesGatewayIntegrationGVKs = []schema.GroupVersionKind{
		wellknown.GatewayClassGVK,
		wellknown.GatewayGVK,
		wellknown.HTTPRouteGVK,
		wellknown.ReferenceGrantGVK,

		v1alpha1.GatewayParametersGVK,
	}

	// InputSnapshotGVKs is the list of GVKs that will be returned by the InputSnapshot API
	InputSnapshotGVKs = slices.Concat(
		KubernetesCoreGVKs,
		GlooGVKs,
		PolicyGVKs,
		KubernetesGatewayIntegrationGVKs,
	)
)

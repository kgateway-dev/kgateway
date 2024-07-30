package iosnapshot

import (
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway2/api/v1alpha1"
	"github.com/solo-io/gloo/projects/gateway2/wellknown"
	ratelimitv1alpha1 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/solo/ratelimit"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	extauthv1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
	wellknownkube "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/kube/wellknown"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	// KubeGatewayDefaultGVKs is the list of resource types to return in the kube gateway input snapshot
	KubeGatewayDefaultGVKs = []schema.GroupVersionKind{
		// Kubernetes Gateway API resources
		wellknown.GatewayClassListGVK,
		wellknown.GatewayListGVK,
		wellknown.HTTPRouteListGVK,
		wellknown.ReferenceGrantListGVK,

		// Gloo resources used in Kubernetes Gateway integration
		v1alpha1.GatewayParametersGVK,
		gatewayv1.ListenerOptionGVK,
		gatewayv1.HttpListenerOptionGVK,

		// resources shared between Edge and Kubernetes Gateway integration
		gatewayv1.RouteOptionGVK,
		gatewayv1.VirtualHostOptionGVK,
		extauthv1.AuthConfigGVK,
		ratelimitv1alpha1.RateLimitConfigGVK,
		gloov1.UpstreamGVK,
		wellknownkube.SecretGVK,
	}
)

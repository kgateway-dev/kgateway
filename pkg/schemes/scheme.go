package schemes

import (
	sologatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1/kube/apis/gateway.solo.io/v1"
	sologatewayv1alpha1 "github.com/solo-io/gloo/projects/gateway2/api/v1alpha1"
	extauthkubev1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1/kube/apis/enterprise.gloo.solo.io/v1"
	graphqlv1beta1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/graphql/v1beta1/kube/apis/graphql.gloo.solo.io/v1beta1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/kube/apis/gloo.solo.io/v1"
	ratelimitv1alpha1 "github.com/solo-io/solo-apis/pkg/api/ratelimit.solo.io/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	apiv1 "sigs.k8s.io/gateway-api/apis/v1"
	apiv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

// SchemeBuilder contains all the Schemes for registering the CRDs with which Gloo Gateway interacts.
// We share one SchemeBuilder as there's no harm in registering all I/O types internally.
var SchemeBuilder = runtime.SchemeBuilder{
	// K8s Gateway API resources
	apiv1.AddToScheme,
	apiv1beta1.AddToScheme,

	// Kubernetes Core resources
	corev1.AddToScheme,
	appsv1.AddToScheme,

	// Solo Kubernetes Gateway API resources
	sologatewayv1alpha1.AddToScheme,

	// Solo Edge Gateway API resources
	sologatewayv1.AddToScheme,

	// Solo Edge Gloo API resources
	gloov1.AddToScheme,

	// Enterprise Extensions
	// These are packed in the OSS Helm Chart, and therefore we register the schemes here as well
	graphqlv1beta1.AddToScheme,
	extauthkubev1.AddToScheme,
	ratelimitv1alpha1.AddToScheme,
}

func AddToScheme(s *runtime.Scheme) error {
	return SchemeBuilder.AddToScheme(s)
}

// DefaultScheme returns a scheme with all the types registered for Gloo Gateway
// We intentionally do not perform this operation in an init!!
// See https://github.com/solo-io/gloo/pull/9692 for context
func DefaultScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = AddToScheme(s)
	return s
}

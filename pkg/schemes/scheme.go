package schemes

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1b1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	sologatewayv1alpha1 "github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
)

// SchemeBuilder contains all the Schemes for registering the CRDs with which Gloo Gateway interacts.
// We share one SchemeBuilder as there's no harm in registering all I/O types internally.
var SchemeBuilder = runtime.SchemeBuilder{
	// K8s Gateway API resources
	gwv1.Install,
	gwv1b1.Install,

	// Kubernetes Core resources
	corev1.AddToScheme,
	appsv1.AddToScheme,
	discoveryv1.AddToScheme,

	// Register the apiextensions API group
	apiextensionsv1.AddToScheme,

	// Solo Kubernetes Gateway API resources
	sologatewayv1alpha1.AddToScheme,

	// Solo Edge Gloo API resources
	// gloov1.AddToScheme,

	// Enterprise Extensions
	// These are packed in the OSS Helm Chart, and therefore we register the schemes here as well
	// graphqlv1beta1.AddToScheme,
	// extauthkubev1.AddToScheme,
	// ratelimitv1alpha1.AddToScheme,
}

func AddToScheme(s *runtime.Scheme) error {
	return SchemeBuilder.AddToScheme(s)
}

// DefaultScheme returns a scheme with all the types registered for Gloo Gateway
// We intentionally do not perform this operation in an init!!
// See https://github.com/kgateway-dev/kgateway/pull/9692 for context
func DefaultScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = AddToScheme(s)
	return s
}

// GatewayScheme unconditionally includes the default and required Gateway API schemes.
// Use the Default scheme with AddGatewayV1A2Scheme to conditionally add the v1alpha2 scheme.
func GatewayScheme() *runtime.Scheme {
	s := DefaultScheme()
	if err := gwv1a2.Install(s); err != nil {
		panic(fmt.Sprintf("Failed to install gateway v1alpha2 scheme: %v", err))
	}
	if err := gwv1b1.Install(s); err != nil {
		panic(fmt.Sprintf("Failed to install gateway v1beta1 scheme: %v", err))
	}
	return s
}

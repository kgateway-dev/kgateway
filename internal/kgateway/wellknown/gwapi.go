package wellknown

import (
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	inf "sigs.k8s.io/gateway-api-inference-extension/api/v1"
	apiv1 "sigs.k8s.io/gateway-api/apis/v1"
	apiv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	apiv1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"
	apiv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
	apixv1alpha1 "sigs.k8s.io/gateway-api/apisx/v1alpha1"
)

const (
	// Group string for Gateway API resources
	GatewayGroup      = apiv1.GroupName
	XListenerSetGroup = apixv1alpha1.GroupName

	// Kind strings
	ServiceKind          = "Service"
	HTTPRouteKind        = "HTTPRoute"
	TCPRouteKind         = "TCPRoute"
	TLSRouteKind         = "TLSRoute"
	GRPCRouteKind        = "GRPCRoute"
	GatewayKind          = "Gateway"
	GatewayClassKind     = "GatewayClass"
	ReferenceGrantKind   = "ReferenceGrant"
	BackendTLSPolicyKind = "BackendTLSPolicy"

	// Kind string for XListenerSet resource
	XListenerSetKind = "XListenerSet"

	// Kind string for InferencePool resource
	InferencePoolKind = "InferencePool"

	// List Kind strings
	HTTPRouteListKind      = "HTTPRouteList"
	GatewayListKind        = "GatewayList"
	GatewayClassListKind   = "GatewayClassList"
	ReferenceGrantListKind = "ReferenceGrantList"

	// Gateway API CRD names
	TCPRouteCRDName = "tcproutes.gateway.networking.k8s.io"
)

var (
	GatewayGVK = schema.GroupVersionKind{
		Group:   GatewayGroup,
		Version: apiv1.GroupVersion.Version,
		Kind:    GatewayKind,
	}
	GatewayClassGVK = schema.GroupVersionKind{
		Group:   GatewayGroup,
		Version: apiv1.GroupVersion.Version,
		Kind:    GatewayClassKind,
	}
	HTTPRouteGVK = schema.GroupVersionKind{
		Group:   GatewayGroup,
		Version: apiv1.GroupVersion.Version,
		Kind:    HTTPRouteKind,
	}
	TLSRouteGVK = schema.GroupVersionKind{
		Group:   GatewayGroup,
		Version: apiv1alpha2.GroupVersion.Version,
		Kind:    TLSRouteKind,
	}
	TCPRouteGVK = schema.GroupVersionKind{
		Group:   GatewayGroup,
		Version: apiv1alpha2.GroupVersion.Version,
		Kind:    TCPRouteKind,
	}
	GRPCRouteGVK = schema.GroupVersionKind{
		Group:   GatewayGroup,
		Version: apiv1.GroupVersion.Version,
		Kind:    GRPCRouteKind,
	}
	ReferenceGrantGVK = schema.GroupVersionKind{
		Group:   GatewayGroup,
		Version: apiv1beta1.GroupVersion.Version,
		Kind:    ReferenceGrantKind,
	}
	BackendTLSPolicyGVK = schema.GroupVersionKind{
		Group:   GatewayGroup,
		Version: apiv1alpha3.GroupVersion.Version,
		Kind:    BackendTLSPolicyKind,
	}
	InferencePoolGVK = schema.GroupVersionKind{
		Group:   inf.GroupVersion.Group,
		Version: inf.GroupVersion.Version,
		Kind:    InferencePoolKind,
	}

	BackendTLSPolicyGVR = schema.GroupVersionResource{
		Group:    GatewayGroup,
		Version:  apiv1alpha3.GroupVersion.Version,
		Resource: "backendtlspolicies",
	}

	TCPRouteCRD = apiextv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: TCPRouteCRDName,
		},
	}

	XListenerSetGVK = schema.GroupVersionKind{
		Group:   XListenerSetGroup,
		Version: apixv1alpha1.GroupVersion.Version,
		Kind:    XListenerSetKind,
	}
	XListenerSetGVR = schema.GroupVersionResource{
		Group:    XListenerSetGroup,
		Version:  apixv1alpha1.GroupVersion.Version,
		Resource: "xlistenersets",
	}
)

// IsInferencePoolGK returns true if the given group and kind match
// the InferencePool Group, Version, and Kind.
func IsInferencePoolGK(group, kind string) bool {
	return InferencePoolGVK.Group == group &&
		InferencePoolGVK.Kind == kind
}

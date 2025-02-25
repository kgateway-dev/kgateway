package wellknown

import (
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	apiv1 "sigs.k8s.io/gateway-api/apis/v1"
	apiv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	apiv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

const (
	// Group string for Gateway API resources
	GatewayGroup = apiv1.GroupName

	// Kind string for k8s service
	ServiceKind = "Service"

	// Kind string for HTTPRoute resource
	HTTPRouteKind = "HTTPRoute"

	// Kind string for TCPRoute resource
	TCPRouteKind = "TCPRoute"

	// Kind string for Gateway resource
	GatewayKind = "Gateway"

	// Kind string for GatewayClass resource
	GatewayClassKind = "GatewayClass"

	// Kind string for ReferenceGrant resource
	ReferenceGrantKind = "ReferenceGrant"

	// Kind string for InferencePool resource
	InferencePoolKind = "InferencePool"

	// Kind strings for Gateway API list types
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
	ReferenceGrantGVK = schema.GroupVersionKind{
		Group:   GatewayGroup,
		Version: apiv1beta1.GroupVersion.Version,
		Kind:    ReferenceGrantKind,
	}

	GatewayListGVK = schema.GroupVersionKind{
		Group:   GatewayGroup,
		Version: apiv1.GroupVersion.Version,
		Kind:    GatewayListKind,
	}
	GatewayClassListGVK = schema.GroupVersionKind{
		Group:   GatewayGroup,
		Version: apiv1.GroupVersion.Version,
		Kind:    GatewayClassListKind,
	}
	HTTPRouteListGVK = schema.GroupVersionKind{
		Group:   GatewayGroup,
		Version: apiv1.GroupVersion.Version,
		Kind:    HTTPRouteListKind,
	}
	HTCPRouteListGVK = schema.GroupVersionKind{
		Group:   GatewayGroup,
		Version: apiv1alpha2.GroupVersion.Version,
		Kind:    HTTPRouteListKind,
	}
	ReferenceGrantListGVK = schema.GroupVersionKind{
		Group:   GatewayGroup,
		Version: apiv1beta1.GroupVersion.Version,
		Kind:    ReferenceGrantListKind,
	}

	// GatewayStandardCRDs defines the set of Gateway API CRDs from the standard release channel.
	GatewayStandardCRDs = sets.New[string](
		"gatewayclasses.gateway.networking.k8s.io",
		"gateways.gateway.networking.k8s.io",
		"httproutes.gateway.networking.k8s.io",
		"grpcroutes.gateway.networking.k8s.io",
		"referencegrants.gateway.networking.k8s.io",
	)

	TCPRouteCRD = apiextv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: TCPRouteCRDName,
		},
	}
)

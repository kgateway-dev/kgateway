package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (

	// GatewayParametersKind is the kind for the GatewayParameters CRD.
	GatewayParametersKind = "GatewayParameters"
	// DirectResponseKind is the kind for the DirectResponse CRD.
	DirectResponseKind     = "DirectResponse"
	UpstreamKind           = "Upstream"
	RoutePolicyKind        = "RoutePolicy"
	ListenerPolicyKind     = "ListenerPolicy"
	HTTPListenerPolicyKind = "HTTPListenerPolicy"
)

var (
	GatewayParametersGVK = schema.GroupVersionKind{
		Group:   GroupName,
		Version: GroupVersion.Version,
		Kind:    GatewayParametersKind,
	}

	DirectResponseGVK = schema.GroupVersionKind{
		Group:   GroupName,
		Version: GroupVersion.Version,
		Kind:    DirectResponseKind,
	}
	UpstreamGVK = schema.GroupVersionKind{
		Group:   GroupName,
		Version: GroupVersion.Version,
		Kind:    UpstreamKind,
	}
	RoutePolicyGVK = schema.GroupVersionKind{
		Group:   GroupName,
		Version: GroupVersion.Version,
		Kind:    RoutePolicyKind,
	}
	ListenerPolicyGVK = schema.GroupVersionKind{
		Group:   GroupName,
		Version: GroupVersion.Version,
		Kind:    ListenerPolicyKind,
	}
	HTTPListenerPolicyGVK = schema.GroupVersionKind{
		Group:   GroupName,
		Version: GroupVersion.Version,
		Kind:    HTTPListenerPolicyKind,
	}
)

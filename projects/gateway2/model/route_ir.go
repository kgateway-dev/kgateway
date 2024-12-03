package model

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type Policy any
type Policies []Policy

type HttpRouteIR struct {
	metav1.ObjectMeta
	ParentRefs       []gwv1.ParentReference
	Hostnames        []string
	AttachedPolicies map[string]Policies
	Rules            []HttpRouteRuleIR
}

type HttpRouteRuleIR struct {
	gwv1.HTTPRouteRule
	ExtensionRefs    map[string]Policies
	AttachedPolicies map[string]Policies
}

type ListenerIR struct {
	Name             string
	BindAddress      string
	BindPort         uint32
	AttachedPolicies map[string]Policies

	HttpFilterChain []HttpFilterChainIR
	Tcp             []TcpIR
}

type VirtualHost struct {
	Hostnames []string
	Rules     []HttpRouteRuleIR
}

type FilterChainMatch struct {
	ServerName string
}

type HttpFilterChainIR struct {
	Matcher          FilterChainMatch
	Vhosts           []*VirtualHost
	AttachedPolicies map[string]Policies
}

type TcpIR struct {
	Matcher     FilterChainMatch
	BackendRefs []gwv1.BackendRef
}

type GatewayIR struct {
	Listeners []ListenerIR

	AttachedPolicies map[string]Policies
}

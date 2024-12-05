package model

import (
	"fmt"

	"istio.io/istio/pkg/kube/krt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type AttachmentPoints int

const (
	HttpAttachmentPoint AttachmentPoints = iota
	ListenerAttachmentPoint
)

type PolicyTargetRef struct {
	Group       string
	Kind        string
	Name        string
	SectionName string
}

type PolicyWrapper struct {
	GK schema.GroupKind
	// Errors processing it for status.
	// note: these errors are based on policy itself, regardless of whether it's attached to a resource.
	// TODO: change for conditions
	Errors []error

	// original object. ideally with structural errors removed.
	// Opaque to us other than metadata.
	Obj metav1.Object

	TargetRefs []PolicyTargetRef
}

func (c PolicyWrapper) ResourceName() string {
	return fmt.Sprintf("%s/%s/%s/%s", c.GK.Group, c.GK.Kind, c.Obj.GetNamespace(), c.Obj.GetName())
}

func (c PolicyWrapper) Equals(in PolicyWrapper) bool {
	var versionEquals bool
	if c.Obj.GetGeneration() != 0 && in.Obj.GetGeneration() != 0 {
		versionEquals = c.Obj.GetGeneration() == in.Obj.GetGeneration()
	} else {
		versionEquals = c.Obj.GetResourceVersion() == in.Obj.GetResourceVersion()
	}

	return versionEquals && c.Obj.GetUID() == in.Obj.GetUID()
}

type Policy interface {
	Obj() metav1.Object
}

type Policies []Policy

//type AttachedPolicies map[string]Policies

type HttpPolicy Policy
type ListenerPolicy Policy

type AttachedPolicies[P Policy] struct {
	Policies map[schema.GroupKind][]P
}

type Backend struct {
	//Upstream    Upstream
	ClusterName string
	Weight      uint32
}

type HttpBackend struct {
	Backend
	AttachedPolicies[HttpPolicy]
}

type HttpRouteIR struct {
	SourceObject     client.Object
	ParentRefs       []gwv1.ParentReference
	Hostnames        []string
	AttachedPolicies AttachedPolicies[HttpPolicy]
	Rules            []HttpRouteRuleIR
}

type HttpRouteRuleIR struct {
	gwv1.HTTPRouteRule
	Parent           HttpRouteIR
	ExtensionRefs    AttachedPolicies[HttpPolicy]
	AttachedPolicies AttachedPolicies[HttpPolicy]

	Backends []HttpBackend
}

type ListenerIR struct {
	Name             string
	BindAddress      string
	BindPort         uint32
	AttachedPolicies AttachedPolicies[HttpPolicy]

	HttpFilterChain []HttpFilterChainIR
	Tcp             []TcpIR
}

type VirtualHost struct {
	Name      string
	Hostnames []string
	Rules     []HttpRouteRuleIR
}

type FilterChainMatch struct {
	ServerName string
}
type FitlerChainCommon struct {
	Matcher         FilterChainMatch
	FilterChainName string
	ParentRef       gwv1.ParentReference
}

type HttpFilterChainIR struct {
	FitlerChainCommon
	Vhosts           []*VirtualHost
	ParentRef        gwv1.ParentReference
	AttachedPolicies AttachedPolicies[HttpPolicy]
}

type TcpIR struct {
	FitlerChainCommon
	BackendRefs []Backend
}

// this is 1:1 with envoy deployments
type GatewayIR struct {
	Listeners    []ListenerIR
	SourceObject *gwv1.Gateway

	AttachedPolicies     AttachedPolicies[ListenerPolicy]
	AttachedHttpPolicies AttachedPolicies[HttpPolicy]
}

type Extension struct {
	ContributedUpstreams map[schema.GroupKind]krt.Collection[Upstream]
}

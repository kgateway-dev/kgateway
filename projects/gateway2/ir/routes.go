package ir

import (
	"istio.io/istio/pkg/kube/krt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

type Route interface {
	GetGroupKind() schema.GroupKind
	// GetName returns the name of the route.
	GetName() string
	// GetNamespace returns the namespace of the route.
	GetNamespace() string

	GetParentRefs() []gwv1.ParentReference
	GetSourceObject() metav1.Object
}

// this is 1:1 with httproute, and is a krt type
// maybe move this to krtcollections package?
type HttpRouteIR struct {
	ObjectSource `json:",inline"`
	SourceObject metav1.Object
	ParentRefs   []gwv1.ParentReference

	Hostnames        []string
	AttachedPolicies AttachedPolicies
	Rules            []HttpRouteRuleIR
}

func (c *HttpRouteIR) GetParentRefs() []gwv1.ParentReference {
	return c.ParentRefs
}
func (c *HttpRouteIR) GetSourceObject() metav1.Object {
	return c.SourceObject
}

func (c HttpRouteIR) ResourceName() string {
	return c.ObjectSource.ResourceName()
}

var _ krt.ResourceNamer = &HttpRouteIR{}
var _ krt.ResourceNamer = HttpRouteIR{}

func (c HttpRouteIR) Equals(in HttpRouteIR) bool {
	// TODO: equals should take the attached policies to account too!
	// as backends resolution may change when they are added/remove we need to check equality for them as well
	// we don't need to check the whole backend, just the cluster name (that may swap in and out of black-hole)
	// note - if we stop setting cluster to black whole here (and always set it to the expect cluster name) we can remove the backend equality check.
	return c.ObjectSource == in.ObjectSource && versionEquals(c.SourceObject, in.SourceObject) && c.AttachedPolicies.Equals(in.AttachedPolicies) && c.backendsEqual(in)
}
func (c HttpRouteIR) backendsEqual(in HttpRouteIR) bool {
	if len(c.Rules) != len(in.Rules) {
		return false
	}
	for i, rule := range c.Rules {
		backendsa := rule.Backends
		backendsb := in.Rules[i].Backends
		if len(backendsa) != len(backendsb) {
			return false
		}
		for j, backend := range backendsa {
			if backend.Backend == nil && backendsb[j].Backend == nil {
				continue
			}
			if backend.Backend != nil && backendsb[j].Backend != nil {
				if backend.Backend.ClusterName != backendsb[j].Backend.ClusterName {
					return false
				}
			} else {
				return false
			}
		}
	}
	return true
}

var _ Route = &HttpRouteIR{}

type TcpRouteIR struct {
	ObjectSource     `json:",inline"`
	SourceObject     *gwv1alpha2.TCPRoute
	ParentRefs       []gwv1.ParentReference
	AttachedPolicies AttachedPolicies
	Backends         []Backend
}

func (c *TcpRouteIR) GetParentRefs() []gwv1.ParentReference {
	return c.ParentRefs
}
func (c *TcpRouteIR) GetSourceObject() metav1.Object {
	return c.SourceObject
}
func (c TcpRouteIR) ResourceName() string {
	return c.ObjectSource.ResourceName()
}

func (c TcpRouteIR) Equals(in TcpRouteIR) bool {
	return c.ObjectSource == in.ObjectSource && versionEquals(c.SourceObject, in.SourceObject) && c.AttachedPolicies.Equals(in.AttachedPolicies)
}

var _ Route = &TcpRouteIR{}

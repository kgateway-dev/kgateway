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

	// PrecedenceWeight specifies the weight of this route relative to other route.
	// Higher weight means higher priority, and are evaluated before routes with lower weight
	PrecedenceWeight int32

	// DelegationInheritParentMatcher indicates if the route should inherit the parent matcher
	// from the parent route delegating to it
	DelegationInheritParentMatcher bool
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

// get hostnames
func (c *HttpRouteIR) GetHostnames() []string {
	if c == nil {
		return nil
	}
	return c.Hostnames
}

var (
	_ krt.ResourceNamer = &HttpRouteIR{}
	_ krt.ResourceNamer = HttpRouteIR{}
)

func (c HttpRouteIR) Equals(in HttpRouteIR) bool {
	// as backends resolution may change when they are added/remove we need to check equality for them as well
	// we don't need to check the whole backend, just the cluster name (that may swap in and out of black-hole)
	// note - if we stop setting cluster to black whole here (and always set it to the expect cluster name) we can remove the backend equality check.
	return c.ObjectSource == in.ObjectSource &&
		versionEquals(c.SourceObject, in.SourceObject) &&
		c.AttachedPolicies.Equals(in.AttachedPolicies) &&
		c.rulesEqual(in) &&
		c.PrecedenceWeight == in.PrecedenceWeight &&
		c.DelegationInheritParentMatcher == in.DelegationInheritParentMatcher
}

func (c HttpRouteIR) rulesEqual(in HttpRouteIR) bool {
	// we don't need to check the rules themselves as this is covered by versionEquals.
	// we do need to check backends and policies
	// we also need to check the error on rule
	if len(c.Rules) != len(in.Rules) {
		return false
	}
	for i, rule := range c.Rules {
		if !rule.AttachedPolicies.Equals(in.Rules[i].AttachedPolicies) {
			return false
		}
		if !rule.ExtensionRefs.Equals(in.Rules[i].ExtensionRefs) {
			return false
		}
		backendsa := rule.Backends
		backendsb := in.Rules[i].Backends
		if len(backendsa) != len(backendsb) {
			return false
		}
		for j, backend := range backendsa {
			otherbackend := backendsb[j]
			if backend.Backend == nil && otherbackend.Backend == nil {
				continue
			}
			if backend.Backend != nil && otherbackend.Backend != nil {
				if backend.Backend.ClusterName != otherbackend.Backend.ClusterName {
					return false
				}
				if backend.Backend.Weight != otherbackend.Backend.Weight {
					return false
				}
				if !backend.AttachedPolicies.Equals(otherbackend.AttachedPolicies) {
					return false
				}
			} else {
				return false
			}
		}
		e1 := rule.Err
		e2 := in.Rules[i].Err
		if e1 == nil && e2 != nil {
			return false
		}
		if e1 != nil && e2 == nil {
			return false
		}
		if (e1 != nil && e2 != nil) && e1.Error() != e2.Error() {
			return false
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
	Backends         []BackendRefIR
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
	return c.ObjectSource == in.ObjectSource &&
		versionEquals(c.SourceObject, in.SourceObject) &&
		c.AttachedPolicies.Equals(in.AttachedPolicies) &&
		backendsEqual(c.Backends, in.Backends)
}

// backendsEqual compares two slices of BackendRefIR using the Equals method for readability.
func backendsEqual(a, b []BackendRefIR) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !a[i].Equals(b[i]) {
			return false
		}
	}
	return true
}

var _ Route = &TcpRouteIR{}

type TlsRouteIR struct {
	ObjectSource `json:",inline"`
	SourceObject *gwv1alpha2.TLSRoute
	ParentRefs   []gwv1.ParentReference

	Hostnames        []string
	AttachedPolicies AttachedPolicies
	Backends         []BackendRefIR
}

func (c *TlsRouteIR) GetParentRefs() []gwv1.ParentReference {
	return c.ParentRefs
}

func (c *TlsRouteIR) GetSourceObject() metav1.Object {
	return c.SourceObject
}

func (c TlsRouteIR) ResourceName() string {
	return c.ObjectSource.ResourceName()
}

func (c TlsRouteIR) Equals(in TlsRouteIR) bool {
	return c.ObjectSource == in.ObjectSource &&
		versionEquals(c.SourceObject, in.SourceObject) &&
		c.AttachedPolicies.Equals(in.AttachedPolicies) &&
		backendsEqual(c.Backends, in.Backends)
}

func (c *TlsRouteIR) GetHostnames() []string {
	if c == nil {
		return nil
	}
	return c.Hostnames
}

var _ Route = &TlsRouteIR{}

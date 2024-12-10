package ir

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type ObjectSource struct {
	Group     string `json:"group,omitempty"`
	Kind      string `json:"kind,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name"`
}

// GetKind returns the kind of the route.
func (c ObjectSource) GetGroupKind() schema.GroupKind {
	return schema.GroupKind{
		Group: c.Group,
		Kind:  c.Kind,
	}
}

// GetName returns the name of the route.
func (c ObjectSource) GetName() string {
	return c.Name
}

// GetNamespace returns the namespace of the route.
func (c ObjectSource) GetNamespace() string {
	return c.Namespace
}
func (c ObjectSource) ResourceName() string {
	return fmt.Sprintf("%s/%s/%s/%s", c.Group, c.Kind, c.Namespace, c.Name)
}

func (c ObjectSource) String() string {
	return fmt.Sprintf("%s/%s/%s/%s", c.Group, c.Kind, c.Namespace, c.Name)
}

func (c ObjectSource) Equals(in ObjectSource) bool {
	return c.Namespace == in.Namespace && c.Name == in.Name && c.Group == in.Group && c.Kind == in.Kind
}

type Upstream struct {
	// Ref to source object. sometimes the group and kind are not populated from api-server, so
	// set them explicitly here, and pass this around as the reference.
	ObjectSource `json:",inline"`
	// optional port for if ObjectSource is a service that can have multiple ports.
	Port int32

	// prefix the cluster name with this string to distringuish it from other GVKs.
	// here explicitly as it shows up in stats. each (group, kind) pair should have a unique prefix.
	GvPrefix string
	// for things that integrate with destination rule, we need to know what hostname to use.
	CanonicalHostname string
	// original object. Opaque to us other than metadata.
	Obj metav1.Object

	// can this just be any?
	// i think so, assuming obj -> objir is a 1:1 mapping.
	ObjIr interface{ Equals(any) bool }

	AttachedPolicies AttachedPolicies
}

func (c Upstream) ResourceName() string {
	return fmt.Sprintf("%s:%d", c.ObjectSource.ResourceName(), c.Port)
}

func (c Upstream) Equals(in Upstream) bool {
	return c.ObjectSource.Equals(in.ObjectSource) && versionEquals(c.Obj, in.Obj)

}

type Secret struct {
	// Ref to source object. sometimes the group and kind are not populated from api-server, so
	// set them explicitly here, and pass this around as the reference.
	ObjectSource `json:",inline"`

	// original object. Opaque to us other than metadata.
	Obj metav1.Object

	Data map[string][]byte
}

func (c Secret) ResourceName() string {
	return c.ObjectSource.ResourceName()
}

func (c Secret) Equals(in Secret) bool {
	return c.ObjectSource.Equals(in.ObjectSource) && versionEquals(c.Obj, in.Obj)
}

type Listener struct {
	gwv1.Listener
	AttachedPolicies AttachedPolicies
}

type Gateway struct {
	ObjectSource `json:",inline"`
	Listeners    []Listener
	Obj          *gwv1.Gateway

	AttachedListenerPolicies AttachedPolicies
	AttachedHttpPolicies     AttachedPolicies
}

func (c Gateway) ResourceName() string {
	return c.ObjectSource.ResourceName()
}

func (c Gateway) Equals(in Gateway) bool {
	return c.ObjectSource.Equals(in.ObjectSource) && versionEquals(c.Obj, in.Obj)
}

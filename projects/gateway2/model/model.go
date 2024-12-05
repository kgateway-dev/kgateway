package model

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ObjectSource struct {
	Group     string `json:"group,omitempty"`
	Kind      string `json:"kind,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name"`
}

func (c ObjectSource) ResourceName() string {
	return fmt.Sprintf("%s/%s/%s/%s", c.Group, c.Kind, c.Namespace, c.Name)
}

func (c ObjectSource) Equals(in ObjectSource) bool {
	return c.Namespace == in.Namespace && c.Name == in.Name && c.Group == in.Group && c.Kind == in.Kind
}

type Upstream struct {
	// Ref to source object. sometimes the group and kind are not populated from api-server, so
	// set them explicitly here, and pass this around as the reference.
	ObjectSource `json:",inline"`

	// prefix the cluster name with this string to distringuish it from other GVKs.
	// here explicitly as it shows up in stats. each (group, kind) pair should have a unique prefix.
	GvPrefix string
	// for things that integrate with destination rule, we need to know what hostname to use.
	CanonicalHostname string
	// original object. Opaque to us other than metadata.
	Obj metav1.Object
}

func (c Upstream) ResourceName() string {
	return c.ObjectSource.ResourceName()
}

func (c Upstream) Equals(in Upstream) bool {
	var versionEquals bool
	if c.Obj.GetGeneration() != 0 && in.Obj.GetGeneration() != 0 {
		versionEquals = c.Obj.GetGeneration() == in.Obj.GetGeneration()
	} else {
		versionEquals = c.Obj.GetResourceVersion() == in.Obj.GetResourceVersion()
	}

	return c.ObjectSource.Equals(in.ObjectSource) &&
		versionEquals && c.Obj.GetUID() == in.Obj.GetUID()
}

/*
translate:
initialize cluster

upstreamsToMutation collection
upstramMutation()

apply mutators?

upstream plguin.Mutate(kctx, ucc, upstream, cluster)
--------------------------
eps:
haveEpPlugin?
  cla, version := getEndpoints()
*/

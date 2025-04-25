package ir

import (
	"context"
	"encoding/json"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type BackendInit struct {
	InitBackend func(ctx context.Context, in BackendObjectIR, out *envoy_config_cluster_v3.Cluster)
}

type PolicyRef struct {
	Group       string
	Kind        string
	Name        string
	SectionName string
}

type AttachedPolicyRef struct {
	Group string
	Kind  string
	Name  string
	// policies are local namespace only, but we need this here for usage when
	// processing attached policy reports
	Namespace   string
	SectionName string
}

func (ref *AttachedPolicyRef) ID() string {
	return ref.Group + "/" + ref.Kind + "/" + ref.Namespace + "/" + ref.Name
}

type PolicyAtt struct {
	// GroupKind is the GK of the original policy object
	GroupKind schema.GroupKind
	// original object. ideally with structural errors removed.
	// Opaque to us other than metadata.
	PolicyIr PolicyIR

	// PolicyRef is a ref to the original policy that is attached (can be used to report status correctly).
	// nil if the attachment was done via extension ref
	PolicyRef *AttachedPolicyRef

	Errors []error
}

func (c PolicyAtt) Obj() PolicyIR {
	return c.PolicyIr
}

func (c PolicyAtt) TargetRef() *AttachedPolicyRef {
	return c.PolicyRef
}

func (c PolicyAtt) Equals(in PolicyAtt) bool {
	return c.GroupKind == in.GroupKind && ptrEquals(c.PolicyRef, in.PolicyRef) && c.PolicyIr.Equals(in.PolicyIr)
}

func ptrEquals[T comparable](a, b *T) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

type AttachedPolicies struct {
	Policies map[schema.GroupKind][]PolicyAtt
}

func (a AttachedPolicies) Equals(b AttachedPolicies) bool {
	if len(a.Policies) != len(b.Policies) {
		return false
	}
	for k, v := range a.Policies {
		v2 := b.Policies[k]
		if len(v) != len(v2) {
			return false
		}
		for i, v := range v {
			if !v.Equals(v2[i]) {
				return false
			}
		}
	}
	return true
}

func (l AttachedPolicies) MarshalJSON() ([]byte, error) {
	m := map[string][]PolicyAtt{}
	for k, v := range l.Policies {
		m[k.String()] = v
	}

	return json.Marshal(m)
}

type BackendRefIR struct {
	// TODO: remove cluster name from here, it's redundant.
	ClusterName string
	Weight      uint32

	// backend could be nil if not found or no ref grant
	BackendObject *BackendObjectIR
	// if nil, error might say why
	Err error
}

type HttpBackendOrDelegate struct {
	Backend          *BackendRefIR
	Delegate         *ObjectSource
	AttachedPolicies AttachedPolicies
}

type HttpRouteRuleIR struct {
	ExtensionRefs    AttachedPolicies
	AttachedPolicies AttachedPolicies
	Backends         []HttpBackendOrDelegate
	Matches          []gwv1.HTTPRouteMatch
	Name             string
	Timeouts         *gwv1.HTTPRouteTimeouts
}

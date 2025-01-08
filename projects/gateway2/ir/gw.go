package ir

import (
	"context"
	"encoding/json"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type UpstreamInit struct {
	InitUpstream func(ctx context.Context, in Upstream, out *envoy_config_cluster_v3.Cluster)
}

type PolicyTargetRef struct {
	Group       string
	Kind        string
	Name        string
	SectionName string
}

type PolicyAtt struct {
	GroupKind schema.GroupKind
	// original object. ideally with structural errors removed.
	// Opaque to us other than metadata.
	PolicyIr PolicyIR

	// policy target ref that cause the attachment (can be used to report status correctly). nil if extension ref
	PolicyTargetRef *PolicyTargetRef
}

func (c PolicyAtt) Obj() PolicyIR {
	return c.PolicyIr
}

func (c PolicyAtt) TargetRef() *PolicyTargetRef {
	return c.PolicyTargetRef
}

func (c PolicyAtt) Equals(in PolicyAtt) bool {
	return c.GroupKind == in.GroupKind && ptrEquals(c.PolicyTargetRef, in.PolicyTargetRef) && c.PolicyIr.Equals(in.PolicyIr)
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

type Backend struct {
	// TODO: remove cluster name from here, it's redundant.
	ClusterName string
	Weight      uint32

	// upstream could be nil if not found or no ref grant
	Upstream *Upstream
	// if nil, error might say why
	Err error
}

type HttpBackendOrDelegate struct {
	Backend          *Backend
	Delegate         *ObjectSource
	AttachedPolicies AttachedPolicies
}

type HttpRouteRuleIR struct {
	ExtensionRefs    AttachedPolicies
	AttachedPolicies AttachedPolicies
	Backends         []HttpBackendOrDelegate
	Matches          []gwv1.HTTPRouteMatch
	Name             string
}

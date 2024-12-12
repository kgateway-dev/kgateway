package ir

import (
	"context"
	"encoding/json"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// types here are not in krt collections, so no need for equals

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

type AttachedPolicies struct {
	Policies map[schema.GroupKind][]PolicyAtt
}

func (l AttachedPolicies) MarshalJSON() ([]byte, error) {
	m := map[string][]PolicyAtt{}
	for k, v := range l.Policies {
		m[k.String()] = v
	}

	return json.Marshal(m)
}

type Backend struct {
	ClusterName string
	Weight      uint32

	// upstream could be nil if not found or no ref grant
	Upstream *Upstream
	// if nil, error might say why
	Err error
}

type HttpRouteRuleCommonIR struct {
	SourceRule       *gwv1.HTTPRouteRule
	ExtensionRefs    AttachedPolicies
	AttachedPolicies AttachedPolicies
}

type HttpBackendOrDelegate struct {
	Backend  *Backend
	Delegate *ObjectSource
	AttachedPolicies
}

type HttpBackend struct {
	Backend Backend
	AttachedPolicies
}

type HttpRouteRuleIR struct {
	HttpRouteRuleCommonIR
	Backends []HttpBackendOrDelegate
	Matches  []gwv1.HTTPRouteMatch
	Name     string
}

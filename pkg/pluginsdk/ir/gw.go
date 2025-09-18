package ir

import (
	"context"
	"encoding/json"
	"maps"
	"slices"
	"strings"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	apiannotations "github.com/kgateway-dev/kgateway/v2/api/annotations"
)

var VirtualBuiltInGK = schema.GroupKind{
	Group: "builtin",
	Kind:  "builtin",
}

type BackendInit struct {
	// InitEnvoyBackend optionally returns an `*ir.EndpointsForBackend` that can be used
	// to initialize a ClusterLoadAssignment inline on the Cluster, with proper locality
	// based prioritization applied, as well as endpoint plugins applied.
	// This will never override a ClusterLoadAssignment that is set inside of an InitEnvoyBackend implementation.
	// The CLA is only added if the Cluster has a compatible type (EDS, LOGICAL_DNS, STRICT_DNS).
	InitEnvoyBackend func(ctx context.Context, in BackendObjectIR, out *envoyclusterv3.Cluster) *EndpointsForBackend
}

type PolicyRef struct {
	Group       string
	Kind        string
	Name        string
	SectionName string
	MatchLabels map[string]string
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

	// Generation of the Policy CR contributing to this attachment
	Generation int64

	// original object. ideally with structural errors removed.
	// Opaque to us other than metadata.
	PolicyIr PolicyIR

	// PolicyRef is a ref to the original policy that is attached (can be used to report status correctly).
	// nil if the attachment was done via extension ref or if PolicyAtt is the result of MergePolicies(...)
	PolicyRef *AttachedPolicyRef

	// InheritedPolicyPriority is the priority of the policy when it is inherited by a child resource
	// of the resource this policy is attached to
	InheritedPolicyPriority apiannotations.InheritedPolicyPriorityValue

	// Errors should be formatted for users, so do not include internal lib errors.
	// Instead use a well defined error such as ErrInvalidConfig
	Errors []error

	// HierarchicalPriority is the priority of the policy in an inheritance hierarchy.
	// A higher value means higher priority. It is used to accurately merge policies
	// that are at different levels in the config tree hierarchy.
	HierarchicalPriority int

	// PrecedenceWeight specifies the weight of the policy as an integer value (negative values are allowed).
	// Policies with higher weight implies higher priority, and are evaluated before policies with lower weight.
	// By default, policies have a weight of 0.
	// The policy's weight is relevant to policy prioritization during policy merging, such that higher priority
	// policies are preferred during a merge conflict or when ordering policies during a merge.
	PrecedenceWeight int32

	// MergeOrigins maps field names in the PolicyIr to their original source in the merged PolicyAtt.
	// It can be used to determine which PolicyAtt a merged field came from.
	// Only relevant to policy merging and does not contribute to KRT events
	// +noKrtEquals
	MergeOrigins MergeOrigins
}

func (c PolicyAtt) FormatErrors() string {
	errs := make([]string, len(c.Errors))
	for i, err := range c.Errors {
		errs[i] = err.Error()
	}

	errsStr := strings.Join(errs, "; ")
	if c.MergeOrigins.IsSet() {
		return "Merged policy: " + errsStr
	}
	return errsStr
}

type PolicyAttachmentOpts func(*PolicyAtt)

func WithInheritedPolicyPriority(priority apiannotations.InheritedPolicyPriorityValue) PolicyAttachmentOpts {
	return func(p *PolicyAtt) {
		p.InheritedPolicyPriority = priority
	}
}

func (c PolicyAtt) Obj() PolicyIR {
	return c.PolicyIr
}

func (c PolicyAtt) TargetRef() *AttachedPolicyRef {
	return c.PolicyRef
}

func (c PolicyAtt) Equals(in PolicyAtt) bool {
	if !slices.EqualFunc(c.Errors, in.Errors, func(e1, e2 error) bool {
		if e1 == nil && e2 != nil {
			return false
		}
		if e1 != nil && e2 == nil {
			return false
		}
		if (e1 != nil && e2 != nil) && e1.Error() != e2.Error() {
			return false
		}

		return true
	}) {
		return false
	}

	return c.GroupKind == in.GroupKind &&
		c.Generation == in.Generation &&
		c.PolicyIr.Equals(in.PolicyIr) &&
		ptrEquals(c.PolicyRef, in.PolicyRef) &&
		c.InheritedPolicyPriority == in.InheritedPolicyPriority &&
		c.HierarchicalPriority == in.HierarchicalPriority &&
		c.PrecedenceWeight == in.PrecedenceWeight
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

// ApplyOrderedGroupKinds returns a list of GroupKinds sorted by their application order
// from highest to lowest priority.
// Built-in policies are applied first as they are of highest priority relative to other GroupKinds
// as they are considered more specific than other policy attachments.
func (a AttachedPolicies) ApplyOrderedGroupKinds() []schema.GroupKind {
	return slices.SortedStableFunc(maps.Keys(a.Policies), func(a, b schema.GroupKind) int {
		switch {
		case a.Group == VirtualBuiltInGK.Group:
			// If a is builtin, a should come before b
			return -1
		case b.Group == VirtualBuiltInGK.Group:
			// If b is builtin, a should come after b
			return 1
		default:
			// neither is builtin, preserve relative order
			return 0
		}
	})
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

// Append appends the policies in l in the given order to the policies in a.
func (a *AttachedPolicies) Append(l ...AttachedPolicies) {
	if a.Policies == nil {
		a.Policies = make(map[schema.GroupKind][]PolicyAtt)
	}
	for _, l := range l {
		for k, v := range l.Policies {
			if a.Policies == nil {
				a.Policies = make(map[schema.GroupKind][]PolicyAtt)
			}
			a.Policies[k] = append(a.Policies[k], v...)
		}
	}
}

// Append appends the policies in l in the given order to the policies in a.
func (a *AttachedPolicies) AppendWithPriority(HierarchicalPriority int, l ...AttachedPolicies) {
	if a.Policies == nil {
		a.Policies = make(map[schema.GroupKind][]PolicyAtt)
	}
	for _, l := range l {
		for k, v := range l.Policies {
			for j := range v {
				v[j].HierarchicalPriority = HierarchicalPriority
			}
			a.Policies[k] = append(a.Policies[k], v...)
		}
	}
}

// Prepend prepends the policies in l in the given to the policies in a.
func (a *AttachedPolicies) Prepend(hierarchicalPriority int, l ...AttachedPolicies) {
	if a.Policies == nil {
		a.Policies = make(map[schema.GroupKind][]PolicyAtt)
	}
	// iterate in the reverse order so that the input order in l is preserved at the end
	for i := len(l) - 1; i >= 0; i-- {
		for k, v := range l[i].Policies {
			for j := range v {
				v[j].HierarchicalPriority = hierarchicalPriority
			}
			a.Policies[k] = append(v, a.Policies[k]...)
		}
	}
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

	// Err contains any error encountered during the construction of this HttpRouteRuleIR
	// that should be propagated through to translation to any derived ir.HttpRouteRuleMatchIRs
	Err error
}

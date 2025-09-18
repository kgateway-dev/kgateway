package ir

import (
	"errors"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	types "k8s.io/apimachinery/pkg/types"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
)

// makeTCPRoute constructs a TCPRoute with specified metadata.
func makeTCPRoute(name, namespace, rv string, gen int64, uid types.UID) *gwv1alpha2.TCPRoute {
	return &gwv1alpha2.TCPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			ResourceVersion: rv,
			Generation:      gen,
			UID:             uid,
		},
	}
}

// makeBackendRef constructs a BackendRefIR with no BackendObject for testing.
func makeBackendRef(cluster string, weight uint32) BackendRefIR {
	return BackendRefIR{ClusterName: cluster, Weight: weight, BackendObject: nil}
}

func TestTcpRouteIREquals(t *testing.T) {
	base := makeTCPRoute("route1", "test-ns", "1", 1, types.UID("uid1"))

	emptyPolicies := AttachedPolicies{}
	nonEmptyPolicies := AttachedPolicies{
		Policies: map[schema.GroupKind][]PolicyAtt{
			{Group: "g", Kind: "k"}: {{GroupKind: schema.GroupKind{Group: "g", Kind: "k"}}},
		},
	}

	emptyBackends := []BackendRefIR{}
	backendA := []BackendRefIR{makeBackendRef("clusterA", 5)}
	backendB := []BackendRefIR{makeBackendRef("clusterB", 5)}
	backendWeightDiff := []BackendRefIR{makeBackendRef("clusterA", 10)}
	backendErr := []BackendRefIR{{ClusterName: "clusterA", Weight: 5, BackendObject: nil, Err: errors.New("some error")}}

	tests := []struct {
		name string
		a, b TcpRouteIR
		want bool
	}{
		{
			name: "identical_empty",
			a: TcpRouteIR{
				ObjectSource:     ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:     base,
				AttachedPolicies: emptyPolicies,
				Backends:         emptyBackends,
			},
			b: TcpRouteIR{
				ObjectSource:     ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:     base,
				AttachedPolicies: emptyPolicies,
				Backends:         emptyBackends,
			},
			want: true,
		},
		{
			name: "diff_objectsource",
			a: TcpRouteIR{
				ObjectSource:     ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:     base,
				AttachedPolicies: emptyPolicies,
				Backends:         emptyBackends,
			},
			b: TcpRouteIR{
				ObjectSource:     ObjectSource{Namespace: "test-ns", Name: "route2"},
				SourceObject:     base,
				AttachedPolicies: emptyPolicies,
				Backends:         emptyBackends,
			},
			want: false,
		},
		{
			name: "diff_attached_policies",
			a: TcpRouteIR{
				ObjectSource:     ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:     base,
				AttachedPolicies: emptyPolicies,
				Backends:         emptyBackends,
			},
			b: TcpRouteIR{
				ObjectSource:     ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:     base,
				AttachedPolicies: nonEmptyPolicies,
				Backends:         emptyBackends,
			},
			want: false,
		},
		{
			name: "diff_backends_length",
			a: TcpRouteIR{
				ObjectSource:     ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:     base,
				AttachedPolicies: emptyPolicies,
				Backends:         emptyBackends,
			},
			b: TcpRouteIR{
				ObjectSource:     ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:     base,
				AttachedPolicies: emptyPolicies,
				Backends:         backendA,
			},
			want: false,
		},
		{
			name: "diff_backend_cluster",
			a: TcpRouteIR{
				ObjectSource:     ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:     base,
				AttachedPolicies: emptyPolicies,
				Backends:         backendA,
			},
			b: TcpRouteIR{
				ObjectSource:     ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:     base,
				AttachedPolicies: emptyPolicies,
				Backends:         backendB,
			},
			want: false,
		},
		{
			name: "diff_backend_weight",
			a: TcpRouteIR{
				ObjectSource:     ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:     base,
				AttachedPolicies: emptyPolicies,
				Backends:         backendA,
			},
			b: TcpRouteIR{
				ObjectSource:     ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:     base,
				AttachedPolicies: emptyPolicies,
				Backends:         backendWeightDiff,
			},
			want: false,
		},
		{
			name: "identical_backends",
			a: TcpRouteIR{
				ObjectSource:     ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:     base,
				AttachedPolicies: emptyPolicies,
				Backends:         backendA,
			},
			b: TcpRouteIR{
				ObjectSource:     ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:     base,
				AttachedPolicies: emptyPolicies,
				Backends:         backendA,
			},
			want: true,
		},
		{
			name: "diff_backend_error",
			a: TcpRouteIR{
				ObjectSource:     ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:     base,
				AttachedPolicies: emptyPolicies,
				Backends:         backendA,
			},
			b: TcpRouteIR{
				ObjectSource:     ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:     base,
				AttachedPolicies: emptyPolicies,
				Backends:         backendErr,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.a.Equals(tt.b)
			if got != tt.want {
				t.Errorf("TcpRouteIR.Equals() = %v, want %v", got, tt.want)
			}
			// symmetry
			if tt.a.Equals(tt.b) != tt.b.Equals(tt.a) {
				t.Errorf("symmetry mismatch: a.Equals(b)=%v, b.Equals(a)=%v", tt.a.Equals(tt.b), tt.b.Equals(tt.a))
			}
		})
	}
}

func TestHTTPRouteIREquals(t *testing.T) {
	// Helper functions to create test objects
	makeHTTPRoute := func(name, namespace, rv string, gen int64, uid types.UID) metav1.Object {
		return &metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			ResourceVersion: rv,
			Generation:      gen,
			UID:             uid,
		}
	}

	makeHttpBackendOrDelegate := func(cluster string, weight uint32) HttpBackendOrDelegate {
		return HttpBackendOrDelegate{
			Backend: &BackendRefIR{
				ClusterName: cluster,
				Weight:      weight,
			},
			AttachedPolicies: AttachedPolicies{},
		}
	}

	makeHttpBackendOrDelegateWithPolicies := func(cluster string, weight uint32, policies AttachedPolicies) HttpBackendOrDelegate {
		return HttpBackendOrDelegate{
			Backend: &BackendRefIR{
				ClusterName: cluster,
				Weight:      weight,
			},
			AttachedPolicies: policies,
		}
	}

	makeHttpBackendOrDelegateDelegate := func(delegate ObjectSource) HttpBackendOrDelegate {
		return HttpBackendOrDelegate{
			Delegate:         &delegate,
			AttachedPolicies: AttachedPolicies{},
		}
	}

	// Test data
	base := makeHTTPRoute("route1", "test-ns", "1", 1, types.UID("uid1"))
	differentName := makeHTTPRoute("route2", "test-ns", "1", 1, types.UID("uid1"))
	differentUID := makeHTTPRoute("route1", "test-ns", "1", 1, types.UID("uid2"))
	differentGeneration := makeHTTPRoute("route1", "test-ns", "1", 2, types.UID("uid1"))

	emptyPolicies := AttachedPolicies{}
	nonEmptyPolicies := AttachedPolicies{
		Policies: map[schema.GroupKind][]PolicyAtt{
			{Group: "g", Kind: "k"}: {{GroupKind: schema.GroupKind{Group: "g", Kind: "k"}}},
		},
	}

	// Rule test data
	emptyRules := []HttpRouteRuleIR{}
	ruleA := []HttpRouteRuleIR{{
		ExtensionRefs:    emptyPolicies,
		AttachedPolicies: emptyPolicies,
		Backends:         []HttpBackendOrDelegate{makeHttpBackendOrDelegate("clusterA", 5)},
		Name:             "ruleA",
	}}
	ruleB := []HttpRouteRuleIR{{
		ExtensionRefs:    emptyPolicies,
		AttachedPolicies: emptyPolicies,
		Backends:         []HttpBackendOrDelegate{makeHttpBackendOrDelegate("clusterB", 5)},
		Name:             "ruleB",
	}}
	ruleDiffPolicies := []HttpRouteRuleIR{{
		ExtensionRefs:    nonEmptyPolicies,
		AttachedPolicies: emptyPolicies,
		Backends:         []HttpBackendOrDelegate{makeHttpBackendOrDelegate("clusterA", 5)},
		Name:             "ruleA",
	}}
	ruleDiffBackends := []HttpRouteRuleIR{{
		ExtensionRefs:    emptyPolicies,
		AttachedPolicies: emptyPolicies,
		Backends:         []HttpBackendOrDelegate{makeHttpBackendOrDelegate("clusterA", 10)},
		Name:             "ruleA",
	}}
	ruleWithDelegate := []HttpRouteRuleIR{{
		ExtensionRefs:    emptyPolicies,
		AttachedPolicies: emptyPolicies,
		Backends:         []HttpBackendOrDelegate{makeHttpBackendOrDelegateDelegate(ObjectSource{Name: "delegate", Namespace: "test-ns"})},
		Name:             "ruleA",
	}}
	ruleBackendNil := []HttpRouteRuleIR{{
		ExtensionRefs:    emptyPolicies,
		AttachedPolicies: emptyPolicies,
		Backends:         []HttpBackendOrDelegate{{Backend: nil, AttachedPolicies: emptyPolicies}},
		Name:             "ruleA",
	}}
	ruleBackendPolicyDiff := []HttpRouteRuleIR{{
		ExtensionRefs:    emptyPolicies,
		AttachedPolicies: emptyPolicies,
		Backends:         []HttpBackendOrDelegate{makeHttpBackendOrDelegateWithPolicies("clusterA", 5, nonEmptyPolicies)},
		Name:             "ruleA",
	}}

	tests := []struct {
		name string
		a, b HttpRouteIR
		want bool
	}{
		{
			name: "identical_empty",
			a: HttpRouteIR{
				ObjectSource:                   ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:                   base,
				AttachedPolicies:               emptyPolicies,
				Rules:                          emptyRules,
				PrecedenceWeight:               0,
				DelegationInheritParentMatcher: false,
			},
			b: HttpRouteIR{
				ObjectSource:                   ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:                   base,
				AttachedPolicies:               emptyPolicies,
				Rules:                          emptyRules,
				PrecedenceWeight:               0,
				DelegationInheritParentMatcher: false,
			},
			want: true,
		},
		{
			name: "identical_with_precedence_weight",
			a: HttpRouteIR{
				ObjectSource:                   ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:                   base,
				AttachedPolicies:               emptyPolicies,
				Rules:                          emptyRules,
				PrecedenceWeight:               100,
				DelegationInheritParentMatcher: false,
			},
			b: HttpRouteIR{
				ObjectSource:                   ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:                   base,
				AttachedPolicies:               emptyPolicies,
				Rules:                          emptyRules,
				PrecedenceWeight:               100,
				DelegationInheritParentMatcher: false,
			},
			want: true,
		},
		{
			name: "identical_with_delegation_matcher",
			a: HttpRouteIR{
				ObjectSource:                   ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:                   base,
				AttachedPolicies:               emptyPolicies,
				Rules:                          emptyRules,
				PrecedenceWeight:               0,
				DelegationInheritParentMatcher: true,
			},
			b: HttpRouteIR{
				ObjectSource:                   ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:                   base,
				AttachedPolicies:               emptyPolicies,
				Rules:                          emptyRules,
				PrecedenceWeight:               0,
				DelegationInheritParentMatcher: true,
			},
			want: true,
		},
		{
			name: "diff_objectsource",
			a: HttpRouteIR{
				ObjectSource:                   ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:                   base,
				AttachedPolicies:               emptyPolicies,
				Rules:                          emptyRules,
				PrecedenceWeight:               0,
				DelegationInheritParentMatcher: false,
			},
			b: HttpRouteIR{
				ObjectSource:                   ObjectSource{Namespace: "test-ns", Name: "route2"},
				SourceObject:                   differentName,
				AttachedPolicies:               emptyPolicies,
				Rules:                          emptyRules,
				PrecedenceWeight:               0,
				DelegationInheritParentMatcher: false,
			},
			want: false,
		},
		{
			name: "diff_source_object_uid",
			a: HttpRouteIR{
				ObjectSource:                   ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:                   base,
				AttachedPolicies:               emptyPolicies,
				Rules:                          emptyRules,
				PrecedenceWeight:               0,
				DelegationInheritParentMatcher: false,
			},
			b: HttpRouteIR{
				ObjectSource:                   ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:                   differentUID,
				AttachedPolicies:               emptyPolicies,
				Rules:                          emptyRules,
				PrecedenceWeight:               0,
				DelegationInheritParentMatcher: false,
			},
			want: false,
		},
		{
			name: "diff_source_object_generation",
			a: HttpRouteIR{
				ObjectSource:                   ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:                   base,
				AttachedPolicies:               emptyPolicies,
				Rules:                          emptyRules,
				PrecedenceWeight:               0,
				DelegationInheritParentMatcher: false,
			},
			b: HttpRouteIR{
				ObjectSource:                   ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:                   differentGeneration,
				AttachedPolicies:               emptyPolicies,
				Rules:                          emptyRules,
				PrecedenceWeight:               0,
				DelegationInheritParentMatcher: false,
			},
			want: false,
		},
		{
			name: "diff_attached_policies",
			a: HttpRouteIR{
				ObjectSource:                   ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:                   base,
				AttachedPolicies:               emptyPolicies,
				Rules:                          emptyRules,
				PrecedenceWeight:               0,
				DelegationInheritParentMatcher: false,
			},
			b: HttpRouteIR{
				ObjectSource:                   ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:                   base,
				AttachedPolicies:               nonEmptyPolicies,
				Rules:                          emptyRules,
				PrecedenceWeight:               0,
				DelegationInheritParentMatcher: false,
			},
			want: false,
		},
		{
			name: "diff_precedence_weight",
			a: HttpRouteIR{
				ObjectSource:                   ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:                   base,
				AttachedPolicies:               emptyPolicies,
				Rules:                          emptyRules,
				PrecedenceWeight:               100,
				DelegationInheritParentMatcher: false,
			},
			b: HttpRouteIR{
				ObjectSource:                   ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:                   base,
				AttachedPolicies:               emptyPolicies,
				Rules:                          emptyRules,
				PrecedenceWeight:               200,
				DelegationInheritParentMatcher: false,
			},
			want: false,
		},
		{
			name: "diff_delegation_inherit_parent_matcher",
			a: HttpRouteIR{
				ObjectSource:                   ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:                   base,
				AttachedPolicies:               emptyPolicies,
				Rules:                          emptyRules,
				PrecedenceWeight:               0,
				DelegationInheritParentMatcher: true,
			},
			b: HttpRouteIR{
				ObjectSource:                   ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:                   base,
				AttachedPolicies:               emptyPolicies,
				Rules:                          emptyRules,
				PrecedenceWeight:               0,
				DelegationInheritParentMatcher: false,
			},
			want: false,
		},
		{
			name: "diff_rules_length",
			a: HttpRouteIR{
				ObjectSource:                   ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:                   base,
				AttachedPolicies:               emptyPolicies,
				Rules:                          emptyRules,
				PrecedenceWeight:               0,
				DelegationInheritParentMatcher: false,
			},
			b: HttpRouteIR{
				ObjectSource:                   ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:                   base,
				AttachedPolicies:               emptyPolicies,
				Rules:                          ruleA,
				PrecedenceWeight:               0,
				DelegationInheritParentMatcher: false,
			},
			want: false,
		},
		{
			name: "diff_rules_attached_policies",
			a: HttpRouteIR{
				ObjectSource:                   ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:                   base,
				AttachedPolicies:               emptyPolicies,
				Rules:                          ruleA,
				PrecedenceWeight:               0,
				DelegationInheritParentMatcher: false,
			},
			b: HttpRouteIR{
				ObjectSource:                   ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:                   base,
				AttachedPolicies:               emptyPolicies,
				Rules:                          ruleDiffPolicies,
				PrecedenceWeight:               0,
				DelegationInheritParentMatcher: false,
			},
			want: false,
		},
		{
			name: "diff_rules_backends_cluster",
			a: HttpRouteIR{
				ObjectSource:                   ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:                   base,
				AttachedPolicies:               emptyPolicies,
				Rules:                          ruleA,
				PrecedenceWeight:               0,
				DelegationInheritParentMatcher: false,
			},
			b: HttpRouteIR{
				ObjectSource:                   ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:                   base,
				AttachedPolicies:               emptyPolicies,
				Rules:                          ruleB,
				PrecedenceWeight:               0,
				DelegationInheritParentMatcher: false,
			},
			want: false,
		},
		{
			name: "diff_rules_backends_weight",
			a: HttpRouteIR{
				ObjectSource:                   ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:                   base,
				AttachedPolicies:               emptyPolicies,
				Rules:                          ruleA,
				PrecedenceWeight:               0,
				DelegationInheritParentMatcher: false,
			},
			b: HttpRouteIR{
				ObjectSource:                   ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:                   base,
				AttachedPolicies:               emptyPolicies,
				Rules:                          ruleDiffBackends,
				PrecedenceWeight:               0,
				DelegationInheritParentMatcher: false,
			},
			want: false,
		},
		{
			name: "diff_rules_backend_vs_delegate",
			a: HttpRouteIR{
				ObjectSource:                   ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:                   base,
				AttachedPolicies:               emptyPolicies,
				Rules:                          ruleA,
				PrecedenceWeight:               0,
				DelegationInheritParentMatcher: false,
			},
			b: HttpRouteIR{
				ObjectSource:                   ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:                   base,
				AttachedPolicies:               emptyPolicies,
				Rules:                          ruleWithDelegate,
				PrecedenceWeight:               0,
				DelegationInheritParentMatcher: false,
			},
			want: false,
		},
		{
			name: "diff_rules_backend_nil_vs_non_nil",
			a: HttpRouteIR{
				ObjectSource:                   ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:                   base,
				AttachedPolicies:               emptyPolicies,
				Rules:                          ruleA,
				PrecedenceWeight:               0,
				DelegationInheritParentMatcher: false,
			},
			b: HttpRouteIR{
				ObjectSource:                   ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:                   base,
				AttachedPolicies:               emptyPolicies,
				Rules:                          ruleBackendNil,
				PrecedenceWeight:               0,
				DelegationInheritParentMatcher: false,
			},
			want: false,
		},
		{
			name: "diff_rules_backend_attached_policies",
			a: HttpRouteIR{
				ObjectSource:                   ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:                   base,
				AttachedPolicies:               emptyPolicies,
				Rules:                          ruleA,
				PrecedenceWeight:               0,
				DelegationInheritParentMatcher: false,
			},
			b: HttpRouteIR{
				ObjectSource:                   ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:                   base,
				AttachedPolicies:               emptyPolicies,
				Rules:                          ruleBackendPolicyDiff,
				PrecedenceWeight:               0,
				DelegationInheritParentMatcher: false,
			},
			want: false,
		},
		{
			name: "identical_with_rules",
			a: HttpRouteIR{
				ObjectSource:                   ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:                   base,
				AttachedPolicies:               emptyPolicies,
				Rules:                          ruleA,
				PrecedenceWeight:               0,
				DelegationInheritParentMatcher: false,
			},
			b: HttpRouteIR{
				ObjectSource:                   ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:                   base,
				AttachedPolicies:               emptyPolicies,
				Rules:                          ruleA,
				PrecedenceWeight:               0,
				DelegationInheritParentMatcher: false,
			},
			want: true,
		},
		{
			name: "identical_both_backends_nil",
			a: HttpRouteIR{
				ObjectSource:                   ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:                   base,
				AttachedPolicies:               emptyPolicies,
				Rules:                          ruleBackendNil,
				PrecedenceWeight:               0,
				DelegationInheritParentMatcher: false,
			},
			b: HttpRouteIR{
				ObjectSource:                   ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:                   base,
				AttachedPolicies:               emptyPolicies,
				Rules:                          ruleBackendNil,
				PrecedenceWeight:               0,
				DelegationInheritParentMatcher: false,
			},
			want: true,
		},
		{
			name: "identical_with_delegates",
			a: HttpRouteIR{
				ObjectSource:                   ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:                   base,
				AttachedPolicies:               emptyPolicies,
				Rules:                          ruleWithDelegate,
				PrecedenceWeight:               0,
				DelegationInheritParentMatcher: false,
			},
			b: HttpRouteIR{
				ObjectSource:                   ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:                   base,
				AttachedPolicies:               emptyPolicies,
				Rules:                          ruleWithDelegate,
				PrecedenceWeight:               0,
				DelegationInheritParentMatcher: false,
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := assert.New(t)

			got := tt.a.Equals(tt.b)
			a.Equal(tt.want, got, cmp.Diff(tt.a, tt.b))
		})
	}
}

package trafficpolicy

import (
	"errors"
	"testing"
	"time"

	envoymatchingv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/common/matching/v3"
	envoy_ext_authz_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"

	apiannotations "github.com/kgateway-dev/kgateway/v2/api/annotations"
	kgateway "github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/policy"
)

func TestMergePoliciesPreservesErrors(t *testing.T) {
	err1 := errors.New("err1")
	err2 := errors.New("err2")

	gk := schema.GroupKind{Group: "test", Kind: "TrafficPolicy"}

	p1 := ir.PolicyAtt{
		GroupKind: gk,
		PolicyRef: &ir.AttachedPolicyRef{Name: "p1"},
		PolicyIr:  &TrafficPolicy{ct: time.Now()},
		Errors:    []error{err1},
	}
	p2 := ir.PolicyAtt{
		GroupKind: gk,
		PolicyRef: &ir.AttachedPolicyRef{Name: "p2"},
		PolicyIr:  &TrafficPolicy{ct: time.Now().Add(time.Minute)},
		Errors:    []error{err2},
	}

	merged := policy.MergePolicies([]ir.PolicyAtt{p1, p2}, mergeTrafficPolicies, "")
	require.Len(t, merged.Errors, 2)

	// Each merged error should be a *PolicyError attributing the source policy
	// while the original sentinel remains reachable via errors.Is.
	byName := map[string]*ir.PolicyError{}
	for _, e := range merged.Errors {
		var pe *ir.PolicyError
		require.True(t, errors.As(e, &pe), "merged error should be a *ir.PolicyError")
		require.NotNil(t, pe.Ref)
		byName[pe.Ref.Name] = pe
	}
	require.Contains(t, byName, "p1")
	require.Contains(t, byName, "p2")
	assert.True(t, errors.Is(byName["p1"], err1))
	assert.True(t, errors.Is(byName["p2"], err2))
}

func TestMergeRequestMirror(t *testing.T) {
	p2Ref := &ir.AttachedPolicyRef{Name: "p2", Namespace: "default"}
	mergeOptions := policy.MergeOptions{Strategy: policy.AugmentedShallowMerge}

	t.Run("higher priority false wins over lower priority true", func(t *testing.T) {
		p1 := &TrafficPolicy{spec: trafficPolicySpecIr{requestMirror: requestMirrorIRWithValue(false)}}
		p2 := &TrafficPolicy{spec: trafficPolicySpecIr{requestMirror: requestMirrorIRWithValue(true)}}

		MergeTrafficPolicies(p1, p2, p2Ref, nil, mergeOptions, ir.MergeOrigins{}, TrafficPolicyMergeOpts{})

		require.NotNil(t, p1.spec.requestMirror)
		require.NotNil(t, p1.spec.requestMirror.disableShadowHostSuffixAppend)
		assert.False(t, *p1.spec.requestMirror.disableShadowHostSuffixAppend)
	})

	t.Run("lower priority value fills an unset field", func(t *testing.T) {
		p1 := &TrafficPolicy{}
		p2 := &TrafficPolicy{spec: trafficPolicySpecIr{requestMirror: requestMirrorIRWithValue(true)}}

		MergeTrafficPolicies(p1, p2, p2Ref, nil, mergeOptions, ir.MergeOrigins{}, TrafficPolicyMergeOpts{})

		require.NotNil(t, p1.spec.requestMirror)
		require.NotNil(t, p1.spec.requestMirror.disableShadowHostSuffixAppend)
		assert.True(t, *p1.spec.requestMirror.disableShadowHostSuffixAppend)
	})

	// requestMirror is merged as a whole block: a more-specific block wins entirely rather than
	// combining field-by-field with a less-specific one.
	t.Run("more-specific block wins entirely, no field-by-field combine", func(t *testing.T) {
		p1 := &TrafficPolicy{spec: trafficPolicySpecIr{requestMirror: requestMirrorIRWithLiteral("literal.example:8080")}}
		p2 := &TrafficPolicy{spec: trafficPolicySpecIr{requestMirror: requestMirrorIRWithValue(true)}}

		MergeTrafficPolicies(p1, p2, p2Ref, nil, mergeOptions, ir.MergeOrigins{}, TrafficPolicyMergeOpts{})

		require.NotNil(t, p1.spec.requestMirror)
		// The more-specific block only had the literal, so the less-specific bool must not leak in.
		require.NotNil(t, p1.spec.requestMirror.hostRewriteLiteral)
		assert.Equal(t, "literal.example:8080", *p1.spec.requestMirror.hostRewriteLiteral)
		assert.Nil(t, p1.spec.requestMirror.disableShadowHostSuffixAppend)
	})
}

// TestMergePoliciesDoesNotMutateSourceIRs covers the delegated-route shape from
// test/e2e/features/route_delegation: one child route reached through two parents with
// different inherited-policy-priority annotations.
//
// A policy IR is KRT collection output shared by every translation that references it,
// and a child's policy is shared by all of its delegating parents. Merging must not write
// into it: the source would then contribute its already-merged value to the next
// translation, so config accumulates across routes and across translation cycles, and
// values leak between delegation trees.
func TestMergePoliciesDoesNotMutateSourceIRs(t *testing.T) {
	gk := schema.GroupKind{Group: "gateway.kgateway.dev", Kind: "TrafficPolicy"}

	// translate merges a child policy with a parent policy inherited from a delegating
	// parent route, the way runRoutePlugins does for a delegated route.
	//
	// mergeSettingsJSON is the policyMerge setting, which policy.MergePolicies applies to
	// the merge within a hierarchy but not to the merge across hierarchies.
	translate := func(child, parent *TrafficPolicy, prio apiannotations.InheritedPolicyPriorityValue, mergeSettingsJSON string) *TrafficPolicy {
		t.Helper()
		merged := policy.MergePolicies([]ir.PolicyAtt{
			{
				GroupKind:            gk,
				PolicyIr:             child,
				PolicyRef:            &ir.AttachedPolicyRef{Group: gk.Group, Kind: gk.Kind, Name: "child", Namespace: "team1"},
				HierarchicalPriority: 0,
			},
			{
				GroupKind:               gk,
				PolicyIr:                parent,
				PolicyRef:               &ir.AttachedPolicyRef{Group: gk.Group, Kind: gk.Kind, Name: "parent", Namespace: "infra"},
				HierarchicalPriority:    -1,
				InheritedPolicyPriority: prio,
			},
		}, mergeTrafficPolicies, mergeSettingsJSON)
		out, ok := merged.PolicyIr.(*TrafficPolicy)
		require.True(t, ok, "merged PolicyIr should be a *TrafficPolicy")
		return out
	}

	t.Run("transformation", func(t *testing.T) {
		setOrigin := func(t *testing.T, value string) *TrafficPolicy {
			t.Helper()
			tp := &TrafficPolicy{ct: time.Now()}
			in := &kgateway.TrafficPolicy{
				Spec: kgateway.TrafficPolicySpec{
					Transformation: &kgateway.TransformationPolicy{
						Response: &kgateway.Transform{
							Set: []kgateway.HeaderTransformation{{Name: "origin", Value: kgateway.InjaTemplate(value)}},
						},
					},
				},
			}
			require.NoError(t, constructRustformation(in, &tp.spec))
			return tp
		}
		transformJSON := func(t *testing.T, tp *TrafficPolicy) string {
			t.Helper()
			require.NotNil(t, tp.spec.rustformation)
			sv := &wrapperspb.StringValue{}
			require.NoError(t, tp.spec.rustformation.config.GetFilterConfig().UnmarshalTo(sv))
			return sv.GetValue()
		}

		child := setOrigin(t, "svc1")
		preferChildParent := setOrigin(t, "parent1")
		preferParentParent := setOrigin(t, "parent2")
		childJSON := transformJSON(t, child)

		// The child is reached through the DeepMergePreferChild parent, then through the
		// DeepMergePreferParent parent, then through the first parent again on the next
		// translation cycle. Every result must be a pure function of its own inputs.
		wantPreferChild := transformJSON(t, translate(child, preferChildParent, apiannotations.DeepMergePreferChild, ""))
		assert.Equal(t, childJSON, transformJSON(t, child), "child IR must be unchanged after merging")

		gotPreferParent := transformJSON(t, translate(child, preferParentParent, apiannotations.DeepMergePreferParent, ""))
		assert.Equal(t, childJSON, transformJSON(t, child), "child IR must be unchanged after merging")
		assert.NotContains(t, gotPreferParent, "parent1", "the other tree's parent must not leak in")

		gotPreferChild := transformJSON(t, translate(child, preferChildParent, apiannotations.DeepMergePreferChild, ""))
		assert.Equal(t, childJSON, transformJSON(t, child), "child IR must be unchanged after merging")
		assert.Equal(t, wantPreferChild, gotPreferChild, "re-merging the same inputs must give the same result")
		assert.NotContains(t, gotPreferChild, "parent2", "the other tree's parent must not leak in")

		// Envoy applies "set" last-wins, so the surviving value is the last entry.
		assert.Regexp(t, `"value":"svc1"\}\]`, gotPreferChild, "DeepMergePreferChild: child wins")
		assert.Regexp(t, `"value":"parent2"\}\]`, gotPreferParent, "DeepMergePreferParent: parent wins")
	})

	t.Run("httpACL", func(t *testing.T) {
		acl := func(t *testing.T, cidr shared.IPOrCIDR) *TrafficPolicy {
			t.Helper()
			tp := &TrafficPolicy{ct: time.Now()}
			in := &kgateway.TrafficPolicy{
				Spec: kgateway.TrafficPolicySpec{
					ACL: &shared.ACLPolicy{
						DefaultAction: shared.ACLActionDeny,
						Rules:         []shared.ACLRule{{Action: shared.ACLActionAllow, CIDRs: []shared.IPOrCIDR{cidr}}},
					},
				},
			}
			require.NoError(t, constructHttpACL(in, &tp.spec))
			return tp
		}
		aclJSON := func(t *testing.T, tp *TrafficPolicy) string {
			t.Helper()
			require.NotNil(t, tp.spec.httpACL)
			sv := &wrapperspb.StringValue{}
			require.NoError(t, tp.spec.httpACL.config.GetFilterConfig().UnmarshalTo(sv))
			return sv.GetValue()
		}

		child := acl(t, "10.0.0.0/8")
		parent := acl(t, "192.168.0.0/16")
		childJSON := aclJSON(t, child)

		// mergeHttpACL deep merges within a hierarchy by default, which materializes a
		// fresh config and hides the aliasing. Opting the hierarchy merge into a shallow
		// merge leaves p1 pointing at the child's IR for the cross-hierarchy deep merge,
		// which is the shape that corrupts it.
		const shallowACL = `{"trafficPolicy":{"acl":"ShallowMerge"}}`

		first := aclJSON(t, translate(child, parent, apiannotations.DeepMergePreferChild, shallowACL))
		assert.Equal(t, childJSON, aclJSON(t, child), "child IR must be unchanged after merging")

		second := aclJSON(t, translate(child, parent, apiannotations.DeepMergePreferChild, shallowACL))
		assert.Equal(t, first, second, "re-merging the same inputs must give the same result")
	})

	// extProc and extAuth both merge by unioning providers, so a child and a parent that
	// reference different GatewayExtensions is the shape that appends the parent's provider
	// onto the child's IR. fetchExtension stands in for the GatewayExtension collection.
	fetchExtension := func(name string, build func(*TrafficPolicyGatewayExtensionIR)) FetchGatewayExtensionFunc {
		return func(krt.HandlerContext, shared.NamespacedObjectReference, string) (*TrafficPolicyGatewayExtensionIR, error) {
			ext := &TrafficPolicyGatewayExtensionIR{Name: name}
			build(ext)
			return ext, nil
		}
	}
	extensionRef := &shared.NamespacedObjectReference{Name: "ext"}

	t.Run("extProc", func(t *testing.T) {
		policyFor := func(t *testing.T, provider string) *TrafficPolicy {
			t.Helper()
			tp := &TrafficPolicy{ct: time.Now()}
			in := &kgateway.TrafficPolicy{
				Spec: kgateway.TrafficPolicySpec{ExtProc: &kgateway.ExtProcPolicy{ExtensionRef: extensionRef}},
			}
			fetch := fetchExtension(provider, func(e *TrafficPolicyGatewayExtensionIR) {
				e.ExtProc = &envoymatchingv3.ExtensionWithMatcher{}
			})
			require.NoError(t, constructExtProc(nil, in, fetch, &tp.spec))
			require.Equal(t, sets.New(provider), tp.spec.extProc.providerNames)
			return tp
		}

		for _, prio := range []apiannotations.InheritedPolicyPriorityValue{
			apiannotations.DeepMergePreferChild,
			apiannotations.DeepMergePreferParent,
		} {
			child := policyFor(t, "child-provider")
			parent := policyFor(t, "parent-provider")

			merged := translate(child, parent, prio, "")
			require.Len(t, merged.spec.extProc.perProviderConfig, 2, "%s: merged policy should hold both providers", prio)

			assert.Equal(t, sets.New("child-provider"), child.spec.extProc.providerNames,
				"%s: the parent's provider must not be added to the child IR", prio)
			assert.Len(t, child.spec.extProc.perProviderConfig, 1,
				"%s: the parent's config must not be appended to the child IR", prio)
			assert.Len(t, parent.spec.extProc.perProviderConfig, 1,
				"%s: parent IR must be unchanged after merging", prio)
		}
	})

	t.Run("extAuth", func(t *testing.T) {
		policyFor := func(t *testing.T, provider string) *TrafficPolicy {
			t.Helper()
			tp := &TrafficPolicy{ct: time.Now()}
			in := &kgateway.TrafficPolicy{
				Spec: kgateway.TrafficPolicySpec{ExtAuth: &kgateway.ExtAuthPolicy{ExtensionRef: extensionRef}},
			}
			fetch := fetchExtension(provider, func(e *TrafficPolicyGatewayExtensionIR) {
				e.ExtAuth = &envoy_ext_authz_v3.ExtAuthz{}
			})
			require.NoError(t, constructExtAuth(nil, in, fetch, &tp.spec))
			require.Equal(t, sets.New(provider), tp.spec.extAuth.providerNames)
			return tp
		}

		for _, prio := range []apiannotations.InheritedPolicyPriorityValue{
			apiannotations.DeepMergePreferChild,
			apiannotations.DeepMergePreferParent,
		} {
			child := policyFor(t, "child-provider")
			parent := policyFor(t, "parent-provider")

			merged := translate(child, parent, prio, "")
			require.Len(t, merged.spec.extAuth.perProviderConfig, 2, "%s: merged policy should hold both providers", prio)

			assert.Equal(t, sets.New("child-provider"), child.spec.extAuth.providerNames,
				"%s: the parent's provider must not be added to the child IR", prio)
			assert.Len(t, child.spec.extAuth.perProviderConfig, 1,
				"%s: the parent's config must not be appended to the child IR", prio)
			assert.Len(t, parent.spec.extAuth.perProviderConfig, 1,
				"%s: parent IR must be unchanged after merging", prio)
		}
	})
}

func TestMergeHttpACL(t *testing.T) {
	p2Ref := &ir.AttachedPolicyRef{Name: "p2", Namespace: "default"}

	strPtr := func(s string) *string { return &s }
	int32Ptr := func(i int32) *int32 { return &i }

	makePolicy := func(t *testing.T, defaultAction shared.ACLAction, rules []shared.ACLRule, denyResponse *shared.ACLDenyResponse) *TrafficPolicy {
		t.Helper()
		tp := &TrafficPolicy{ct: time.Now()}
		k := &kgateway.TrafficPolicy{
			Spec: kgateway.TrafficPolicySpec{
				ACL: &shared.ACLPolicy{
					DefaultAction: defaultAction,
					Rules:         rules,
					DenyResponse:  denyResponse,
				},
			},
		}
		require.NoError(t, constructHttpACL(k, &tp.spec))
		return tp
	}

	extractJSON := func(t *testing.T, tp *TrafficPolicy) map[string]any {
		t.Helper()
		require.NotNil(t, tp.spec.httpACL)
		j, err := utils.AnyToJson(tp.spec.httpACL.config.FilterConfig)
		require.NoError(t, err)
		m, ok := j.(map[string]any)
		require.True(t, ok, "expected map[string]any")
		return m
	}

	assertRuleCIDR := func(t *testing.T, rules []any, idx int, expectedCIDR string) {
		t.Helper()
		require.Less(t, idx, len(rules), "rule index out of range")
		rule, ok := rules[idx].(map[string]any)
		require.True(t, ok, "rule at index %d is not a map", idx)
		cidrs, ok := rule["cidrs"].([]any)
		require.True(t, ok, "cidrs field is not a slice")
		require.Len(t, cidrs, 1)
		assert.Equal(t, expectedCIDR, cidrs[0])
	}

	assertHeader := func(t *testing.T, hdrs []any, idx int, expectedName, expectedValue string) {
		t.Helper()
		require.Less(t, idx, len(hdrs), "header index out of range")
		hdr, ok := hdrs[idx].(map[string]any)
		require.True(t, ok, "header at index %d is not a map", idx)
		assert.Equal(t, expectedName, hdr["name"])
		assert.Equal(t, expectedValue, hdr["value"])
	}

	t.Run("shallow augmented: p2 fills in when p1 empty", func(t *testing.T) {
		p1 := &TrafficPolicy{ct: time.Now()}
		p2 := makePolicy(t, shared.ACLActionAllow, nil, nil)
		origins := ir.MergeOrigins{}

		mergeHttpACL(p1, p2, p2Ref, nil, policy.MergeOptions{Strategy: policy.AugmentedShallowMerge}, origins, TrafficPolicyMergeOpts{})

		j := extractJSON(t, p1)
		assert.Equal(t, "allow", j["defaultAction"])
		// origins only get populated when p2 is used, so just checking it is not empty
		// and contains the httpACL key in the map is enough
		assert.Contains(t, origins, "httpACL", "p2 should be recorded as origin")
	})

	t.Run("shallow augmented: p1 wins when already set", func(t *testing.T) {
		p1 := makePolicy(t, shared.ACLActionDeny, nil, nil)
		p2 := makePolicy(t, shared.ACLActionAllow, nil, nil)
		origins := ir.MergeOrigins{}

		mergeHttpACL(p1, p2, p2Ref, nil, policy.MergeOptions{Strategy: policy.AugmentedShallowMerge}, origins, TrafficPolicyMergeOpts{})

		j := extractJSON(t, p1)
		assert.Equal(t, "deny", j["defaultAction"], "p1 should win when already set")
		assert.Empty(t, origins, "p2 should not appear in origins when overridden")
	})

	t.Run("shallow overridable: p2 always replaces p1", func(t *testing.T) {
		p1 := makePolicy(t, shared.ACLActionDeny, nil, nil)
		p2 := makePolicy(t, shared.ACLActionAllow, nil, nil)
		origins := ir.MergeOrigins{}

		mergeHttpACL(p1, p2, p2Ref, nil, policy.MergeOptions{Strategy: policy.OverridableShallowMerge}, origins, TrafficPolicyMergeOpts{})

		j := extractJSON(t, p1)
		assert.Equal(t, "allow", j["defaultAction"], "p2 should override p1")
		assert.Contains(t, origins, "httpACL")
	})

	t.Run("deep augmented: rules unioned, same defaultAction", func(t *testing.T) {
		p1 := makePolicy(t, shared.ACLActionDeny, []shared.ACLRule{
			{CIDRs: []shared.IPOrCIDR{"10.0.0.0/8"}, Action: shared.ACLActionAllow},
		}, nil)
		p2 := makePolicy(t, shared.ACLActionDeny, []shared.ACLRule{
			{CIDRs: []shared.IPOrCIDR{"192.168.0.0/16"}, Action: shared.ACLActionAllow},
		}, nil)
		origins := ir.MergeOrigins{}

		mergeHttpACL(p1, p2, p2Ref, nil, policy.MergeOptions{Strategy: policy.AugmentedDeepMerge}, origins, TrafficPolicyMergeOpts{})

		j := extractJSON(t, p1)
		assert.Equal(t, "deny", j["defaultAction"])
		rules, ok := j["rules"].([]any)
		require.True(t, ok)
		assert.Len(t, rules, 2, "rules from both policies should be unioned")
		// Even with deep merge where the result contains p1 and p2 rules, origins will still
		// only contain p2 as the only element in the map
		assert.Contains(t, origins, "httpACL")
		assert.Equal(t, 1, len(origins["httpACL"]))
	})

	t.Run("deep augmented: on defaultAction conflict, fallback to default shallow merge", func(t *testing.T) {
		p1 := makePolicy(t, shared.ACLActionDeny, []shared.ACLRule{
			{CIDRs: []shared.IPOrCIDR{"10.0.0.0/8"}, Action: shared.ACLActionAllow},
		}, nil)
		p2 := makePolicy(t, shared.ACLActionAllow, []shared.ACLRule{
			{CIDRs: []shared.IPOrCIDR{"192.168.0.0/16"}, Action: shared.ACLActionDeny},
		}, nil)
		origins := ir.MergeOrigins{}

		mergeHttpACL(p1, p2, p2Ref, nil, policy.MergeOptions{Strategy: policy.AugmentedDeepMerge}, origins, TrafficPolicyMergeOpts{})

		j := extractJSON(t, p1)
		assert.Equal(t, "deny", j["defaultAction"], "should stay with p1")
		rules, ok := j["rules"].([]any)
		require.True(t, ok)
		assert.Len(t, rules, 1, "should only use rules from p1")
		assertRuleCIDR(t, rules, 0, "10.0.0.0/8")
		assert.Empty(t, origins)
	})

	t.Run("deep overridable: on defaultAction conflict, fallback to default shallow merge", func(t *testing.T) {
		p1 := makePolicy(t, shared.ACLActionDeny, []shared.ACLRule{
			{CIDRs: []shared.IPOrCIDR{"10.0.0.0/8"}, Action: shared.ACLActionAllow},
		}, nil)
		p2 := makePolicy(t, shared.ACLActionAllow, []shared.ACLRule{
			{CIDRs: []shared.IPOrCIDR{"192.168.0.0/16"}, Action: shared.ACLActionDeny},
		}, nil)
		origins := ir.MergeOrigins{}

		mergeHttpACL(p1, p2, p2Ref, nil, policy.MergeOptions{Strategy: policy.OverridableDeepMerge}, origins, TrafficPolicyMergeOpts{})
		// When falling back to shallow merge due to conflict, the merge with keep "Augmented" or "Overridable" strategy
		// So, in this case, it will fall back to OverridableShallowMerge which is p2 wins

		j := extractJSON(t, p1)
		assert.Equal(t, "allow", j["defaultAction"], "should pick p2")
		rules, ok := j["rules"].([]any)
		require.True(t, ok)
		assert.Len(t, rules, 1, "should only use rules from p2")
		assertRuleCIDR(t, rules, 0, "192.168.0.0/16")
		assert.Contains(t, origins, "httpACL")
		assert.Contains(t, origins["httpACL"], "//default/p2")
	})

	t.Run("deep augmented: on defaultAction conflict, fallback to default shallow merge", func(t *testing.T) {
		p1 := makePolicy(t, shared.ACLActionDeny, []shared.ACLRule{
			{CIDRs: []shared.IPOrCIDR{"10.0.0.0/8"}, Action: shared.ACLActionAllow},
		}, nil)
		p2 := makePolicy(t, shared.ACLActionAllow, []shared.ACLRule{
			{CIDRs: []shared.IPOrCIDR{"192.168.0.0/16"}, Action: shared.ACLActionDeny},
		}, nil)
		origins := ir.MergeOrigins{}

		mergeHttpACL(p1, p2, p2Ref, nil, policy.MergeOptions{Strategy: policy.AugmentedDeepMerge}, origins, TrafficPolicyMergeOpts{})

		// When falling back to shallow merge due to conflict, the merge with keep "Augmented" or "Overridable" strategy
		// So, in this case, it will fall back to AugmentedShallowMerge which is p1 wins
		j := extractJSON(t, p1)
		assert.Equal(t, "deny", j["defaultAction"], "should stay with p1")
		rules, ok := j["rules"].([]any)
		require.True(t, ok)
		assert.Len(t, rules, 1, "should only use rules from p1")
		assertRuleCIDR(t, rules, 0, "10.0.0.0/8")
		assert.Empty(t, origins)
	})

	t.Run("deep augmented: p1 nil httpACL, p2 fills in fully", func(t *testing.T) {
		p1 := &TrafficPolicy{ct: time.Now()}
		p2 := makePolicy(t, shared.ACLActionAllow, []shared.ACLRule{
			{CIDRs: []shared.IPOrCIDR{"10.0.0.0/8"}, Action: shared.ACLActionDeny},
		}, nil)
		origins := ir.MergeOrigins{}

		mergeHttpACL(p1, p2, p2Ref, nil, policy.MergeOptions{Strategy: policy.AugmentedDeepMerge}, origins, TrafficPolicyMergeOpts{})

		j := extractJSON(t, p1)
		assert.Equal(t, "allow", j["defaultAction"])
		rules, ok := j["rules"].([]any)
		require.True(t, ok)
		assert.Len(t, rules, 1)
		assert.Contains(t, origins, "httpACL")
	})

	t.Run("deep augmented: denyResponse scalars merged, headers unioned", func(t *testing.T) {
		p1 := makePolicy(t, shared.ACLActionDeny, nil, &shared.ACLDenyResponse{
			StatusCode: int32Ptr(403),
			Headers:    []shared.ACLResponseHeader{{Name: "X-Block", Value: "1"}},
		})
		p2 := makePolicy(t, shared.ACLActionDeny, nil, &shared.ACLDenyResponse{
			StatusCode:          int32Ptr(403),
			Headers:             []shared.ACLResponseHeader{{Name: "X-Reason", Value: "geo"}},
			BlockedByHeaderName: strPtr("X-Blocked-By"),
		})
		origins := ir.MergeOrigins{}

		mergeHttpACL(p1, p2, p2Ref, nil, policy.MergeOptions{Strategy: policy.AugmentedDeepMerge}, origins, TrafficPolicyMergeOpts{})

		j := extractJSON(t, p1)
		dr, ok := j["denyResponse"].(map[string]any)
		require.True(t, ok, "denyResponse should be present")
		assert.Equal(t, float64(403), dr["statusCode"], "p1 statusCode wins")
		assert.Equal(t, "X-Blocked-By", dr["blockedByHeaderName"], "p2 blockedByHeaderName fills in")
		hdrs, ok := dr["headers"].([]any)
		require.True(t, ok)
		assert.Len(t, hdrs, 2, "headers should be unioned")
	})

	t.Run("deep augmented: p1 has no denyResponse, p2 fills in", func(t *testing.T) {
		p1 := makePolicy(t, shared.ACLActionDeny, nil, nil)
		p2 := makePolicy(t, shared.ACLActionDeny, nil, &shared.ACLDenyResponse{
			StatusCode: int32Ptr(451),
		})
		origins := ir.MergeOrigins{}

		mergeHttpACL(p1, p2, p2Ref, nil, policy.MergeOptions{Strategy: policy.AugmentedDeepMerge}, origins, TrafficPolicyMergeOpts{})

		j := extractJSON(t, p1)
		dr, ok := j["denyResponse"].(map[string]any)
		require.True(t, ok, "denyResponse should be filled from p2")
		assert.Equal(t, float64(451), dr["statusCode"])
	})

	t.Run("deep augmented: denyResponse with conflicting status fallback to default shallow merge", func(t *testing.T) {
		p1 := makePolicy(t, shared.ACLActionDeny, nil, &shared.ACLDenyResponse{
			Headers:    []shared.ACLResponseHeader{{Name: "X-Block-1", Value: "1"}},
			StatusCode: int32Ptr(403),
		})
		p2 := makePolicy(t, shared.ACLActionDeny, nil, &shared.ACLDenyResponse{
			Headers:    []shared.ACLResponseHeader{{Name: "X-Block-2", Value: "2"}},
			StatusCode: int32Ptr(451),
		})
		origins := ir.MergeOrigins{}

		mergeHttpACL(p1, p2, p2Ref, nil, policy.MergeOptions{Strategy: policy.AugmentedDeepMerge}, origins, TrafficPolicyMergeOpts{})
		// When falling back to shallow merge due to conflict, the merge with keep "Augmented" or "Overridable" strategy
		// So, in this case, it will fall back to AugmentedShallowMerge which is p1 wins

		j := extractJSON(t, p1)
		dr, ok := j["denyResponse"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, float64(403), dr["statusCode"], "fallback to default shallow merge because of conflict")
		hdrs, ok := dr["headers"].([]any)
		require.True(t, ok)
		assert.Len(t, hdrs, 1)
		assertHeader(t, hdrs, 0, "X-Block-1", "1")
	})

	t.Run("deep overridable: denyResponse with conflicting status fallback to default shallow merge", func(t *testing.T) {
		p1 := makePolicy(t, shared.ACLActionDeny, nil, &shared.ACLDenyResponse{
			Headers:    []shared.ACLResponseHeader{{Name: "X-Block-1", Value: "1"}},
			StatusCode: int32Ptr(403),
		})
		p2 := makePolicy(t, shared.ACLActionDeny, nil, &shared.ACLDenyResponse{
			Headers:    []shared.ACLResponseHeader{{Name: "X-Block-2", Value: "2"}},
			StatusCode: int32Ptr(451),
		})
		origins := ir.MergeOrigins{}

		mergeHttpACL(p1, p2, p2Ref, nil, policy.MergeOptions{Strategy: policy.OverridableDeepMerge}, origins, TrafficPolicyMergeOpts{})
		// When falling back to shallow merge due to conflict, the merge with keep "Augmented" or "Overridable" strategy
		// So, in this case, it will fall back to OverridableShallowMerge which is p2 wins

		j := extractJSON(t, p1)
		dr, ok := j["denyResponse"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, float64(451), dr["statusCode"], "fallback to default shallow merge because of conflict")
		hdrs, ok := dr["headers"].([]any)
		require.True(t, ok)
		assert.Len(t, hdrs, 1)
		assertHeader(t, hdrs, 0, "X-Block-2", "2")
	})

	t.Run("deep overridable: denyResponse with no conflict", func(t *testing.T) {
		p1 := makePolicy(t, shared.ACLActionDeny, nil, &shared.ACLDenyResponse{
			Headers:    []shared.ACLResponseHeader{{Name: "X-Block-1", Value: "1"}},
			StatusCode: int32Ptr(403),
		})
		p2 := makePolicy(t, shared.ACLActionDeny, nil, &shared.ACLDenyResponse{
			Headers:    []shared.ACLResponseHeader{{Name: "X-Block-2", Value: "2"}},
			StatusCode: int32Ptr(403),
		})
		origins := ir.MergeOrigins{}

		mergeHttpACL(p1, p2, p2Ref, nil, policy.MergeOptions{Strategy: policy.OverridableDeepMerge}, origins, TrafficPolicyMergeOpts{})

		j := extractJSON(t, p1)
		dr, ok := j["denyResponse"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, float64(403), dr["statusCode"])
		hdrs, ok := dr["headers"].([]any)
		require.True(t, ok)
		assert.Len(t, hdrs, 2, "headers should be merged from both policies")
	})

	t.Run("detectHttpACLMergeConflict: no conflict when defaultActions match", func(t *testing.T) {
		m1 := map[string]any{"defaultAction": "deny", "rules": []any{}}
		m2 := map[string]any{"defaultAction": "deny", "rules": []any{
			map[string]any{"cidrs": []any{"10.0.0.0/8"}, "action": "allow"},
		}}
		conflicts := detectHttpACLMergeConflict(m1, m2)
		assert.Empty(t, conflicts)
	})

	t.Run("detectHttpACLMergeConflict: conflict returned when defaultActions differ", func(t *testing.T) {
		m1 := map[string]any{"defaultAction": "deny"}
		m2 := map[string]any{"defaultAction": "allow"}
		conflicts := detectHttpACLMergeConflict(m1, m2)
		assert.Len(t, conflicts, 1)
		assert.Contains(t, conflicts[0].Error(), "defaultAction conflict")
	})
}

package trafficpolicy

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"

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

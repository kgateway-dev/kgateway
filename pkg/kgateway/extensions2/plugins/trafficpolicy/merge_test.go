package trafficpolicy

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"

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

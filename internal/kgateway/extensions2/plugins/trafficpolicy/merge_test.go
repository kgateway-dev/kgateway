package trafficpolicy

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
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

	merged := mergePolicies([]ir.PolicyAtt{p1, p2})
	require.Len(t, merged.Errors, 2)
	assert.Contains(t, merged.Errors, err1)
	assert.Contains(t, merged.Errors, err2)
}

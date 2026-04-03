package krtcollections

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func TestPreferPortSpecificBackendTLSPolicies(t *testing.T) {
	otherGK := schema.GroupKind{Group: "test.io", Kind: "ConnectionPolicy"}
	serviceWidePolicies := []ir.PolicyAtt{
		{GroupKind: wellknown.BackendTLSPolicyGVK.GroupKind()},
		{GroupKind: otherGK},
	}
	portPolicies := []ir.PolicyAtt{
		{GroupKind: wellknown.BackendTLSPolicyGVK.GroupKind()},
	}

	filtered := preferPortSpecificBackendTLSPolicies(serviceWidePolicies, portPolicies)

	require.Len(t, filtered, 1)
	require.Equal(t, otherGK, filtered[0].GroupKind)
}

package controller

import (
	"testing"

	"github.com/stretchr/testify/require"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
)

func TestOwnedObjectLabelSelector(t *testing.T) {
	// Unset, the watch must stay unfiltered so it shares the translation collection's informer.
	require.Empty(t, ownedObjectLabelSelector(""))

	// Set, the watch must select the proxy objects the envoy chart renders, independent of the
	// operator's selector, which those objects need not match.
	for _, selector := range []apisettings.LabelSelector{"team=a", "team in (a,b)", "kgateway.dev/watch=true"} {
		require.Equal(t, "app.kubernetes.io/managed-by=kgateway", ownedObjectLabelSelector(selector), string(selector))
	}
}

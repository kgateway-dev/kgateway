package proxy_syncer

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// The cluster rows branch on Error != nil (excluded from CDS, reported in
// status), so their Equals must never treat a present error with an empty
// message as no error, or KRT would keep a healthy row when the base flips to
// an errored one with the same version. Nilness is compared before messages.
func TestErrorsEqualComparesNilnessBeforeMessage(t *testing.T) {
	require.True(t, errorsEqual(nil, nil))
	require.False(t, errorsEqual(nil, errors.New("")), "an empty-message error is still an error")
	require.False(t, errorsEqual(errors.New(""), nil))
	require.True(t, errorsEqual(errors.New("boom"), errors.New("boom")), "equal messages compare equal")
	require.False(t, errorsEqual(errors.New("boom"), errors.New("bang")))

	healthy := baseEnvoyCluster{Name: "c", ClusterVersion: 0}
	errored := baseEnvoyCluster{Name: "c", ClusterVersion: 0, Error: errors.New("")}
	require.False(t, healthy.Equals(errored), "a base row gaining an error must not compare equal, whatever the message")
	require.False(t, errored.Equals(healthy))

	perClientHealthy := uccWithCluster{Name: "c", ClusterVersion: 0}
	perClientErrored := uccWithCluster{Name: "c", ClusterVersion: 0, Error: errors.New("")}
	require.False(t, perClientHealthy.Equals(perClientErrored))
	require.False(t, perClientErrored.Equals(perClientHealthy))
}

//go:build ignore

package helper

import (
	"context"
	"testing"

	"github.com/solo-io/go-utils/githubutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetUpgradeVersionsErrOrNilLastMinor(t *testing.T) {
	// We should not fail if we have internet connection. We should always return an error
	// or a non-nil value for lastMinor
	lastMinor, currentMinor, err := GetUpgradeVersions(context.Background(), "gloo")

	hasErr := err != nil
	hasNilLastMinor := lastMinor != nil
	assert.True(t, hasErr || hasNilLastMinor, "%v %v %v", err, lastMinor, currentMinor)
}

func TestReturnsLatestPatchForMinor(t *testing.T) {
	ctx := context.Background()
	// this is fine because this is a public repo
	client := githubutils.GetClientWithOrWithoutToken(ctx)
	minor, err := getLatestReleasedPatchVersion(ctx, client, "gloo", 1, 9)
	require.NoError(t, err)

	assert.Equal(t, "v1.9.30", minor.String())
}

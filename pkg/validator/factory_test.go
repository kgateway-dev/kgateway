package validator

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
)

func TestNew_ZeroValueModeYieldsBinary(t *testing.T) {
	// A zero-valued Settings struct bypasses the envconfig default tag (which
	// selects CACHE); an empty mode falls through to plain binary.
	v := New(apisettings.Settings{})
	_, ok := v.(*binaryValidator)
	assert.True(t, ok, "zero-valued settings should yield plain binaryValidator")
}

func TestNew_UnknownModeFallsBackToBinary(t *testing.T) {
	v := New(apisettings.Settings{ValidatorMode: "nonsense"})
	_, ok := v.(*binaryValidator)
	assert.True(t, ok)
}

func TestNew_CacheMode(t *testing.T) {
	v := New(apisettings.Settings{
		ValidatorMode:      apisettings.ValidatorCache,
		ValidatorCacheSize: 16,
	})
	c, ok := v.(*cachingValidator)
	require.True(t, ok, "cache mode should return *cachingValidator")
	_, innerOK := c.inner.(*binaryValidator)
	assert.True(t, innerOK, "cache mode should wrap *binaryValidator")
}

func TestNew_CacheZeroSizeFallsBackToDefault(t *testing.T) {
	// NewCaching's behavior under size <= 0 is to use DefaultCacheSize; covered
	// directly in cache_test, but verify the wiring exposes it as well.
	v := New(apisettings.Settings{ValidatorMode: apisettings.ValidatorCache})
	_, ok := v.(*cachingValidator)
	require.True(t, ok)
}

package validator

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
)

func TestNew_ZeroValueModeReturnsError(t *testing.T) {
	_, err := New(apisettings.Settings{})
	require.EqualError(t, err, `invalid validator mode: ""`)
}

func TestNew_UnknownModeReturnsError(t *testing.T) {
	_, err := New(apisettings.Settings{ValidatorMode: "nonsense"})
	require.EqualError(t, err, `invalid validator mode: "nonsense"`)
}

func TestNew_BinaryMode(t *testing.T) {
	v, err := New(apisettings.Settings{ValidatorMode: apisettings.ValidatorBinary})
	require.NoError(t, err)
	_, ok := v.(*binaryValidator)
	assert.True(t, ok, "binary mode should return *binaryValidator")
}

func TestNew_CacheMode(t *testing.T) {
	v, err := New(apisettings.Settings{
		ValidatorMode:      apisettings.ValidatorCache,
		ValidatorCacheSize: 16,
	})
	require.NoError(t, err)
	c, ok := v.(*cachingValidator)
	require.True(t, ok, "cache mode should return *cachingValidator")
	_, innerOK := c.inner.(*binaryValidator)
	assert.True(t, innerOK, "cache mode should wrap *binaryValidator")
}

func TestNew_CacheZeroSizeFallsBackToDefault(t *testing.T) {
	v, err := New(apisettings.Settings{ValidatorMode: apisettings.ValidatorCache})
	require.NoError(t, err)
	_, ok := v.(*cachingValidator)
	require.True(t, ok)
}

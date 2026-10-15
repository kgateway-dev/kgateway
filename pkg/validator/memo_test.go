package validator

import (
	"context"
	"errors"
	"fmt"
	"testing"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/structpb"
)

func stringStruct(v string) *structpb.Struct {
	return &structpb.Struct{Fields: map[string]*structpb.Value{"v": structpb.NewStringValue(v)}}
}

func memoTestCluster(name string, md map[string]string) *envoyclusterv3.Cluster {
	c := &envoyclusterv3.Cluster{Name: name}
	if md != nil {
		c.Metadata = &envoycorev3.Metadata{}
		// A map field, so the key must not depend on iteration order.
		c.Metadata.FilterMetadata = map[string]*structpb.Struct{}
		for k, v := range md {
			c.Metadata.FilterMetadata[k] = stringStruct(v)
		}
	}
	return c
}

func TestContentKeyOf_StableAndDistinct(t *testing.T) {
	md := map[string]string{"a": "1", "b": "2", "c": "3", "d": "4", "e": "5"}
	k1, err := ContentKeyOf(memoTestCluster("c", md))
	require.NoError(t, err)
	for range 20 {
		k2, err := ContentKeyOf(memoTestCluster("c", md))
		require.NoError(t, err)
		assert.Equal(t, k1, k2, "equal messages must produce equal keys regardless of map order")
	}
	k3, err := ContentKeyOf(memoTestCluster("c", map[string]string{"a": "1"}))
	require.NoError(t, err)
	assert.NotEqual(t, k1, k3, "different content must produce different keys")
	k4, err := ContentKeyOf(memoTestCluster("other", md))
	require.NoError(t, err)
	assert.NotEqual(t, k1, k4, "different name must produce a different key")
}

func TestMemo_HitsAndMisses(t *testing.T) {
	m := NewMemo(16)
	calls := 0
	run := func(context.Context) error { calls++; return nil }

	k1, _ := ContentKeyOf(memoTestCluster("a", nil))
	for range 3 {
		require.NoError(t, m.Validate(context.Background(), k1, run))
	}
	assert.Equal(t, 1, calls, "identical content must run the validator once")

	k2, _ := ContentKeyOf(memoTestCluster("b", nil))
	require.NoError(t, m.Validate(context.Background(), k2, run))
	assert.Equal(t, 2, calls, "distinct content must run the validator")
}

func TestMemo_MemoizesInvalidVerdict(t *testing.T) {
	m := NewMemo(16)
	calls := 0
	run := func(context.Context) error { calls++; return fmt.Errorf("%w: bad cluster cfg", ErrInvalidXDS) }

	k, _ := ContentKeyOf(memoTestCluster("a", nil))
	err1 := m.Validate(context.Background(), k, run)
	err2 := m.Validate(context.Background(), k, run)
	require.ErrorIs(t, err1, ErrInvalidXDS)
	require.ErrorIs(t, err2, ErrInvalidXDS, "memoized verdict must keep ErrInvalidXDS identity")
	assert.Equal(t, err1.Error(), err2.Error(), "memoized message must match the original")
	assert.Equal(t, 1, calls, "an invalid verdict must be memoized")
}

func TestMemo_DoesNotMemoizeTransientErrors(t *testing.T) {
	m := NewMemo(16)
	calls := 0
	transient := errors.New("envoy validate invocation failed: exec format error")
	run := func(context.Context) error { calls++; return transient }

	k, _ := ContentKeyOf(memoTestCluster("a", nil))
	require.ErrorIs(t, m.Validate(context.Background(), k, run), transient)
	require.ErrorIs(t, m.Validate(context.Background(), k, run), transient)
	assert.Equal(t, 2, calls, "transient errors must run the validator every time")

	run = func(context.Context) error { calls++; return nil }
	require.NoError(t, m.Validate(context.Background(), k, run))
	require.NoError(t, m.Validate(context.Background(), k, run))
	assert.Equal(t, 3, calls, "a later success must be memoized")
}

func TestMemo_NilMemoRunsValidator(t *testing.T) {
	var m *Memo
	calls := 0
	run := func(context.Context) error { calls++; return nil }
	k, _ := ContentKeyOf(memoTestCluster("a", nil))
	require.NoError(t, m.Validate(context.Background(), k, run))
	require.NoError(t, m.Validate(context.Background(), k, run))
	assert.Equal(t, 2, calls, "a nil memo must be a passthrough")
}

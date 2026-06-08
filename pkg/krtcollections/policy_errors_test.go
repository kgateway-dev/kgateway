package krtcollections

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestUnknownBackendKindError(t *testing.T) {
	// The error classification in pkg/kgateway/query and pkg/kgateway/translator/irtranslator
	// relies on errors.Is(err, ErrUnknownBackendKind), so the typed error must keep matching it.
	t.Run("matches the ErrUnknownBackendKind sentinel", func(t *testing.T) {
		err := &UnknownBackendKindError{GroupKind: schema.GroupKind{Group: "example.com", Kind: "Foo"}}
		assert.ErrorIs(t, err, ErrUnknownBackendKind)
	})

	t.Run("message includes the kind and group", func(t *testing.T) {
		err := &UnknownBackendKindError{GroupKind: schema.GroupKind{Group: "example.com", Kind: "Foo"}}
		assert.Equal(t, `unknown backend kind "Foo" in group "example.com"`, err.Error())
	})

	t.Run("message omits the empty core group", func(t *testing.T) {
		err := &UnknownBackendKindError{GroupKind: schema.GroupKind{Kind: "ConfigMap"}}
		assert.Equal(t, `unknown backend kind "ConfigMap"`, err.Error())
	})
}

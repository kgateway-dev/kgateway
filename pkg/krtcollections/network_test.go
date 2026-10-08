package krtcollections

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"istio.io/istio/pkg/kube/krt"
)

func TestFetchSystemNamespaceNetwork(t *testing.T) {
	assert.Empty(t, FetchSystemNamespaceNetwork(krt.TestingDummyContext{}, nil), "nil singleton")
	assert.Empty(t, FetchSystemNamespaceNetwork(krt.TestingDummyContext{}, krt.NewStatic[string](nil, true)), "unset network")
	assert.Equal(t, "n1", FetchSystemNamespaceNetwork(krt.TestingDummyContext{}, krt.NewStatic(new("n1"), true)))
}

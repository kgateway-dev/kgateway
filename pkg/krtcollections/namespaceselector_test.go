package krtcollections

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"istio.io/istio/pkg/kube/krt"
	klabels "k8s.io/apimachinery/pkg/labels"
)

func TestNamespaceSelectorReturnsFalseForMissingNamespace(t *testing.T) {
	namespaces := krt.NewStaticCollection[NamespaceMetadata](nil, nil)

	matches := NamespaceSelector(namespaces, klabels.SelectorFromSet(klabels.Set{
		"team": "blue",
	}))

	assert.False(t, matches(krt.TestingDummyContext{}, "missing"), "missing namespaces should not match selectors")
}

func TestNamespaceSelectorMatchesMissingNamespaceForMatchAllSelector(t *testing.T) {
	namespaces := krt.NewStaticCollection[NamespaceMetadata](nil, nil)

	matches := NamespaceSelector(namespaces, klabels.Everything())

	assert.True(t, matches(krt.TestingDummyContext{}, "missing"), "missing namespaces should match an empty selector")
}

func TestNamespaceSelectorMatchesExistingNamespace(t *testing.T) {
	namespaces := krt.NewStaticCollection[NamespaceMetadata](nil, []NamespaceMetadata{{
		Name: "default",
		Labels: map[string]string{
			"team": "blue",
		},
	}})

	matches := NamespaceSelector(namespaces, klabels.SelectorFromSet(klabels.Set{
		"team": "blue",
	}))

	assert.True(t, matches(krt.TestingDummyContext{}, "default"), "existing namespaces with matching labels should match selectors")
}

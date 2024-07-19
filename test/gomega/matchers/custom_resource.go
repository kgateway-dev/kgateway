package matchers

import (
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	"github.com/onsi/gomega/types"
	"k8s.io/apimachinery/pkg/runtime/schema"

	k8stypes "k8s.io/apimachinery/pkg/types"
)

// HaveObjectMeta returns a GomegaMatcher which matches a struct that has the provided name/namespace
// This should be used when asserting that a CustomResource has a provided name/namespace
func HaveObjectMeta(namespacedName k8stypes.NamespacedName) types.GomegaMatcher {
	return gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
		"Name":          Equal(namespacedName.Name),
		"Namespace":     Equal(namespacedName.Namespace),
		"ManagedFields": BeEmpty(),
	})
}

// HaveTypeMeta returns a GomegaMatcher which matches a struct that has the provide Group/Version/Kind
func HaveTypeMeta(gvk schema.GroupVersionKind) types.GomegaMatcher {
	return gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
		"APIVersion": Equal(gvk.GroupVersion().String()),
		"Kind":       Equal(gvk.Kind),
	})
}

// ContainCustomResource returns a GomegaMatcher which matches resource in a list if the provided
// typeMeta, objectMeta and spec matchers match
func ContainCustomResource(typeMetaMatcher, objectMetaMatcher, specMatcher types.GomegaMatcher) types.GomegaMatcher {
	return ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
		"TypeMeta":   typeMetaMatcher,
		"ObjectMeta": objectMetaMatcher,
		"Spec":       specMatcher,
	}))
}

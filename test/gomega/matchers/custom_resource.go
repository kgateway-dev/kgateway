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
func HaveObjectMeta(namespacedName k8stypes.NamespacedName, additionalMetaMatchers ...types.GomegaMatcher) types.GomegaMatcher {
	nameNamespaceMatcher := gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
		"Name":      Equal(namespacedName.Name),
		"Namespace": Equal(namespacedName.Namespace),
	})

	return And(append(additionalMetaMatchers, nameNamespaceMatcher)...)
}

func HaveNilManagedFields() types.GomegaMatcher {
	return gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
		"ManagedFields": BeNil(),
	})
}

func HaveAFields() types.GomegaMatcher {
	return gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
		"ManagedFields": BeNil(),
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
	return ContainElement(MatchCustomResource(typeMetaMatcher, objectMetaMatcher, specMatcher))
}

// MatchCustomResource returns a GomegaMatcher which matches a resource if the provided  typeMeta, objectMeta and spec matchers match
func MatchCustomResource(typeMetaMatcher, objectMetaMatcher, specMatcher types.GomegaMatcher) types.GomegaMatcher {
	return gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
		"TypeMeta":   typeMetaMatcher,
		"ObjectMeta": objectMetaMatcher,
		"Spec":       gstruct.PointTo(specMatcher),
	})
}

// ContainCustomResourceType returns a GomegaMatcher which matches resource in a list if the provided
// typeMeta match
func ContainCustomResourceType(gvk schema.GroupVersionKind) types.GomegaMatcher {
	return ContainCustomResource(HaveTypeMeta(gvk), gstruct.Ignore(), gstruct.Ignore())
}

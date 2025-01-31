//go:build ignore

package matchers

import (
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	"github.com/onsi/gomega/types"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stypes "k8s.io/apimachinery/pkg/types"
)

// MatchObjectMeta returns a GomegaMatcher which matches a struct that has the provided name/namespace
// This should be used when asserting that a CustomResource has a provided name/namespace
func MatchObjectMeta(namespacedName k8stypes.NamespacedName, additionalMetaMatchers ...types.GomegaMatcher) types.GomegaMatcher {
	nameNamespaceMatcher := gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
		"Name":      Equal(namespacedName.Name),
		"Namespace": Equal(namespacedName.Namespace),
	})

	return And(append(additionalMetaMatchers, nameNamespaceMatcher)...)
}

// HaveNilManagedFields returns a GomegaMatcher which matches a struct that has a `ManagedFields` property, which is nil
// This should be used with the above MatchObjectMeta, and can be passed as an additionalMetaMatcher:
// MatchObjectMeta(NamespacedName{Name:n,Namespace:ns}, HaveNilManagedFields())
func HaveNilManagedFields() types.GomegaMatcher {
	return gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
		"ManagedFields": BeNil(),
	})
}

// MatchTypeMeta returns a GomegaMatcher which matches a struct that has the provide Group/Version/Kind
func MatchTypeMeta(gvk schema.GroupVersionKind) types.GomegaMatcher {
	return gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
		"APIVersion": Equal(gvk.GroupVersion().String()),
		"Kind":       Equal(gvk.Kind),
	})
}

// ContainCustomResource returns a GomegaMatcher which matches resource in a list if the provided
// typeMeta, objectMeta and spec matchers match
// This method is purely syntactic sugar around combining ContainElement and MatchCustomResource
func ContainCustomResource(typeMetaMatcher, objectMetaMatcher, specMatcher types.GomegaMatcher) types.GomegaMatcher {
	return ContainElement(MatchCustomResource(typeMetaMatcher, objectMetaMatcher, specMatcher))
}

// MatchCustomResource returns a GomegaMatcher which matches a resource if the provided  typeMeta, objectMeta and spec matchers match
// CAUTION TO DEVELOPERS!!
// When passing the specMatcher, keep in mind that the Spec is a pointer, so if you are asserting some behavior,
// you will likely need to wrap your matcher in: gstruct.PointTo({matcher})
func MatchCustomResource(typeMetaMatcher, objectMetaMatcher, specMatcher types.GomegaMatcher) types.GomegaMatcher {
	return gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
		"TypeMeta":   typeMetaMatcher,
		"ObjectMeta": objectMetaMatcher,
		"Spec":       specMatcher,
	})
}

// ContainCustomResourceType returns a GomegaMatcher which matches resource in a list if the provided
// typeMeta match
func ContainCustomResourceType(gvk schema.GroupVersionKind) types.GomegaMatcher {
	return ContainCustomResource(MatchTypeMeta(gvk), gstruct.Ignore(), gstruct.Ignore())
}

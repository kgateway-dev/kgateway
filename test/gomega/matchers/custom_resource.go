package matchers

import (
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	"github.com/onsi/gomega/types"
	"k8s.io/apimachinery/pkg/runtime/schema"

	k8s_types "k8s.io/apimachinery/pkg/types"
)

func HaveObjectMeta(namespacedName k8s_types.NamespacedName) types.GomegaMatcher {
	return gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
		"Name":          Equal(namespacedName.Name),
		"Namespace":     Equal(namespacedName.Namespace),
		"ManagedFields": BeEmpty(),
	})
}

func HaveTypeMeta(gvk schema.GroupVersionKind) types.GomegaMatcher {
	return gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
		"APIVersion": Equal(gvk.GroupVersion().String()),
		"Kind":       Equal(gvk.Kind),
	})
}

func ContainCustomResource(typeMetaMatcher, objectMetaMatcher, specMatcher types.GomegaMatcher) types.GomegaMatcher {
	return ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
		"TypeMeta":   typeMetaMatcher,
		"ObjectMeta": objectMetaMatcher,
		"Spec":       specMatcher,
	}))
}

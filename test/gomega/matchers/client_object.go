package matchers

import (
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	k8stypes "k8s.io/apimachinery/pkg/types"
)

func MatchClientObject(gvk schema.GroupVersionKind, namespacedName k8stypes.NamespacedName, customMatchers ...types.GomegaMatcher) types.GomegaMatcher {
	standardMatcher := And(
		HaveNameAndNamespace(namespacedName.Name, namespacedName.Namespace),
		MatchClientObjectGvk(gvk),
	)
	return And(append(customMatchers, standardMatcher)...)
}

func MatchClientObjectGvk(gvk schema.GroupVersionKind) types.GomegaMatcher {
	return WithTransform(func(object client.Object) schema.GroupVersionKind { return object.GetObjectKind().GroupVersionKind() }, Equal(gvk))
}

// HaveNameAndNamespace returns a matcher that will match a pointer to a client.Object
// with the given name and namespace
func HaveNameAndNamespace(name string, namespace string) types.GomegaMatcher {
	return And(
		WithTransform(func(object client.Object) string { return object.GetName() }, Equal(name)),
		WithTransform(func(object client.Object) string { return object.GetNamespace() }, Equal(namespace)),
	)
}

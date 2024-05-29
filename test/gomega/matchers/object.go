package matchers

import (
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	"github.com/onsi/gomega/types"
)

// HaveNameAndNamespace returns a matcher that will match a pointer to a client.Object
// with the given name and namespace
func HaveNameAndNamespace(name string, namespace string) types.GomegaMatcher {
	return gstruct.PointTo(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
		"ObjectMeta": gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
			"Name":      gomega.Equal(name),
			"Namespace": gomega.Equal(namespace),
		}),
	}))
}

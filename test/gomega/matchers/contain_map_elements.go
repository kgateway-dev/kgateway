package matchers

import (
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
)

// ContainMapElements produces a matcher that will only match if all provided map elements
// are completely accounted for. The actual value is expected to not be nil or empty since
// there are other, more appropriate matchers for those cases.
func ContainMapElements[keyT comparable, valT any](m map[keyT]valT) types.GomegaMatcher {
	subMatchers := []types.GomegaMatcher{
		gomega.Not(gomega.BeNil()),
		gomega.Not(gomega.BeEmpty()),
	}
	for k, v := range m {
		subMatchers = append(subMatchers, gomega.HaveKeyWithValue(k, v))
	}
	return gomega.And(subMatchers...)
}

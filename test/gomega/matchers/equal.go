package matchers

import (
	"encoding/json"
	"fmt"

	"github.com/onsi/gomega/types"

	"github.com/onsi/gomega/matchers"
	"github.com/sergi/go-diff/diffmatchpatch"
)

var (
	_ types.GomegaMatcher = new(BeEquivalentToDiffMatcher)
)

// BeEquivalentToDiff is the same as BeEquivalentTo
// but prints a nice diff on failure best effect use ginkgo with -noColor
func BeEquivalentToDiff(expected interface{}) *BeEquivalentToDiffMatcher {
	return &BeEquivalentToDiffMatcher{
		BeEquivalentToMatcher: matchers.BeEquivalentToMatcher{
			Expected: expected,
		},
	}
}

type BeEquivalentToDiffMatcher struct {
	matchers.BeEquivalentToMatcher
}

func (matcher *BeEquivalentToDiffMatcher) FailureMessage(actual interface{}) (message string) {
	return fmt.Sprintf("%s\ndiff: %s", matcher.BeEquivalentToMatcher.FailureMessage(actual), diff(matcher.Expected, actual))
}

func (matcher *BeEquivalentToDiffMatcher) NegatedFailureMessage(actual interface{}) (message string) {
	return fmt.Sprintf("%s\ndiff: %s", matcher.BeEquivalentToMatcher.NegatedFailureMessage(actual), diff(matcher.Expected, actual))
}

func diff(expected, actual interface{}) string {
	jsonexpected, _ := json.MarshalIndent(expected, "", "  ")
	jsonactual, _ := json.MarshalIndent(actual, "", "  ")
	dmp := diffmatchpatch.New()
	rawDiff := dmp.DiffMain(string(jsonactual), string(jsonexpected), true)
	return dmp.DiffPrettyText(rawDiff)
}

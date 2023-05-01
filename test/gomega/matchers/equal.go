package matchers

import (
	"encoding/json"
	"github.com/sergi/go-diff/diffmatchpatch"
)

func diff(expected, actual interface{}) string {
	jsonexpected, _ := json.MarshalIndent(expected, "", "  ")
	jsonactual, _ := json.MarshalIndent(actual, "", "  ")
	dmp := diffmatchpatch.New()
	rawDiff := dmp.DiffMain(string(jsonactual), string(jsonexpected), true)
	return dmp.DiffPrettyText(rawDiff)
}

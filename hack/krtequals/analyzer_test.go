package krtequals

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestDeepEqualCheckEnabled(t *testing.T) {
	testdata := analysistest.TestData()
	analyser := newAnalyzer(&Config{DeepEqual: true})
	analysistest.Run(t, testdata, analyser, "deepequalon")
}

func TestDeepEqualCheckDisabled(t *testing.T) {
	testdata := analysistest.TestData()
	analyser := newAnalyzer(&Config{DeepEqual: false})
	analysistest.Run(t, testdata, analyser, "deepequaloff")
}

func TestMarkers(t *testing.T) {
	testdata := analysistest.TestData()
	analyser := newAnalyzer(&Config{})
	analysistest.Run(t, testdata, analyser, "markers")
}

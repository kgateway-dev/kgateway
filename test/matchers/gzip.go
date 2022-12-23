package matchers

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io/ioutil"

	"github.com/onsi/gomega/types"
)

// HaveDecompressedValue returns a GomegaMatcher which validates that a set of bytes can be decompressed
// and match a particular string
func HaveDecompressedValue(expectedUncompressedBody string) types.GomegaMatcher {
	return &GzipResponseMatcher{
		expectedUncompressedBody: expectedUncompressedBody,
	}
}

type GzipResponseMatcher struct {
	expectedUncompressedBody string
}

func (g *GzipResponseMatcher) Match(actual interface{}) (success bool, err error) {
	actualResponseBytes, ok := actual.([]byte)
	if !ok {
		return false, fmt.Errorf("expected an string, got %+v", actual)
	}

	reader, err := gzip.NewReader(bytes.NewBuffer(actualResponseBytes))
	if err != nil {
		return false, fmt.Errorf("failed to create gzip reader: %v", err)
	}
	defer reader.Close()

	body, err := ioutil.ReadAll(reader)
	if err != nil {
		return false, fmt.Errorf("failed to read gzip reader: %v", err)
	}

	if string(body) != g.expectedUncompressedBody {
		return false, fmt.Errorf("expected %s, got %s", g.expectedUncompressedBody, string(body))
	}

	return true, nil
}

func (g *GzipResponseMatcher) FailureMessage(actual interface{}) (message string) {
	return fmt.Sprintf("Expected\n%s\n%s\n%s", formatActualResponseBytes(actual), "to have compressed bytes", g.expectedUncompressedBody)
}

func (g *GzipResponseMatcher) NegatedFailureMessage(actual interface{}) (message string) {
	return fmt.Sprintf("Expected\n%s\n%s\n%s", formatActualResponseBytes(actual), "not to have compressed bytes", g.expectedUncompressedBody)
}

func formatActualResponseBytes(input interface{}) string {
	return string(input.([]byte))
}

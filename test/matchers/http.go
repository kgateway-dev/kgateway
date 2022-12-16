package matchers

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/onsi/gomega/format"
	"github.com/onsi/gomega/types"
)

func MatchHttpResponse(expected interface{}) types.GomegaMatcher {
	return &MatchHttpResponseMatcher{
		Expected: expected,
	}
}

type MatchHttpResponseMatcher struct {
	Expected interface{}
}

func (m *MatchHttpResponseMatcher) Match(actual interface{}) (success bool, err error) {
	if actual == nil {
		return false, errors.New("expected an http.response, got nil")
	}

	actualResponse, isHttpResponse := actual.(*http.Response)
	expectedResponse := m.Expected.(*http.Response)

	if !isHttpResponse {
		return false, fmt.Errorf("Expected an http.response.  Got:\n%s", format.Object(actual, 1))
	}

	if actualResponse.StatusCode != expectedResponse.StatusCode {
		return false, fmt.Errorf("Expected StatusCode :%d.  Got:%d", expectedResponse.StatusCode, actualResponse.StatusCode)
	}

	return true, nil
}

func (m *MatchHttpResponseMatcher) FailureMessage(actual interface{}) (message string) {
	return format.Message(actual, "to match response", m.Expected)
}

func (m *MatchHttpResponseMatcher) NegatedFailureMessage(actual interface{}) (message string) {
	return format.Message(actual, "not to match response", m.Expected)
}

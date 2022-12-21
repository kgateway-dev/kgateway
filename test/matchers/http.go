package matchers

import (
	"fmt"

	"github.com/onsi/gomega/matchers"
	"github.com/onsi/gomega/types"
)

type HttpResponse struct {
	StatusCode int
	Body       interface{}
}

func MatchHttpResponse(expected *HttpResponse) types.GomegaMatcher {
	expectedBody := expected.Body
	if expectedBody == nil {
		// Default to an empty body
		expectedBody = ""
	}

	return &MatchHttpResponseMatcher{
		Expected: expected,
		HaveHTTPStatusMatcher: matchers.HaveHTTPStatusMatcher{
			Expected: []interface{}{
				expected.StatusCode,
			},
		},
		HaveHTTPBodyMatcher: matchers.HaveHTTPBodyMatcher{
			Expected: expectedBody,
		},
	}
}

type MatchHttpResponseMatcher struct {
	Expected *HttpResponse
	matchers.HaveHTTPStatusMatcher
	matchers.HaveHTTPBodyMatcher

	// TODO (sam-heilbron) Add support matchers.HaveHTTPHeaderWithValueMatcher
}

func (m *MatchHttpResponseMatcher) Match(actual interface{}) (success bool, err error) {
	if ok, matchStatusErr := m.HaveHTTPStatusMatcher.Match(actual); !ok {
		return false, matchStatusErr
	}

	if ok, matchBodyErr := m.HaveHTTPBodyMatcher.Match(actual); !ok {
		return false, matchBodyErr
	}

	return true, nil
}

func (m *MatchHttpResponseMatcher) FailureMessage(actual interface{}) (message string) {
	return fmt.Sprintf("%s\n%s\ndiff: %s",
		m.HaveHTTPStatusMatcher.FailureMessage(actual),
		m.HaveHTTPBodyMatcher.FailureMessage(actual),
		diff(m.Expected, actual))
}

func (m *MatchHttpResponseMatcher) NegatedFailureMessage(actual interface{}) (message string) {
	return fmt.Sprintf("%s\n%s\ndiff: %s",
		m.HaveHTTPStatusMatcher.NegatedFailureMessage(actual),
		m.HaveHTTPBodyMatcher.NegatedFailureMessage(actual),
		diff(m.Expected, actual))
}

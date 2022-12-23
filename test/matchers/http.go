package matchers

import (
	"fmt"
	"net/http"

	"github.com/onsi/gomega"

	"github.com/onsi/gomega/matchers"
	"github.com/onsi/gomega/types"
)

// HaveOkResponse expects a 200 response with an empty body
func HaveOkResponse() types.GomegaMatcher {
	return HaveHttpResponse(&HttpResponse{
		StatusCode: http.StatusOK,
		Body:       "",
	})
}

// HaveStatusCode expects an http response with a particular status code and an empty body
func HaveStatusCode(statusCode int) types.GomegaMatcher {
	return HaveHttpResponse(&HttpResponse{
		StatusCode: statusCode,
		Body:       "",
	})
}

// HaveExactResponseBody expects a 200 response with a body that matches the provided string
func HaveExactResponseBody(body string) types.GomegaMatcher {
	return HaveHttpResponse(&HttpResponse{
		StatusCode: http.StatusOK,
		Body:       body,
	})
}

// HavePartialResponseBody expects a 200 response with a body that contains the provided substring
func HavePartialResponseBody(substring string) types.GomegaMatcher {
	return HaveHttpResponse(&HttpResponse{
		StatusCode: http.StatusOK,
		Body:       gomega.ContainSubstring(substring),
	})
}

// HttpResponse defines the set of properties that we can validate from an http.Response
type HttpResponse struct {
	// StatusCode is the expected status code for an http.Response
	StatusCode int
	// Body is the expected response body for an http.Response
	// Body can be of type: {string, bytes, GomegaMatcher}
	Body interface{}
}

// HaveHttpResponse returns a GomegaMatcher which validates that an http.Response contains
// particular expected properties (status, body..etc)
// If an expected body isn't defined, we default to expecting an empty response
func HaveHttpResponse(expected *HttpResponse) types.GomegaMatcher {
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

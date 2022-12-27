package matchers

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"

	"github.com/onsi/gomega/matchers"
	"github.com/onsi/gomega/types"
)

// HaveOkResponse expects a 200 response with an empty body
func HaveOkResponse() types.GomegaMatcher {
	return HaveHttpResponse(&HttpResponse{
		StatusCode: http.StatusOK,
		Body:       gomega.BeEmpty(),
	})
}

// HaveStatusCode expects an http response with a particular status code and an empty body
func HaveStatusCode(statusCode int) types.GomegaMatcher {
	return HaveHttpResponse(&HttpResponse{
		StatusCode: statusCode,
		Body:       gomega.BeEmpty(),
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

// HaveOkResponseWithHeaders expects an 200 response with a set of headers that match the provided headers
func HaveOkResponseWithHeaders(headers map[string]interface{}) types.GomegaMatcher {
	return HaveHttpResponse(&HttpResponse{
		StatusCode: http.StatusOK,
		Body:       gomega.BeEmpty(),
		Headers:    headers,
	})
}

// HttpResponse defines the set of properties that we can validate from an http.Response
type HttpResponse struct {
	// StatusCode is the expected status code for an http.Response
	// Required
	StatusCode int
	// Body is the expected response body for an http.Response
	// Body can be of type: {string, bytes, GomegaMatcher}
	// Optional: If not provided, defaults to an empty string
	Body interface{}
	// Headers is the set of expected header values for an http.Response
	// Each header can be of type: {string, GomegaMatcher}
	// Optional: If not provided, does not perform header validation
	Headers map[string]interface{}
	// Custom is a generic matcher that can be applied to validate any other properties of an http.Response
	// Optional: If not provided, does not perform additional validation
	Custom types.GomegaMatcher
}

// HaveHttpResponse returns a GomegaMatcher which validates that an http.Response contains
// particular expected properties (status, body..etc)
// If an expected body isn't defined, we default to expecting an empty response
// NOTE TO DEVELOPERS: The underlying matchers cache properties, so if you are wrapping this
// matcher in an Eventually, be sure to create a new matcher each iteration.
func HaveHttpResponse(expected *HttpResponse) types.GomegaMatcher {
	expectedBody := expected.Body
	if expectedBody == nil {
		// Default to an empty body
		expectedBody = ""
	}

	var headerMatchers []matchers.HaveHTTPHeaderWithValueMatcher
	for headerName, headerMatch := range expected.Headers {
		// HttpHeaderWithValueMatcher uses Header.Get under the hood, which only extracts
		// the first value associated with a given header
		headerMatchers = append(headerMatchers, matchers.HaveHTTPHeaderWithValueMatcher{
			Header: headerName,
			Value:  headerMatch,
		})
	}

	expectedCustomMatcher := expected.Custom
	if expected.Custom == nil {
		// Default to an always accept matcher
		expectedCustomMatcher = gstruct.Ignore()
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
		headerMatchers: headerMatchers,
		customMatcher:  expectedCustomMatcher,
	}
}

type MatchHttpResponseMatcher struct {
	Expected *HttpResponse
	matchers.HaveHTTPStatusMatcher
	matchers.HaveHTTPBodyMatcher

	headerMatchers []matchers.HaveHTTPHeaderWithValueMatcher

	customMatcher types.GomegaMatcher
}

func (m *MatchHttpResponseMatcher) Match(actual interface{}) (success bool, err error) {
	if ok, matchStatusErr := m.HaveHTTPStatusMatcher.Match(actual); !ok {
		return false, matchStatusErr
	}

	if ok, matchBodyErr := m.HaveHTTPBodyMatcher.Match(actual); !ok {
		return false, matchBodyErr
	}

	for _, headerMatcher := range m.headerMatchers {
		if ok, headerMatchErr := headerMatcher.Match(actual); !ok {
			return false, headerMatchErr
		}
	}

	if ok, customMatchErr := m.customMatcher.Match(actual); !ok {
		return false, customMatchErr
	}

	return true, nil
}

func (m *MatchHttpResponseMatcher) FailureMessage(actual interface{}) (message string) {
	return fmt.Sprintf("%s\n%s\n%s\n%s\n\ndiff: %s",
		m.HaveHTTPStatusMatcher.FailureMessage(actual),
		m.HaveHTTPBodyMatcher.FailureMessage(actual),
		m.headersFailureMessage(actual),
		m.customMatcher.FailureMessage(actual),
		diff(m.Expected, actual))
}

func (m *MatchHttpResponseMatcher) NegatedFailureMessage(actual interface{}) (message string) {
	return fmt.Sprintf("%s\n%s\n%s\n%s\n\ndiff: %s",
		m.HaveHTTPStatusMatcher.NegatedFailureMessage(actual),
		m.HaveHTTPBodyMatcher.NegatedFailureMessage(actual),
		m.headersNegatedFailureMessage(actual),
		m.customMatcher.NegatedFailureMessage(actual),
		diff(m.Expected, actual))
}

func (m *MatchHttpResponseMatcher) headersFailureMessage(actual interface{}) (message string) {
	var lines []string
	for _, headerMatcher := range m.headerMatchers {
		lines = append(lines, headerMatcher.FailureMessage(actual))
	}
	return strings.Join(lines, "\n")
}

func (m *MatchHttpResponseMatcher) headersNegatedFailureMessage(actual interface{}) (message string) {
	var lines []string
	for _, headerMatcher := range m.headerMatchers {
		lines = append(lines, headerMatcher.NegatedFailureMessage(actual))
	}
	return strings.Join(lines, "\n")
}

package matchers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	"github.com/onsi/gomega/types"
)

var _ types.GomegaMatcher = new(HaveHttpRequestMatcher)

// HaveRequestWithHeaders expects a set of headers that match the provided headers
func HaveRequestWithHeaders(headers map[string]interface{}) types.GomegaMatcher {
	return HaveHttpRequest(&HttpRequest{
		Headers: headers,
	})
}

// HaveRequestWithoutHeaders expects a request that does not contain the specified headers
func HaveRequestWithoutHeaders(headerNames ...string) types.GomegaMatcher {
	return HaveHttpRequest(&HttpRequest{
		NotHeaders: headerNames,
	})
}

// HttpRequest defines the set of properties that we can validate from an http.Request
type HttpRequest struct {
	// Method is the expected request method (eg GET, POST ...etc)
	// Optional: If not provided, does not perform method validation
	Method string
	// Path is the expected request path (including any qs param)
	// Optional: If not provided, does not perform path validation
	Path string

	// Body is the expected request body for an http.Request
	// Body can be of type: {string, bytes, GomegaMatcher}
	// Optional: If not provided, defaults to an empty string
	// TODO: currently, the http echo service we use in our test does not
	//       return the request body. So, this is not implemented yet and commented out
	// Body interface{}

	// Headers is the set of expected header values for an http.Request
	// Each header can be of type: {string, GomegaMatcher}
	// Optional: If not provided, does not perform header validation
	Headers map[string]interface{}
	// NotHeaders is a list of headers that should not be present in the request
	// Optional: If not provided, does not perform header absence validation
	NotHeaders []string
	// Custom is a generic matcher that can be applied to validate any other properties of an http.Request
	// Optional: If not provided, does not perform additional validation
	Custom types.GomegaMatcher
}

func (r *HttpRequest) String() string {
	return fmt.Sprintf("HttpRequest{Method: %s, Path: %s, Headers: %v, NotHeaders: %v, Custom: %v}",
		r.Method, r.Path, r.Headers, r.NotHeaders, r.Custom)
}

// HaveHttpRequest returns a GomegaMatcher which validates that an http.Request contains
// particular expected properties (method, body..etc)
// If an expected body isn't specified, the body is not matched
func HaveHttpRequest(expected *HttpRequest) types.GomegaMatcher {
	expectedCustomMatcher := expected.Custom
	if expected.Custom == nil {
		// Default to an always accept matcher
		expectedCustomMatcher = gstruct.Ignore()
	}

	var partialRequestMatchers []types.GomegaMatcher
	if len(expected.Method) > 0 {
		partialRequestMatchers = append(partialRequestMatchers, &HaveHTTPRequestHeaderWithValueMatcher{
			Header: ":method",
			Value:  expected.Method,
		})
	}
	if len(expected.Path) > 0 {
		partialRequestMatchers = append(partialRequestMatchers, &HaveHTTPRequestHeaderWithValueMatcher{
			Header: ":path",
			Value:  expected.Path,
		})
	}
	for headerName, headerMatch := range expected.Headers {
		partialRequestMatchers = append(partialRequestMatchers, &HaveHTTPRequestHeaderWithValueMatcher{
			Header: headerName,
			Value:  headerMatch,
		})
	}
	for _, headerName := range expected.NotHeaders {
		partialRequestMatchers = append(partialRequestMatchers, &NotHaveHTTPRequestHeaderMatcher{
			Header: headerName,
		})
	}
	partialRequestMatchers = append(partialRequestMatchers, expectedCustomMatcher)

	return &HaveHttpRequestMatcher{
		Expected:       expected,
		requestMatcher: gomega.And(partialRequestMatchers...),
	}
}

type HaveHttpRequestMatcher struct {
	Expected *HttpRequest

	requestMatcher types.GomegaMatcher
}

func (m *HaveHttpRequestMatcher) Match(actual interface{}) (success bool, err error) {
	if ok, matchErr := m.requestMatcher.Match(actual); !ok {
		return false, matchErr
	}

	return true, nil
}

func (m *HaveHttpRequestMatcher) FailureMessage(actual interface{}) (message string) {
	return fmt.Sprintf("%s \n%s",
		m.requestMatcher.FailureMessage(actual),
		informativeRequestComparison(m.Expected, actual))
}

func (m *HaveHttpRequestMatcher) NegatedFailureMessage(actual interface{}) (message string) {
	return fmt.Sprintf("%s \n%s",
		m.requestMatcher.NegatedFailureMessage(actual),
		informativeRequestComparison(m.Expected, actual))
}

// HaveHTTPRequestHeaderWithValueMatcher is a matcher that checks if a header exists and match the value in the HTTP request
type HaveHTTPRequestHeaderWithValueMatcher struct {
	Header string
	Value  any
}

func (m *HaveHTTPRequestHeaderWithValueMatcher) Match(actual interface{}) (success bool, err error) {
	request, ok := actual.(*http.Request)
	if !ok {
		return false, fmt.Errorf("HaveHTTPRequestHeaderWithValueMatcher expects an *http.Request, got %T", actual)
	}

	if request == nil {
		return false, errors.New("HaveHTTPRequestHeaderWithValueMatcher matcher requires a non-nil *http.Request")
	}

	values := request.Header.Values(m.Header)
	valuesMap := make(map[string]bool)
	for _, value := range values {
		valuesMap[value] = true
	}

	switch expected := m.Value.(type) {
	case string:
		return valuesMap[expected], nil
	case []string:
		for _, v := range expected {
			if !valuesMap[v] {
				return false, nil
			}
		}
		return true, nil

	default:
		return false, errors.New("HaveHTTPRequestHeaderWithValueMatcher only supports string or []string value")
	}
}

func (m *HaveHTTPRequestHeaderWithValueMatcher) FailureMessage(actual interface{}) string {
	request, ok := actual.(*http.Request)
	if !ok || request == nil {
		return fmt.Sprintf("Expected a valid *http.Request, got %T", actual)
	}

	values := request.Header.Values(m.Header)
	if len(values) == 0 {
		return fmt.Sprintf("Expected HTTP request to have header '%s: %s', but it does not exist", m.Header, m.Value)
	}
	return fmt.Sprintf("Expected HTTP request to have header '%s: %s', but the value does not match. Actual: %v", m.Header, m.Value, values)
}

func (m *HaveHTTPRequestHeaderWithValueMatcher) NegatedFailureMessage(actual interface{}) string {
	request, ok := actual.(*http.Request)
	if !ok || request == nil {
		return fmt.Sprintf("Expected a valid *http.Request, got %T", actual)
	}

	return fmt.Sprintf("Expected HTTP request to not have header '%s: %s', but it was not present", m.Header, m.Value)
}

// NotHaveHTTPRequestHeaderMatcher is a matcher that checks if a header is not present in the HTTP request
type NotHaveHTTPRequestHeaderMatcher struct {
	Header string
}

func (m *NotHaveHTTPRequestHeaderMatcher) Match(actual interface{}) (success bool, err error) {
	request, ok := actual.(*http.Request)
	if !ok {
		return false, fmt.Errorf("NotHaveHTTPRequestHeaderMatcher expects an *http.Request, got %T", actual)
	}

	if request == nil {
		return false, errors.New("NotHaveHTTPRequestHeaderMatcher matcher requires a non-nil *http.Request")
	}

	_, headerExists := request.Header[http.CanonicalHeaderKey(m.Header)]
	return !headerExists, nil
}

func (m *NotHaveHTTPRequestHeaderMatcher) FailureMessage(actual interface{}) string {
	request, ok := actual.(*http.Request)
	if !ok || request == nil {
		return fmt.Sprintf("Expected a valid *http.Request, got %T", actual)
	}

	return fmt.Sprintf("Expected HTTP request not to have header '%s', but it was present", m.Header)
}

func (m *NotHaveHTTPRequestHeaderMatcher) NegatedFailureMessage(actual interface{}) string {
	request, ok := actual.(*http.Request)
	if !ok || request == nil {
		return fmt.Sprintf("Expected a valid *http.Request, got %T", actual)
	}

	return fmt.Sprintf("Expected HTTP request to have header '%s', but it was not present", m.Header)
}

// informativeRequestComparison returns a string which presents data to the user to help them understand why a failure occurred.
// The HaveHttpRequestMatcher uses an And matcher, which intentionally short-circuits and only
// logs the first failure that occurred.
// To help developers, we print more details in this function.
func informativeRequestComparison(expected, actual interface{}) string {
	expectedJson, _ := json.MarshalIndent(expected, "", "  ")

	request, ok := actual.(*http.Request)
	if !ok || request == nil {
		return fmt.Sprintf("\nexpected: %s", expectedJson)
	}
	return fmt.Sprintf("\nexpected: %s actual request headers: %v", expectedJson, request.Header)
}

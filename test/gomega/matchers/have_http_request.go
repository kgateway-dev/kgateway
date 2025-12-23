package matchers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/onsi/gomega"
	"github.com/onsi/gomega/format"
	"github.com/onsi/gomega/gstruct"
	"github.com/onsi/gomega/matchers"
	"github.com/onsi/gomega/types"
)

var _ types.GomegaMatcher = new(HaveHttpRequestMatcher)

// HaveRequestWithHeaders expects a set of headers that match the provided headers
func HaveRequestWithHeaders(headers map[string]any) types.GomegaMatcher {
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
	Body any

	// Headers is the set of expected header values for an http.Request
	// Each header can be of type: {string, GomegaMatcher}
	// Optional: If not provided, does not perform header validation
	Headers map[string]any
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
	if expected.Body != nil {
		partialRequestMatchers = append(partialRequestMatchers, &HaveHTTPRequestBodyMatcher{
			Expected: expected.Body,
		})
	}

	return &HaveHttpRequestMatcher{
		Expected:       expected,
		requestMatcher: gomega.And(partialRequestMatchers...),
	}
}

type HaveHttpRequestMatcher struct {
	Expected *HttpRequest

	requestMatcher types.GomegaMatcher
}

func (m *HaveHttpRequestMatcher) Match(actual any) (success bool, err error) {
	if ok, matchErr := m.requestMatcher.Match(actual); !ok {
		return false, matchErr
	}

	return true, nil
}

func (m *HaveHttpRequestMatcher) FailureMessage(actual any) (message string) {
	return fmt.Sprintf("%s \n%s",
		m.requestMatcher.FailureMessage(actual),
		informativeRequestComparison(m.Expected, actual))
}

func (m *HaveHttpRequestMatcher) NegatedFailureMessage(actual any) (message string) {
	return fmt.Sprintf("%s \n%s",
		m.requestMatcher.NegatedFailureMessage(actual),
		informativeRequestComparison(m.Expected, actual))
}

// HaveHTTPRequestHeaderWithValueMatcher is a matcher that checks if a header exists and match the value in the HTTP request
type HaveHTTPRequestHeaderWithValueMatcher struct {
	Header string
	Value  any
}

func (m *HaveHTTPRequestHeaderWithValueMatcher) Match(actual any) (success bool, err error) {
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

	case types.GomegaMatcher:
		for _, value := range values {
			matched, _ := expected.Match(value)
			if matched {
				return true, nil
			}
		}
		return false, nil

	default:
		return false, errors.New("HaveHTTPRequestHeaderWithValueMatcher only supports string, []string, GomegaMatcher value")
	}
}

func (m *HaveHTTPRequestHeaderWithValueMatcher) FailureMessage(actual any) string {
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

func (m *HaveHTTPRequestHeaderWithValueMatcher) NegatedFailureMessage(actual any) string {
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

func (m *NotHaveHTTPRequestHeaderMatcher) Match(actual any) (success bool, err error) {
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

func (m *NotHaveHTTPRequestHeaderMatcher) FailureMessage(actual any) string {
	request, ok := actual.(*http.Request)
	if !ok || request == nil {
		return fmt.Sprintf("Expected a valid *http.Request, got %T", actual)
	}

	return fmt.Sprintf("Expected HTTP request not to have header '%s', but it was present", m.Header)
}

func (m *NotHaveHTTPRequestHeaderMatcher) NegatedFailureMessage(actual any) string {
	request, ok := actual.(*http.Request)
	if !ok || request == nil {
		return fmt.Sprintf("Expected a valid *http.Request, got %T", actual)
	}

	return fmt.Sprintf("Expected HTTP request to have header '%s', but it was not present", m.Header)
}

type HaveHTTPRequestBodyMatcher struct {
	Expected      any
	cachedRequest any
	cachedBody    []byte
}

func (matcher *HaveHTTPRequestBodyMatcher) Match(actual any) (bool, error) {
	body, err := matcher.body(actual)
	if err != nil {
		return false, err
	}

	switch e := matcher.Expected.(type) {
	case string:
		return (&matchers.EqualMatcher{Expected: e}).Match(string(body))
	case []byte:
		return (&matchers.EqualMatcher{Expected: e}).Match(body)
	case types.GomegaMatcher:
		return e.Match(body)
	default:
		return false, fmt.Errorf("HaveHTTPBody matcher expects string, []byte, or GomegaMatcher. Got:\n%s", format.Object(matcher.Expected, 1))
	}
}

func (matcher *HaveHTTPRequestBodyMatcher) FailureMessage(actual any) (message string) {
	body, err := matcher.body(actual)
	if err != nil {
		return fmt.Sprintf("failed to read body: %s", err)
	}

	switch e := matcher.Expected.(type) {
	case string:
		return (&matchers.EqualMatcher{Expected: e}).FailureMessage(string(body))
	case []byte:
		return (&matchers.EqualMatcher{Expected: e}).FailureMessage(body)
	case types.GomegaMatcher:
		return e.FailureMessage(body)
	default:
		return fmt.Sprintf("HaveHTTPBody matcher expects string, []byte, or GomegaMatcher. Got:\n%s", format.Object(matcher.Expected, 1))
	}
}

func (matcher *HaveHTTPRequestBodyMatcher) NegatedFailureMessage(actual any) (message string) {
	body, err := matcher.body(actual)
	if err != nil {
		return fmt.Sprintf("failed to read body: %s", err)
	}

	switch e := matcher.Expected.(type) {
	case string:
		return (&matchers.EqualMatcher{Expected: e}).NegatedFailureMessage(string(body))
	case []byte:
		return (&matchers.EqualMatcher{Expected: e}).NegatedFailureMessage(body)
	case types.GomegaMatcher:
		return e.NegatedFailureMessage(body)
	default:
		return fmt.Sprintf("HaveHTTPBody matcher expects string, []byte, or GomegaMatcher. Got:\n%s", format.Object(matcher.Expected, 1))
	}
}

// body returns the body. It is cached because once we read it in Match()
// the Reader is closed and it is not readable again in FailureMessage()
// or NegatedFailureMessage()
func (matcher *HaveHTTPRequestBodyMatcher) body(actual any) ([]byte, error) {
	if matcher.cachedRequest == actual && matcher.cachedBody != nil {
		return matcher.cachedBody, nil
	}

	body := func(a *http.Request) ([]byte, error) {
		if a.Body != nil {
			defer a.Body.Close()
			var err error
			matcher.cachedBody, err = io.ReadAll(a.Body)
			if err != nil {
				return nil, fmt.Errorf("error reading request body: %w", err)
			}
		}
		return matcher.cachedBody, nil
	}

	switch a := actual.(type) {
	case *http.Request:
		matcher.cachedRequest = a
		return body(a)
	default:
		return nil, fmt.Errorf("HaveHTTPRequestBody matcher expects *http.Request. Got:\n%s", format.Object(actual, 1))
	}
}

// informativeRequestComparison returns a string which presents data to the user to help them understand why a failure occurred.
// The HaveHttpRequestMatcher uses an And matcher, which intentionally short-circuits and only
// logs the first failure that occurred.
// To help developers, we print more details in this function.
func informativeRequestComparison(expected, actual any) string {
	expectedJson, _ := json.MarshalIndent(expected, "", "  ")

	request, ok := actual.(*http.Request)
	if !ok || request == nil {
		return fmt.Sprintf("\nexpected: %s", expectedJson)
	}
	return fmt.Sprintf("\nexpected: %s actual request headers: %v", expectedJson, request.Header)
}

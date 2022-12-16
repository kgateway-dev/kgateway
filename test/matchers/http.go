package matchers

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/onsi/gomega/format"
	"github.com/onsi/gomega/types"
)

type HttpResponse struct {
	*http.Response
	Body string
}

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
	expectedResponse := m.Expected.(*HttpResponse)

	if !isHttpResponse {
		return false, fmt.Errorf("Expected an http.response.  Got:\n%s", format.Object(actual, 1))
	}

	if ok, matchStatusCodeErr := m.matchStatusCode(expectedResponse.StatusCode, actualResponse); !ok {
		return false, matchStatusCodeErr
	}

	if ok, matchBodyErr := m.matchBody(expectedResponse.Body, actualResponse); !ok {
		return false, matchBodyErr
	}

	return true, nil
}

func (m *MatchHttpResponseMatcher) matchStatusCode(expectedStatusCode int, actualResponse *http.Response) (success bool, err error) {
	if actualResponse.StatusCode != expectedStatusCode {
		return false, fmt.Errorf("Expected StatusCode :%d.  Got:%d", expectedStatusCode, actualResponse.StatusCode)
	}
	return true, nil
}

func (m *MatchHttpResponseMatcher) matchBody(expectedBody string, actualResponse *http.Response) (success bool, err error) {
	defer actualResponse.Body.Close()
	actualBody, parseErr := ioutil.ReadAll(actualResponse.Body)
	if parseErr != nil {
		return false, fmt.Errorf("Expected Body :%v.  Encountered error while parsing:%v", expectedBody, parseErr)
	}

	if string(actualBody) != expectedBody {
		return false, fmt.Errorf("Expected Body :%s.  Got:%s", expectedBody, actualBody)
	}

	return true, nil
}

func (m *MatchHttpResponseMatcher) FailureMessage(actual interface{}) (message string) {
	return format.Message(actual, "to match response", m.Expected)
}

func (m *MatchHttpResponseMatcher) NegatedFailureMessage(actual interface{}) (message string) {
	return format.Message(actual, "not to match response", m.Expected)
}

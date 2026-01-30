//go:build e2e

package transformation

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
	reports "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/testutils/helper"
	envoyadmincli "github.com/kgateway-dev/kgateway/v2/test/envoyutils/admincli"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/helpers"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

const (
	httpbin_echo_base_path = "/anything/:anything"
)

var (
	// manifests
	simpleServiceManifest                = filepath.Join(fsutils.MustGetThisDir(), "testdata", "service.yaml")
	gatewayManifest                      = filepath.Join(fsutils.MustGetThisDir(), "testdata", "gateway.yaml")
	transformForCustomFunctionsManifest  = filepath.Join(fsutils.MustGetThisDir(), "testdata", "transform-for-custom-functions.yaml")
	transformForHeadersManifest          = filepath.Join(fsutils.MustGetThisDir(), "testdata", "transform-for-headers.yaml")
	transformForPseudoHeadersManifest    = filepath.Join(fsutils.MustGetThisDir(), "testdata", "transform-for-pseudo-headers.yaml")
	transformForBodyJsonManifest         = filepath.Join(fsutils.MustGetThisDir(), "testdata", "transform-for-body-json.yaml")
	rustformationForBodyJsonManifest     = filepath.Join(fsutils.MustGetThisDir(), "testdata", "transform-for-body-json-rust.yaml")
	transformForBodyAsStringManifest     = filepath.Join(fsutils.MustGetThisDir(), "testdata", "transform-for-body-as-string.yaml")
	gatewayAttachedTransformManifest     = filepath.Join(fsutils.MustGetThisDir(), "testdata", "gateway-attached-transform.yaml")
	transformForMatchPathManifest        = filepath.Join(fsutils.MustGetThisDir(), "testdata", "transform-for-match-path.yaml")
	transformForMatchHeaderManifest      = filepath.Join(fsutils.MustGetThisDir(), "testdata", "transform-for-match-header.yaml")
	transformForMatchQueryManifest       = filepath.Join(fsutils.MustGetThisDir(), "testdata", "transform-for-match-query.yaml")
	transformForMatchMethodManifest      = filepath.Join(fsutils.MustGetThisDir(), "testdata", "transform-for-match-method.yaml")
	transformForHeaderToBodyJsonManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "transform-for-header-to-body-json.yaml")
	transformForBodyLocalReplyManifest   = filepath.Join(fsutils.MustGetThisDir(), "testdata", "transform-for-body-local-reply.yaml")

	proxyObjectMeta = metav1.ObjectMeta{
		Name:      "gw",
		Namespace: "default",
	}

	// test cases
	setup = base.TestCase{
		Manifests: []string{
			defaults.CurlPodManifest,
			simpleServiceManifest,
			gatewayManifest,
			transformForCustomFunctionsManifest,
			transformForHeadersManifest,
			transformForPseudoHeadersManifest,
			transformForBodyAsStringManifest,
			gatewayAttachedTransformManifest,
			transformForMatchHeaderManifest,
			transformForMatchMethodManifest,
			transformForMatchPathManifest,
			transformForMatchQueryManifest,
			transformForHeaderToBodyJsonManifest,
			transformForBodyLocalReplyManifest,
		},
	}

	// Because the jinja template syntax are slightly different between C++ and rust when
	// accessing the json object after parsing the body as json, we need to use different
	// resources for the same test case when switching between the C++ (classic transformation)
	// and Rust (rustformation). Also because there is no hook in the testsuite frame work
	// to run custom function right before applying the resource, if you look at the log from envoy
	// you will see something like this:
	// [2025-11-17 15:37:40.956][1][warning][config]
	// [external/envoy/source/extensions/config_subscription/grpc/grpc_subscription_impl.cc:138]
	// gRPC config for type.googleapis.com/envoy.config.route.v3.RouteConfiguration rejected:
	// Failed to parse response template: Failed to parse header template 'from-incoming':
	// [inja.exception.parser_error] (at 1:67) malformed expression
	// This is because envoy is still configured to use the classic transformation while the rust
	// specific resource is applied. Once the rust test starts, it will switch envoy to the
	// rust dynamic module filter and the route will be accepted (and the error will go away)
	testCases = map[string]*base.TestCase{
		"TestGatewayWithTransformedRoute": {
			Manifests: []string{
				transformForBodyJsonManifest,
			},
		},
		"TestGatewayRustformationsWithTransformedRoute": {
			Manifests: []string{
				rustformationForBodyJsonManifest,
			},
		},
	}
)

type transformationTestCase struct {
	name      string
	routeName string
	opts      []curl.Option
	resp      *testmatchers.HttpResponse
	req       *testmatchers.HttpRequest
	// with go-httpbin, cannot use curl.WithPath directly in opts because we
	// need to add a path prefix (anything/:anything) to get the request data.
	// so, use the to add something to the path if you need to match it in the
	// test
	url string
}

// testingSuite is a suite of basic routing / "happy path" tests
type testingSuite struct {
	*base.BaseTestingSuite
	// testcases that are common between the classic transformation (c++) and rustformation
	// once the rustformation is in feature parity with the classic transformation,
	// they should both just use this.
	commonTestCases []transformationTestCase
}

// select specific test cases to run. Mainly for speeding up local testing
// when working on a specific test case. By default, when indices is empty,
// it returns all test cases. -1 index select the last one.
func selectCommonTestCases(indices ...int) []transformationTestCase {
	commonTestCases := []transformationTestCase{
		{
			// test 0
			name:      "basic-gateway-attached",
			routeName: "gateway-attached-transform",
			opts: []curl.Option{
				// in testdata/gateway-attached-transform.yaml,
				//    for x-empty, the value is set to ""
				//    for x-not-set, the value is not set
				// The behavior for both is removing the existing header
				// Testing this to make sure rustformation behaves the same
				curl.WithHeader("x-empty", "not empty"),
				curl.WithHeader("x-not-set", "set"),
			},
			resp: &testmatchers.HttpResponse{
				StatusCode: http.StatusOK,
				Headers: map[string]any{
					"response-gateway": "goodbye",
				},
				NotHeaders: []string{
					"x-foo-response",
				},
			},
			req: &testmatchers.HttpRequest{
				Headers: map[string]any{
					"request-gateway": "hello",
				},
				NotHeaders: []string{
					"x-not-set",
					"x-empty",
				},
			},
		},
		{
			// test 1
			name:      "basic",
			routeName: "headers",
			opts: []curl.Option{
				curl.WithBody("hello"),
				curl.WithHeader("cookie", "foo=bar"),
				curl.WithHeader("User-Agent", "curl/8.18.0"),
			},
			resp: &testmatchers.HttpResponse{
				StatusCode: http.StatusOK,
				Headers: map[string]any{
					"x-foo-response":        "notsuper",
					"x-foo-response-status": "200",
					// These are commented out so the testcase will pass on both classic and rustformation
					// and left here for documentation purpose
					// There should be a space at the beginning and end but
					// rust minijinja template rendering seems to right trim the space at the end
					// "x-space-test": " foobar",
					// while C++ inja leave the space untouched.
					// "x-space-test": " foobar ",
					// The http-bin response has "*" and we added "foo.com" in the policy. The library combined
					// them with a ','

					// REMOVE-ENVOY-1.37: Add header is no-op for arm build, so comment this out for now until after we upgrade to ENVOY-1.37
					// "access-control-allow-origin": "*,foo.com",
				},
				NotHeaders: []string{
					"response-gateway",
				},
			},
			req: &testmatchers.HttpRequest{
				Headers: map[string]any{
					"x-foo-bar":  "foolen_5",
					"x-foo-bar2": "foolen_5",
					// There should be a space at the beginning and end but
					// there might be a side effect from the echo server where the header values are trimmed
					"x-space-test": "foobar",
					"x-client":     "text",

					// REMOVE-ENVOY-1.37: Add header is no-op for arm build, so comment this out for now until after we upgrade to ENVOY-1.37
					// "cookie":       []string{"foo=bar", "test=123"},
				},
				NotHeaders: []string{
					// looks like the way we set up transformation targeting gateway, we are
					// also using RouteTransformation instead of FilterTransformation and it's
					// set , so it's set at the route table level and if there is a more specific
					// transformation (eg in vhost or prefix match), the gateway attached transformation
					// will not apply. Make sure it's not there.
					"request-gateway",
				},
			},
		},
		{
			// test 2
			name:      "remove headers",
			routeName: "headers",
			opts: []curl.Option{
				curl.WithBody("hello"),
				curl.WithHeader("x-remove-me", "test"),
				curl.WithHeader("x-dont-remove-me", "in request"),
			},
			resp: &testmatchers.HttpResponse{
				StatusCode: http.StatusOK,
				// go-httpbin doesn't allow setting custom response header, so make sure
				// we get one of the default access-control header and removed the other
				Headers: map[string]any{
					// REMOVE-ENVOY-1.37: Add header is no-op for arm build, so comment this out for now until after we upgrade to ENVOY-1.37
					// "access-control-allow-origin": "*,foo.com",
				},
				NotHeaders: []string{
					"access-control-allow-credentials",
				},
			},
			req: &testmatchers.HttpRequest{
				Headers: map[string]any{
					"x-dont-remove-me": "in request",
				},
				NotHeaders: []string{
					"x-remove-me",
				},
			},
		},
		{
			// test 3
			name:      "set headers with headers already exists multiple times",
			routeName: "headers",
			opts: []curl.Option{
				curl.WithBody("hello"),
				// The 2 x-foo-bar headers will be replaced with a single one when we set the header
				// to a value using transformation
				curl.WithMultiHeader("x-foo-bar", []string{"original_1", "original_2"}),
			},
			resp: &testmatchers.HttpResponse{
				StatusCode: http.StatusOK,
				Headers: map[string]any{
					"x-foo-response":        "notsuper",
					"x-foo-response-status": "200",
				},
				NotHeaders: []string{
					"response-gateway",
				},
			},
			req: &testmatchers.HttpRequest{
				Headers: map[string]any{
					"x-foo-bar":  "foolen_5",
					"x-foo-bar2": "foolen_5",
				},
				NotHeaders: []string{
					// looks like the way we set up transformation targeting gateway, we are
					// also using RouteTransformation instead of FilterTransformation and it's
					// set , so it's set at the route table level and if there is a more specific
					// transformation (eg in vhost or prefix match), the gateway attached transformation
					// will not apply. Make sure it's not there.
					"request-gateway",
				},
				Body: "hello",
			},
		},
		{
			// test 4
			name:      "conditional set by request header", // inja and the request_header function in use
			routeName: "headers",
			opts: []curl.Option{
				curl.WithBody("hello-world"),
				curl.WithHeader("x-add-bar", "super"),
			},
			resp: &testmatchers.HttpResponse{
				StatusCode: http.StatusOK,
				Headers: map[string]any{
					"x-foo-response":        "supersupersuper",
					"x-foo-response-status": "200",
				},
			},
			req: &testmatchers.HttpRequest{
				Headers: map[string]any{
					"x-foo-bar":  "foolen_11",
					"x-foo-bar2": "foolen_11",
				},
				NotHeaders: []string{
					// looks like the way we set up transformation targeting gateway, we are
					// also using RouteTransformation instead of FilterTransformation and it's
					// set , so it's set at the route table level and if there is a more specific
					// transformation (eg in vhost or prefix match), the gateway attached transformation
					// will not apply. Make sure it's not there.
					"request-gateway",
				},
			},
		},
		{
			// test 5
			// When all matching criterion are met, path match takes precedence
			name:      "match-all",
			routeName: "match",
			opts: []curl.Option{
				curl.WithHeader("foo", "bar"),
				curl.WithQueryParameters(map[string]string{"test": "123"}),
			},
			url: "/path_match/index.html",
			resp: &testmatchers.HttpResponse{
				StatusCode: http.StatusOK,
				Headers: map[string]any{
					"x-foo-response":  "path matched",
					"x-path-response": "matched",
				},
				NotHeaders: []string{
					"response-gateway",
					"x-method-response",
					"x-header-response",
					"x-query-response",
				},
			},
			req: &testmatchers.HttpRequest{
				Headers: map[string]any{
					"x-foo-request":  "path matched",
					"x-path-request": "matched",
				},
				NotHeaders: []string{
					"request-gateway",
					"x-method-request",
					"x-header-request",
					"x-query-request",
				},
			},
		},
		{
			// test 6
			// When all matching criterion are met except path, method match takes precedence
			name:      "match-method-header-and-query",
			routeName: "match",
			opts: []curl.Option{
				curl.WithHeader("foo", "bar"),
				curl.WithQueryParameters(map[string]string{"test": "123"}),
			},
			resp: &testmatchers.HttpResponse{
				StatusCode: http.StatusOK,
				Headers: map[string]any{
					"x-foo-response":    "method matched",
					"x-method-response": "matched",
				},
				NotHeaders: []string{
					"response-gateway",
					"x-path-response",
					"x-header-response",
					"x-query-response",
				},
			},
			req: &testmatchers.HttpRequest{
				Headers: map[string]any{
					"x-foo-request":    "method matched",
					"x-method-request": "matched",
				},
				NotHeaders: []string{
					"request-gateway",
					"x-path-request",
					"x-header-request",
					"x-query-request",
				},
			},
		},
		{
			// test 7
			// When all matching criterion are met except path and method, header match takes precedence
			name:      "match-header-and-query",
			routeName: "match",
			opts: []curl.Option{
				curl.WithBody("hello"),
				curl.WithHeader("foo", "bar"),
				curl.WithQueryParameters(map[string]string{"test": "123"}),
			},
			resp: &testmatchers.HttpResponse{
				StatusCode: http.StatusOK,
				Headers: map[string]any{
					"x-foo-response":    "header matched",
					"x-header-response": "matched",
				},
				NotHeaders: []string{
					"response-gateway",
					"x-path-response",
					"x-method-response",
					"x-query-response",
				},
			},
			req: &testmatchers.HttpRequest{
				Headers: map[string]any{
					"x-foo-request":    "header matched",
					"x-header-request": "matched",
				},
				NotHeaders: []string{
					"request-gateway",
					"x-path-request",
					"x-method-request",
					"x-query-request",
				},
			},
		},
		{
			// test 8
			name:      "match-query",
			routeName: "match",
			opts: []curl.Option{
				curl.WithBody("hello"),
				curl.WithQueryParameters(map[string]string{"test": "123"}),
			},
			resp: &testmatchers.HttpResponse{
				StatusCode: http.StatusOK,
				Headers: map[string]any{
					"x-foo-response":   "query matched",
					"x-query-response": "matched",
				},
				NotHeaders: []string{
					"response-gateway",
					"x-path-response",
					"x-method-response",
					"x-header-response",
				},
			},
			req: &testmatchers.HttpRequest{
				Headers: map[string]any{
					"x-foo-request":   "query matched",
					"x-query-request": "matched",
				},
				NotHeaders: []string{
					"request-gateway",
					"x-path-request",
					"x-method-request",
					"x-header-request",
				},
			},
		},
		{
			// test 9
			// Interesting Note: because when a transformation attached to the gateway is set at route-table
			// level, when nothing match and envoy returns 404, that transformation won't ge applied neither!
			name:      "match-none",
			routeName: "match",
			opts: []curl.Option{
				curl.WithBody("hello"),
			},
			resp: &testmatchers.HttpResponse{
				StatusCode: http.StatusNotFound,
				Headers:    map[string]any{
					// The Gateway attached transformation never apply when no route match
					//						"response-gateway": "goodbyte",
				},
				NotHeaders: []string{
					"response-gateway",
					"x-path-response",
					"x-method-response",
					"x-header-response",
					"x-query-response",
					"x-foo-response",
				},
			},
			req: &testmatchers.HttpRequest{
				Headers: map[string]any{
					// The Gateway attached transformation never apply when no route match
					//						"request-gateway": "hello",
				},
				NotHeaders: []string{
					"request-gateway",
					"x-path-request",
					"x-method-request",
					"x-header-request",
					"x-foo-request",
					"x-query-request",
				},
			},
		},
		{
			// test 10
			name:      "custom functions",
			routeName: "custom-functions",
			opts: []curl.Option{
				curl.WithBody(`{"foo":"\"bar\""}`),
				curl.WithHeader("x-nested-call", "my name is andy"),
			},
			resp: &testmatchers.HttpResponse{
				StatusCode: http.StatusOK,
				Headers: map[string]any{
					"x-base64-encode":                   "YmFzZTY0IGVuY29kZSBpbiByZXNwb25zZSBoZWFkZXI=",
					"x-base64-decode":                   "base64 decode in response header",
					"x-base64-decode-invalid-non-empty": "foobar",
					"x-substring":                       "response",
					"x-substring2":                      "resp",
					// when the len is invalid, we default to the end of the string
					"x-substring-invalid2": "response",
					"x-env":                gomega.MatchRegexp(`default/gw-[a-f0-9]*-[a-z0-9]*`),
					"x-replace-random":     gomega.MatchRegexp(`.+ be or not .+ be`),
					// replace_with_random creates a string longer then 4 characters, so if `andy`
					// is not replaced in the x-nested-call request header, this will not match
					"x-replace-nested": gomega.MatchRegexp(`my name is .....+`),
				},
				NotHeaders: []string{
					// When decode fail, we return an empty string which in turn becomes a "remove" header ops
					"x-base64-decode-invalid",
					// when start is invalid, we return an empty string which in turn becomes a "remove" header ops
					"x-substring-invalid",
					"x-env-not-set",
				},
			},
			req: &testmatchers.HttpRequest{
				Headers: map[string]any{
					"x-base64-encode":                   "YmFzZTY0IGVuY29kZSBpbiByZXF1ZXN0IGhlYWRlcg==",
					"x-base64-decode":                   "base64 decode in request header",
					"x-base64-decode-invalid-non-empty": "foobar",
					"x-substring":                       "request",
					"x-substring2":                      "req",
					// when the len is invalid, we default to the end of the string
					"x-substring-invalid2": "request",
					"x-env":                gomega.MatchRegexp(`default/gw-[a-f0-9]*-[a-z0-9]*`),
					"x-replace-random":     gomega.MatchRegexp(`.+ be or not .+ be`),
					"content-length":       "31",
				},
				NotHeaders: []string{
					// When decode fail, we return an empty string which in turn becomes a "remove" header ops
					"x-base64-decode-invalid",
					// when start is invalid, we return an empty string which in turn becomes a "remove" header ops
					"x-substring-invalid",
					"x-env-not-set",
				},
				Body: testmatchers.JSONContains([]byte(`{"Foo":"\"bar\""}`)),
			},
		},
		{
			// test 11
			name:      "pull json info", // shows we parse the body as json
			routeName: "route-for-body-json",
			opts: []curl.Option{
				curl.WithBody(`{"mykey": {"myinnerkey": "myinnervalue"}}`),
				curl.WithHeader("X-Incoming-Stuff", "super"),
			},
			resp: &testmatchers.HttpResponse{
				StatusCode: http.StatusOK,
				Headers: map[string]any{
					"x-how-great":   "level_super",
					"from-incoming": "key_level_myinnervalue",
				},
				// The test dump the headers field from the echo response into the top level of
				// the body, so all the request headers would be at the top level of the json body
				Body: testmatchers.JSONContains([]byte(`{"X-Incoming-Stuff":["super"],"X-Transformed-Incoming":["level_myinnervalue"]}`)),
			},
			// Note: for this test, there is a response body transformation setup which extracts just the headers field
			// When we create the Request Object from the echo response, we accounted for that
			req: &testmatchers.HttpRequest{
				Headers: map[string]any{
					"X-Transformed-Incoming": "level_myinnervalue",
				},
			},
		},
		{
			// test 12
			// The default for Body parsing is AsString which translate to body passthrough (no buffering in envoy)
			// For this test, the response header transformation is set to try to use the `headers` field in the response
			// json body, because the body is never parse, so `headers` is undefine and envoy returns 400 response
			name:      "dont pull info if we dont parse json",
			routeName: "route-for-body",
			opts: []curl.Option{
				curl.WithBody(`{"mykey": {"myinnerkey": "myinnervalue"}}`),
				curl.WithHeader("X-Incoming-Stuff", "super"),
			},
			resp: &testmatchers.HttpResponse{
				StatusCode: http.StatusBadRequest, // bad transformation results in 400
				NotHeaders: []string{
					"x-what-method",
				},
			},
		},
		{
			// test 13
			name:      "dont pull json info if not json", // shows we parse the body as json
			routeName: "route-for-body-json",
			opts: []curl.Option{
				curl.WithBody("hello"),
			},
			resp: &testmatchers.HttpResponse{
				StatusCode: http.StatusBadRequest, // transformation should choke
			},
		},
		{
			// test 14
			name:      "header to body with json parsing",
			routeName: "route-for-header-to-body-json",
			opts: []curl.Option{
				curl.WithBody(`[3,2,1]`),
				curl.WithHeader("X-my-name", "andy"),
			},
			resp: &testmatchers.HttpResponse{
				StatusCode: http.StatusOK,
				Headers:    map[string]any{},
			},
			req: &testmatchers.HttpRequest{
				Headers: map[string]any{
					// The original value is "andy" but we use body modification to change the httpbin
					// response to a random string which is longer than 5 characters. It will fail if
					// the modification did not happen because "andy" is only 4 characters
					"x-my-name": gomega.MatchRegexp(`.....+`),
				},
				Body: fmt.Sprintf("321-%s", httpbin_echo_base_path),
			},
		},
		{
			// test 15
			name:      "modify :method and :status header foo=bar",
			routeName: "pseudo-headers",
			opts: []curl.Option{
				curl.WithHeader("foo", "bar"),
			},
			resp: &testmatchers.HttpResponse{
				StatusCode: http.StatusCreated,
				Headers:    map[string]any{},
			},
			req: &testmatchers.HttpRequest{
				Method: "POST",
			},
		},
		{
			// test 16
			name:      "modify :method and :status header foo=baz",
			routeName: "pseudo-headers",
			opts: []curl.Option{
				curl.WithHeader("foo", "baz"),
			},
			resp: &testmatchers.HttpResponse{
				StatusCode: http.StatusAccepted,
				Headers:    map[string]any{},
			},
			req: &testmatchers.HttpRequest{
				Method: "POST",
			},
		},
		{
			// test 17
			name:      "body transform for local reply",
			routeName: "route-for-body-local-reply",
			opts: []curl.Option{
				curl.WithBody(strings.Repeat("x", 1500)),
			},
			resp: &testmatchers.HttpResponse{
				StatusCode: http.StatusRequestEntityTooLarge,
				Headers: map[string]any{
					"content-length": "17",
				},
				Body: gomega.HaveLen(17), // The body should have the string "Payload Too Large" (17 bytes)
			},
			req: &testmatchers.HttpRequest{},
		},
		{
			// test 18
			name:      "body transform for local reply no body()",
			routeName: "route-for-body-local-reply",
			url:       "/foobar",
			opts: []curl.Option{
				curl.WithBody(strings.Repeat("x", 1500)),
			},
			resp: &testmatchers.HttpResponse{
				StatusCode: http.StatusRequestEntityTooLarge,
				Headers: map[string]any{
					"content-length": "6",
				},
				Body: "foobar",
			},
			req: &testmatchers.HttpRequest{},
		},
	}

	// If no indices are provided, return the full original slice.
	if len(indices) == 0 {
		return commonTestCases
	}

	var selected []transformationTestCase

	for _, index := range indices {
		if index < 0 {
			index = len(commonTestCases) + index
		}

		if index >= 0 && index < len(commonTestCases) {
			selected = append(selected, commonTestCases[index])
		} else {
			fmt.Printf("warning: Index %d out of bounds. Skipping.\n", index)
		}
	}

	return selected
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		base.NewBaseTestingSuite(ctx, testInst, setup, testCases),
		// For local development only!
		// Enter a list of indices to select specific tests, -1 means the last test.
		// Default will return all common test cases.
		// reviewers: please flag the PR if the argument is not empty!
		selectCommonTestCases(),
	}
}

func (s *testingSuite) SetupSuite() {
	s.BaseTestingSuite.SetupSuite()

	s.assertSuiteResourceStatus()
}

func (s *testingSuite) TestGatewayWithTransformedRoute() {
	s.SetRustformationInController(false)
	s.assertTestResourceStatus()
	testutils.Cleanup(s.T(), func() {
		s.SetRustformationInController(true)
	})

	s.TestInstallation.AssertionsT(s.T()).AssertEnvoyAdminApi(
		s.Ctx,
		proxyObjectMeta,
		s.dynamicModuleAssertion(false),
	)

	testCases := []transformationTestCase{}
	testCases = append(testCases, s.commonTestCases...)
	s.runTestCases((testCases))
}

func (s *testingSuite) SetRustformationInController(enabled bool) {
	// make a copy of the original controller deployment
	controllerDeploymentOriginal := &appsv1.Deployment{}
	err := s.TestInstallation.ClusterContext.Client.Get(s.Ctx, client.ObjectKey{
		Namespace: s.TestInstallation.Metadata.InstallNamespace,
		Name:      helpers.DefaultKgatewayDeploymentName,
	}, controllerDeploymentOriginal)
	s.Assert().NoError(err, "has controller deployment")

	rustFormationsEnvVar := corev1.EnvVar{
		Name:  "KGW_USE_RUST_FORMATIONS",
		Value: "false",
	}
	controllerDeployModified := controllerDeploymentOriginal.DeepCopy()
	if !enabled {
		// add the environment variable RUSTFORMATIONS to the modified controller deployment
		controllerDeployModified.Spec.Template.Spec.Containers[0].Env = append(
			controllerDeployModified.Spec.Template.Spec.Containers[0].Env,
			rustFormationsEnvVar,
		)
		controllerDeployModified.ResourceVersion = ""
	} else {
		controllerDeployModified.Spec.Template.Spec.Containers[0].Env = slices.DeleteFunc(controllerDeployModified.Spec.Template.Spec.Containers[0].Env, func(envVar corev1.EnvVar) bool {
			return envVar.Name == "KGW_USE_RUST_FORMATIONS"
		})
	}

	// patch the deployment
	err = s.TestInstallation.ClusterContext.Client.Patch(s.Ctx, controllerDeployModified, client.MergeFrom(controllerDeploymentOriginal))
	s.Assert().NoError(err, "patching controller deployment")

	if !enabled {
		// wait for the changes to be reflected in pod
		s.TestInstallation.AssertionsT(s.T()).EventuallyPodContainerContainsEnvVar(
			s.Ctx,
			s.TestInstallation.Metadata.InstallNamespace,
			metav1.ListOptions{
				LabelSelector: defaults.ControllerLabelSelector,
			},
			helpers.KgatewayContainerName,
			rustFormationsEnvVar,
		)
	} else {
		// make sure the env var is removed
		s.TestInstallation.AssertionsT(s.T()).EventuallyPodContainerDoesNotContainEnvVar(
			s.Ctx,
			s.TestInstallation.Metadata.InstallNamespace,
			metav1.ListOptions{
				LabelSelector: defaults.ControllerLabelSelector,
			},
			helpers.KgatewayContainerName,
			rustFormationsEnvVar.Name,
		)
	}
}

func (s *testingSuite) TestGatewayRustformationsWithTransformedRoute() {
	s.SetRustformationInController(true)
	s.assertTestResourceStatus()

	// wait for pods to be running again, since controller deployment was patched
	s.TestInstallation.AssertionsT(s.T()).EventuallyPodsRunning(s.Ctx, s.TestInstallation.Metadata.InstallNamespace, metav1.ListOptions{
		LabelSelector: defaults.ControllerLabelSelector,
	})
	s.TestInstallation.AssertionsT(s.T()).EventuallyPodsRunning(s.Ctx, proxyObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", defaults.WellKnownAppLabel, proxyObjectMeta.GetName()),
	})

	s.TestInstallation.AssertionsT(s.T()).AssertEnvoyAdminApi(
		s.Ctx,
		proxyObjectMeta,
		s.dynamicModuleAssertion(true),
	)

	testCases := []transformationTestCase{}
	testCases = append(testCases, s.commonTestCases...)
	s.runTestCases((testCases))
}

func (s *testingSuite) runTestCases(testCases []transformationTestCase) {
	for _, tc := range testCases {
		s.T().Run(tc.name, func(t *testing.T) {
			g := gomega.NewWithT(t)
			resp := s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlReturnResponse(
				s.Ctx,
				defaults.CurlPodExecOpt,
				append(tc.opts,
					curl.WithHost(kubeutils.ServiceFQDN(proxyObjectMeta)),
					curl.WithHostHeader(fmt.Sprintf("example-%s.com", tc.routeName)),
					curl.WithPort(8080),
					curl.WithPath(httpbin_echo_base_path+tc.url), // This is the endpoint for httpbin to return the request in json
				),
				tc.resp,
				6, /* timeout */
				2 /* retry interval */)
			if resp.StatusCode == http.StatusOK {
				req, err := helper.CreateRequestFromHttpBinResponse(resp.Body)
				g.Expect(err).NotTo(gomega.HaveOccurred())
				g.Expect(req).To(testmatchers.HaveHttpRequest(tc.req))
			} else {
				resp.Body.Close()
			}
		})
	}
}

func (s *testingSuite) assertRouteAndTrafficPolicyStatus(routesToCheck, trafficPoliciesToCheck []string) {
	currentTimeout, pollingInterval := helpers.GetTimeouts()
	for i, routeName := range routesToCheck {
		trafficPolicyName := trafficPoliciesToCheck[i]

		// get the traffic policy
		s.TestInstallation.AssertionsT(s.T()).Gomega.Eventually(func(g gomega.Gomega) {
			tp := &kgateway.TrafficPolicy{}
			tpObjKey := client.ObjectKey{
				Name:      trafficPolicyName,
				Namespace: "default",
			}
			err := s.TestInstallation.ClusterContext.Client.Get(s.Ctx, tpObjKey, tp)
			g.Expect(err).NotTo(gomega.HaveOccurred(), "failed to get route policy %s", tpObjKey)

			// get the route
			route := &gwv1.HTTPRoute{}
			routeObjKey := client.ObjectKey{
				Name:      routeName,
				Namespace: "default",
			}
			err = s.TestInstallation.ClusterContext.Client.Get(s.Ctx, routeObjKey, route)
			g.Expect(err).NotTo(gomega.HaveOccurred(), "failed to get route %s", routeObjKey)

			// this is the expected traffic policy status condition
			expectedCond := metav1.Condition{
				Type:               string(shared.PolicyConditionAccepted),
				Status:             metav1.ConditionTrue,
				Reason:             string(shared.PolicyReasonValid),
				Message:            reports.PolicyAcceptedMsg,
				ObservedGeneration: route.Generation,
			}

			actualPolicyStatus := tp.Status
			g.Expect(actualPolicyStatus.Ancestors).To(gomega.HaveLen(1), "%s should have one ancestor", trafficPolicyName)
			ancestorStatus := actualPolicyStatus.Ancestors[0]
			cond := meta.FindStatusCondition(ancestorStatus.Conditions, expectedCond.Type)
			g.Expect(cond).NotTo(gomega.BeNil())
			g.Expect(cond.Status).To(gomega.Equal(expectedCond.Status))
			g.Expect(cond.Reason).To(gomega.Equal(expectedCond.Reason))
			g.Expect(cond.Message).To(gomega.Equal(expectedCond.Message))
			g.Expect(cond.ObservedGeneration).To(gomega.Equal(expectedCond.ObservedGeneration))
		}, currentTimeout, pollingInterval).Should(gomega.Succeed())
	}
}

func (s *testingSuite) assertSuiteResourceStatus() {
	routesToCheck := []string{
		"example-route-for-body-as-string",
		// This route is apply right before that test as this is test specific. Cannot check at suite.
		//		"example-route-for-body-json",
		"example-route-for-custom-functions",
		"example-route-for-gateway-attached-transform",
		"example-route-for-header-match",
		"example-route-for-header-to-body-json",
		"example-route-for-headers",
		"example-route-for-method-match",
		"example-route-for-path-match",
		"example-route-for-pseudo-headers",
		"example-route-for-query-match",
	}
	trafficPoliciesToCheck := []string{
		"example-traffic-policy-for-body-as-string",
		// This policy is applied right before that test as this is test specific. Cannot check at suite.
		//		"example-traffic-policy-for-body-json",
		"example-traffic-policy-for-custom-functions",
		"example-traffic-policy-for-gateway-attached-transform",
		"example-traffic-policy-for-header-match",
		"example-traffic-policy-for-header-to-body-json",
		"example-traffic-policy-for-headers",
		"example-traffic-policy-for-method-match",
		"example-traffic-policy-for-path-match",
		"example-traffic-policy-for-pseudo-headers",
		"example-traffic-policy-for-query-match",
	}
	s.assertRouteAndTrafficPolicyStatus(routesToCheck, trafficPoliciesToCheck)
}

func (s *testingSuite) assertTestResourceStatus() {
	routesToCheck := []string{
		"example-route-for-body-json",
	}
	trafficPoliciesToCheck := []string{
		"example-traffic-policy-for-body-json",
	}
	s.assertRouteAndTrafficPolicyStatus(routesToCheck, trafficPoliciesToCheck)
}

func (s *testingSuite) dynamicModuleAssertion(shouldBeLoaded bool) func(ctx context.Context, adminClient *envoyadmincli.Client) {
	return func(ctx context.Context, adminClient *envoyadmincli.Client) {
		s.TestInstallation.AssertionsT(s.T()).Gomega.Eventually(func(g gomega.Gomega) {
			listener, err := adminClient.GetSingleListenerFromDynamicListeners(ctx, "listener~8080")
			g.Expect(err).ToNot(gomega.HaveOccurred(), "failed to get listener")

			// use a weak filter name check for cyclic imports
			// also we dont intend for this to be long term so dont worry about pulling it out to wellknown or something like that for now
			dynamicModuleLoaded := strings.Contains(listener.String(), "dynamic_modules/")
			if shouldBeLoaded {
				g.Expect(dynamicModuleLoaded).To(gomega.BeTrue(), fmt.Sprintf("dynamic module not loaded: %v", listener.String()))
			} else {
				g.Expect(dynamicModuleLoaded).To(gomega.BeFalse(), fmt.Sprintf("dynamic module should not be loaded: %v", listener.String()))
			}
		}).
			WithContext(ctx).
			WithTimeout(time.Second*20).
			WithPolling(time.Second).
			Should(gomega.Succeed(), "failed to get expected load of dynamic modules")
	}
}

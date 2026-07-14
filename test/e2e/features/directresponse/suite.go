//go:build e2e

package directresponse

import (
	"context"
	"net/http"

	"github.com/stretchr/testify/suite"

	. "github.com/onsi/gomega"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/common"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
	"github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
)

type testingSuite struct {
	*base.BaseTestingSuite
}

func NewTestingSuite(
	ctx context.Context,
	testInst *e2e.TestInstallation,
) suite.TestingSuite {
	setup := base.TestCase{
		Manifests: []string{setupManifest},
	}
	// Only include functional test manifests - negative test cases moved to gateway translator suite
	testCases := map[string]*base.TestCase{
		"TestBasicDirectResponse":   {Manifests: []string{basicDirectResponseManifests}},
		"TestDelegation":            {Manifests: []string{basicDelegationManifests}},
		"TestBodyFormatText":        {Manifests: []string{bodyFormatTextManifests}},
		"TestBodyFormatJSON":        {Manifests: []string{bodyFormatJSONManifests}},
		"TestBodyFormatContentType": {Manifests: []string{bodyFormatContentTypeManifests}},
	}
	return &testingSuite{
		BaseTestingSuite: base.NewBaseTestingSuite(ctx, testInst, setup, testCases),
	}
}

func (s *testingSuite) TestBasicDirectResponse() {
	// verify that a direct response route works as expected
	common.BaseGateway.Send(
		s.T(),
		&matchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body:       ContainSubstring("Disallow: /custom"),
		},
		curl.WithHostHeader("www.example.com"),
		curl.WithPath("/robots.txt"),
	)
}

func (s *testingSuite) TestDelegation() {
	// verify the regular child route works as expected.
	common.BaseGateway.Send(
		s.T(),
		&matchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body:       ContainSubstring(`"headers"`),
		},
		curl.WithHostHeader("www.example.com"),
		curl.WithPath("/headers"),
	)

	// verify the parent's DR works as expected.
	common.BaseGateway.Send(
		s.T(),
		&matchers.HttpResponse{
			StatusCode: http.StatusFound,
			Body:       ContainSubstring(`Hello from parent`),
		},
		curl.WithHostHeader("www.example.com"),
		curl.WithPath("/parent"),
	)

	// verify that the child's DR works as expected.
	common.BaseGateway.Send(
		s.T(),
		&matchers.HttpResponse{
			StatusCode: http.StatusFound,
			Body:       ContainSubstring(`Hello from child`),
		},
		curl.WithHostHeader("www.example.com"),
		curl.WithPath("/child"),
	)
}

func (s *testingSuite) TestBodyFormatText() {
	common.BaseGateway.Send(
		s.T(),
		&matchers.HttpResponse{
			StatusCode: http.StatusOK,
			Headers: map[string]any{
				"Content-Type": "text/plain",
			},
			Body: ContainSubstring("Disallow: /robots.txt"),
		},
		curl.WithHostHeader("www.example.com"),
		curl.WithPath("/robots.txt"),
	)
}

func (s *testingSuite) TestBodyFormatJSON() {
	common.BaseGateway.Send(
		s.T(),
		&matchers.HttpResponse{
			StatusCode: http.StatusOK,
			Headers: map[string]any{
				"Content-Type": "application/json",
			},
			Body: ContainSubstring(`{"path":"/data.json","preservedNull":null}`),
		},
		curl.WithHostHeader("www.example.com"),
		curl.WithPath("/data.json"),
	)
}

func (s *testingSuite) TestBodyFormatContentType() {
	common.BaseGateway.Send(
		s.T(),
		&matchers.HttpResponse{
			StatusCode: http.StatusOK,
			Headers: map[string]any{
				"Content-Type": "text/html",
			},
			Body: ContainSubstring("<strong>Requested Path:</strong> /test.html"),
		},
		curl.WithHostHeader("www.example.com"),
		curl.WithPath("/test.html"),
	)
}

// TODO: This test is commented out due to conflicting route actions in the parent HTTPRoute.
// TODO: Re-enable this test once the issue with conflicting filters is resolved or the expected behavior is clarified.
// TODO: When re-enabling, move this test to the gateway translator suite.
// func (s *testingSuite) TestInvalidDelegationConflictingFilters() {
// 	// the parent httproute both 1) specifies a direct response and 2) delegates to another httproute which routes to a service.
// 	// since these route actions are conflicting, we should get a 500 here
// 	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
// 		s.Ctx,
// 		defaults.CurlPodExecOpt,
// 		[]curl.Option{
// 			curl.WithHost(kubeutils.ServiceFQDN(proxyObjectMeta)),
// 			curl.WithHostHeader("www.example.com"),
// 			curl.WithPath("/headers"),
// 		},
// 		&matchers.HttpResponse{
// 			StatusCode: http.StatusInternalServerError,
// 		},
// 		time.Minute,
// 	)

// 	// the parent should show an error in its status
// 	s.TestInstallation.AssertionsT(s.T()).EventuallyHTTPRouteStatusContainsReason(s.Ctx, gwRouteMeta.Name, gwRouteMeta.Namespace,
// 		string(gwv1.RouteReasonIncompatibleFilters), 10*time.Second, 1*time.Second)
// }

// TODO: This test is commented out due to conflicting route actions in the parent HTTPRoute.
// TODO: Re-enable this test once the issue with conflicting filters is resolved or the expected behavior is clarified.
// TODO: When re-enabling, move this test to the gateway translator suite.
// func (s *testingSuite) TestInvalidMultipleRouteActions() {
// 	// the route specifies both a request redirect and a direct response, which is invalid.
// 	// verify the route was replaced with a 500 direct response due to the
// 	// invalid configuration.
// 	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
// 		s.Ctx,
// 		defaults.CurlPodExecOpt,
// 		[]curl.Option{
// 			curl.WithHost(kubeutils.ServiceFQDN(proxyObjectMeta)),
// 			curl.WithHostHeader("www.example.com"),
// 			curl.WithPath("/"),
// 		},
// 		&matchers.HttpResponse{
// 			StatusCode: http.StatusInternalServerError,
// 		},
// 		time.Minute,
// 	)
// 	s.TestInstallation.AssertionsT(s.T()).EventuallyHTTPRouteStatusContainsReason(s.Ctx, httpbinMeta.Name, httpbinMeta.Namespace,
// 		string(gwv1.RouteReasonIncompatibleFilters), 10*time.Second, 1*time.Second)
// }

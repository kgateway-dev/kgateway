//go:build e2e

package extproc

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	testdefaults "github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/gomega/transforms"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

// testingSuite is a suite of tests for external processing functionality
type testingSuite struct {
	*base.BaseTestingSuite
}

var (
	setup = base.TestCase{
		Manifests: []string{
			setupManifest,
			testdefaults.CurlPodManifest,
			testdefaults.ExtProcManifest,
		},
	}

	testCases = map[string]*base.TestCase{
		"TestExtProcWithGatewayTargetRef": {
			Manifests:       []string{gatewayTargetRefManifest},
			MinGwApiVersion: base.GwApiRequireRouteNames,
		},
		"TestExtProcWithHTTPRouteTargetRef": {
			Manifests: []string{httpRouteTargetRefManifest},
		},
		"TestExtProcWithSingleRoute": {
			Manifests: []string{singleRouteManifest},
		},
		"TestExtProcWithBackendFilter": {
			Manifests: []string{backendFilterManifest},
		},
	}
)

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		BaseTestingSuite: base.NewBaseTestingSuite(ctx, testInst, setup, testCases),
	}
}

// TestExtProcWithGatewayTargetRef tests ExtProc with targetRef to Gateway
func (s *testingSuite) TestExtProcWithGatewayTargetRef() {
	// Test that ExtProc is applied to all routes through the Gateway
	// First route - should have ExtProc applied
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(gatewayService.ObjectMeta)),
			curl.VerboseOutput(),
			curl.WithHostHeader("www.example.com"),
			curl.WithPort(8080),
			curl.WithHeader("instructions", getInstructionsJson(instructions{
				AddHeaders: map[string]string{"extproctest": "true"},
			})),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body: gomega.WithTransform(transforms.WithJsonBody(),
				gomega.And(
					gomega.HaveKeyWithValue("headers", gomega.HaveKey("Extproctest")),
				),
			),
		})

	// Second route rule0 - should also have ExtProc applied
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(gatewayService.ObjectMeta)),
			curl.VerboseOutput(),
			curl.WithHostHeader("www.example.com"),
			curl.WithPath("/myapp"),
			curl.WithPort(8080),
			curl.WithHeader("instructions", getInstructionsJson(instructions{
				AddHeaders: map[string]string{"extproctest": "true"},
			})),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body: gomega.WithTransform(transforms.WithJsonBody(),
				gomega.And(
					gomega.HaveKeyWithValue("headers", gomega.HaveKey("Extproctest")),
				),
			),
		})

	// Second route rule1 - should not have ExtProc applied since it has a disable policy applied
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(gatewayService.ObjectMeta)),
			curl.VerboseOutput(),
			curl.WithHostHeader("www.example.com"),
			curl.WithPath("/extproc-disabled"),
			curl.WithPort(8080),
			curl.WithHeader("instructions", getInstructionsJson(instructions{
				AddHeaders: map[string]string{"extproctest": "true"},
			})),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body: gomega.WithTransform(transforms.WithJsonBody(),
				gomega.And(
					gomega.HaveKeyWithValue("headers", gomega.Not(gomega.HaveKey("Extproctest"))),
				),
			),
		})
}

// TestExtProcWithHTTPRouteTargetRef tests ExtProc with targetRef to HTTPRoute
func (s *testingSuite) TestExtProcWithHTTPRouteTargetRef() {
	// Test route with ExtProc - should have header modified
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(gatewayService.ObjectMeta)),
			curl.VerboseOutput(),
			curl.WithHostHeader("www.example.com"),
			curl.WithPath("/myapp"),
			curl.WithPort(8080),
			curl.WithHeader("instructions", getInstructionsJson(instructions{
				AddHeaders: map[string]string{"extproctest": "true"},
			})),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body: gomega.WithTransform(transforms.WithJsonBody(),
				gomega.And(
					gomega.HaveKeyWithValue("headers", gomega.HaveKey("Extproctest")),
				),
			),
		})

	// Test route without ExtProc - should not have header modified
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(gatewayService.ObjectMeta)),
			curl.VerboseOutput(),
			curl.WithHostHeader("www.example.com"),
			curl.WithPort(8080),
			curl.WithHeader("instructions", getInstructionsJson(instructions{
				AddHeaders: map[string]string{"extproctest": "true"},
			})),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body: gomega.WithTransform(transforms.WithJsonBody(),
				gomega.And(
					gomega.HaveKeyWithValue("headers", gomega.Not(gomega.HaveKey("Extproctest"))),
				),
			),
		})
}

// TestExtProcWithSingleRoute tests ExtProc applied to a specific rule within a route
func (s *testingSuite) TestExtProcWithSingleRoute() {
	// TODO: Should header-based routing work?

	// Test route with ExtProc and matching header - should have header modified
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(gatewayService.ObjectMeta)),
			curl.VerboseOutput(),
			curl.WithHostHeader("www.example.com"),
			curl.WithPath("/myapp"),
			// curl.WithHeader("x-test", "true"),
			curl.WithHeader("instructions", getInstructionsJson(instructions{
				AddHeaders: map[string]string{"extproctest": "true"},
			})),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body: gomega.WithTransform(transforms.WithJsonBody(),
				gomega.And(
					gomega.HaveKeyWithValue("headers", gomega.HaveKey("Extproctest")),
				),
			),
		})

	// Test second rule - should not have header modified
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(gatewayService.ObjectMeta)),
			curl.VerboseOutput(),
			curl.WithHostHeader("www.example.com"),
			curl.WithPort(8080),
			curl.WithHeader("instructions", getInstructionsJson(instructions{
				AddHeaders: map[string]string{"extproctest": "true"},
			})),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body: gomega.WithTransform(transforms.WithJsonBody(),
				gomega.And(
					gomega.HaveKeyWithValue("headers", gomega.Not(gomega.HaveKey("Extproctest"))),
				),
			),
		})
}

// TestExtProcWithBackendFilter tests backend-level ExtProc filtering
func (s *testingSuite) TestExtProcWithBackendFilter() {
	// Test path with ExtProc enabled
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(gatewayService.ObjectMeta)),
			curl.VerboseOutput(),
			curl.WithHostHeader("www.example.com"),
			curl.WithPath("/with-extproc"),
			curl.WithHeader("instructions", getInstructionsJson(instructions{
				AddHeaders: map[string]string{"extproctest": "true"},
			})),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body: gomega.WithTransform(transforms.WithJsonBody(),
				gomega.And(
					gomega.HaveKeyWithValue("headers", gomega.HaveKey("Extproctest")),
				),
			),
		})

	// Test path without ExtProc
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(gatewayService.ObjectMeta)),
			curl.VerboseOutput(),
			curl.WithHostHeader("www.example.com"),
			curl.WithPath("/without-extproc"),
			curl.WithHeader("instructions", getInstructionsJson(instructions{
				AddHeaders: map[string]string{"extproctest": "true"},
			})),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body: gomega.WithTransform(transforms.WithJsonBody(),
				gomega.And(
					gomega.HaveKeyWithValue("headers", gomega.Not(gomega.HaveKey("Extproctest"))),
				),
			),
		})
}

// The instructions format that the example extproc service understands.
// See test/e2e/defaults/extproc/README.md for more details.
type instructions struct {
	// Header key/value pairs to add to the request or response.
	AddHeaders map[string]string `json:"addHeaders"`
	// Header keys to remove from the request or response.
	RemoveHeaders []string `json:"removeHeaders"`
}

func getInstructionsJson(instr instructions) string {
	bytes, _ := json.Marshal(instr)
	return string(bytes)
}

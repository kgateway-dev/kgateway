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
		"TestExtProcWithFilterStage": {
			Manifests:       []string{filterStageManifest},
			MinGwApiVersion: base.GwApiRequireRouteNames,
		},
		// Requires KGW_POLICY_MERGE={"trafficPolicy":{"extProc":"DeepMerge"}}
		"TestExtProcWithFilterStageWeightOrdering": {
			Manifests: []string{dualServersManifest, filterStageWeightManifest},
		},
		"TestExtProcWithDeepMerge": {
			Manifests: []string{dualServersManifest, deepMergeManifest},
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

// TestExtProcWithFilterStage tests that ExtProc works correctly when configured
// with a custom filter stage via filterStage on the GatewayExtension.
func (s *testingSuite) TestExtProcWithFilterStage() {
	// Route with ExtProc at early stage (After Fault) - should have header modified
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(gatewayService.ObjectMeta)),
			curl.VerboseOutput(),
			curl.WithHostHeader("www.example.com"),
			curl.WithPath("/early-extproc"),
			curl.WithPort(8080),
			curl.WithHeader("instructions", getInstructionsJson(instructions{
				AddHeaders: map[string]string{"extproc-early": "true"},
			})),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body: gomega.WithTransform(transforms.WithJsonBody(),
				gomega.And(
					gomega.HaveKeyWithValue("headers", gomega.HaveKey("Extproc-Early")),
				),
			),
		})

	// Route with ExtProc at default stage (After AuthZ) - should also have header modified
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(gatewayService.ObjectMeta)),
			curl.VerboseOutput(),
			curl.WithHostHeader("www.example.com"),
			curl.WithPath("/default-extproc"),
			curl.WithPort(8080),
			curl.WithHeader("instructions", getInstructionsJson(instructions{
				AddHeaders: map[string]string{"extproc-default": "true"},
			})),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body: gomega.WithTransform(transforms.WithJsonBody(),
				gomega.And(
					gomega.HaveKeyWithValue("headers", gomega.HaveKey("Extproc-Default")),
				),
			),
		})

	// Route without ExtProc - should NOT have header modified
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(gatewayService.ObjectMeta)),
			curl.VerboseOutput(),
			curl.WithHostHeader("www.example.com"),
			curl.WithPath("/no-extproc"),
			curl.WithPort(8080),
			curl.WithHeader("instructions", getInstructionsJson(instructions{
				AddHeaders: map[string]string{"extproc-none": "true"},
			})),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body: gomega.WithTransform(transforms.WithJsonBody(),
				gomega.And(
					gomega.HaveKeyWithValue("headers", gomega.Not(gomega.HaveKey("Extproc-None"))),
				),
			),
		})
}

// TestExtProcWithFilterStageWeightOrdering tests that multiple ExtProc filters
// at the same stage are ordered by their filterStage weight. Higher weight
// places the filter earlier in the chain.
// Requires kgateway to be deployed with KGW_POLICY_MERGE={"trafficPolicy":{"extProc":"DeepMerge"}}
// so that multiple ExtProc policies attached via extensionRef filters are both applied.
func (s *testingSuite) TestExtProcWithFilterStageWeightOrdering() {
	// Verify both filters execute: high-weight server-a (weight=10) and low-weight
	// server-b (weight=-5) headers should both be present
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(gatewayService.ObjectMeta)),
			curl.VerboseOutput(),
			curl.WithHostHeader("www.example.com"),
			curl.WithPath("/both"),
			curl.WithPort(8080),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body: gomega.WithTransform(transforms.WithJsonBody(),
				gomega.And(
					gomega.HaveKeyWithValue("headers", gomega.HaveKey("X-Extproc-Server-A")),
					gomega.HaveKeyWithValue("headers", gomega.HaveKey("X-Extproc-Server-B")),
				),
			),
		})

	// Verify filter execution order: high-weight server-a (weight=10) runs before
	// low-weight server-b (weight=-5). Both servers receive the instruction to remove
	// x-extproc-server-a. Server A runs first: its --add-header sets x-extproc-server-a
	// (Envoy applies removes before sets within a single response, so the header is
	// present). Server B runs second: removes x-extproc-server-a (added by server A)
	// and adds x-extproc-server-b.
	// Result: x-extproc-server-a absent proves server A (high-weight) ran before
	// server B (low-weight).
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(gatewayService.ObjectMeta)),
			curl.VerboseOutput(),
			curl.WithHostHeader("www.example.com"),
			curl.WithPath("/both"),
			curl.WithPort(8080),
			curl.WithHeader("instructions", getInstructionsJson(instructions{
				RemoveHeaders: []string{"x-extproc-server-a"},
			})),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body: gomega.WithTransform(transforms.WithJsonBody(),
				gomega.And(
					gomega.HaveKeyWithValue("headers", gomega.Not(gomega.HaveKey("X-Extproc-Server-A"))),
					gomega.HaveKeyWithValue("headers", gomega.HaveKey("X-Extproc-Server-B")),
				),
			),
		})
}

// TestExtProcWithDeepMerge tests that two ExtProc filters from different
// hierarchy levels (Gateway + HTTPRoute) can both be active on the same route
// when the Gateway has the inherited-policy-priority annotation set to
// DeepMergePreferParent. Each filter uses a distinct ExtProc server that adds
// its own identifying header, proving both filters ran.
func (s *testingSuite) TestExtProcWithDeepMerge() {
	// /both - both gateway-level and route-level ExtProc should run
	// Expect both x-extproc-server-a (gateway-level) and x-extproc-server-b (route-level) headers
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(deepMergeGatewayService.ObjectMeta)),
			curl.VerboseOutput(),
			curl.WithHostHeader("www.example.com"),
			curl.WithPath("/both"),
			curl.WithPort(8080),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body: gomega.WithTransform(transforms.WithJsonBody(),
				gomega.And(
					gomega.HaveKeyWithValue("headers", gomega.HaveKey("X-Extproc-Server-A")),
					gomega.HaveKeyWithValue("headers", gomega.HaveKey("X-Extproc-Server-B")),
				),
			),
		})

	// Verify filter execution order: Server A (ext-proc-a, gateway-level) runs before
	// Server B (ext-proc-b, route-level) because filter names are sorted alphabetically
	// when at the same stage.
	// Both servers receive the instruction to remove x-extproc-server-a. Server A runs
	// first: its --add-header sets x-extproc-server-a (Envoy applies removes before sets
	// within a single response, so the header is present). Server B runs second: removes
	// x-extproc-server-a (added by Server A) and adds x-extproc-server-b.
	// Result: x-extproc-server-a absent proves Server A ran before Server B.
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(deepMergeGatewayService.ObjectMeta)),
			curl.VerboseOutput(),
			curl.WithHostHeader("www.example.com"),
			curl.WithPath("/both"),
			curl.WithPort(8080),
			curl.WithHeader("instructions", getInstructionsJson(instructions{
				RemoveHeaders: []string{"x-extproc-server-a"},
			})),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body: gomega.WithTransform(transforms.WithJsonBody(),
				gomega.And(
					gomega.HaveKeyWithValue("headers", gomega.Not(gomega.HaveKey("X-Extproc-Server-A"))),
					gomega.HaveKeyWithValue("headers", gomega.HaveKey("X-Extproc-Server-B")),
				),
			),
		})

	// /gateway-only - only gateway-level ExtProc should run
	// Expect x-extproc-server-a but NOT x-extproc-server-b
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(deepMergeGatewayService.ObjectMeta)),
			curl.VerboseOutput(),
			curl.WithHostHeader("www.example.com"),
			curl.WithPath("/gateway-only"),
			curl.WithPort(8080),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body: gomega.WithTransform(transforms.WithJsonBody(),
				gomega.And(
					gomega.HaveKeyWithValue("headers", gomega.HaveKey("X-Extproc-Server-A")),
					gomega.HaveKeyWithValue("headers", gomega.Not(gomega.HaveKey("X-Extproc-Server-B"))),
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

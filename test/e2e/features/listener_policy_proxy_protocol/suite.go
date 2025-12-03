//go:build e2e

package listener_policy_proxy_protocol

import (
	"context"
	"net/http"

	"github.com/stretchr/testify/suite"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	testdefaults "github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

// testingSuite validates ListenerPolicy proxy protocol behavior
// Specifically: when proxyProtocol is enabled, plain HTTP (no PROXY header) should be rejected.
type testingSuite struct {
	suite.Suite
	ctx              context.Context
	testInstallation *e2e.TestInstallation
	manifests        map[string][]string
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		ctx:              ctx,
		testInstallation: testInst,
	}
}

func (s *testingSuite) SetupSuite() {
	// Apply common setup (backend service + curl client)
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, setupManifest)
	s.Require().NoError(err, "can apply "+setupManifest)

	// Ensure backend and curl pod are ready
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, exampleSvc, curlPod)

	// Define manifests per test
	s.manifests = map[string][]string{
		"TestProxyProtocol": {gatewayManifest, httpRouteManifest, listenerPolicyManifest},
	}
}

func (s *testingSuite) TearDownSuite() {
	// Cleanup setup
	_ = s.testInstallation.Actions.Kubectl().DeleteFileSafe(s.ctx, setupManifest)
}

func (s *testingSuite) BeforeTest(suiteName, testName string) {
	manifests, ok := s.manifests[testName]
	if !ok {
		s.FailNow("no manifests found for %s", testName)
	}
	for _, m := range manifests {
		s.Require().NoError(s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, m), "apply "+m)
	}
	// wait for proxy service/pod
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, proxyService, proxyDeployment)
}

func (s *testingSuite) AfterTest(suiteName, testName string) {
	manifests, ok := s.manifests[testName]
	if !ok {
		return
	}
	for _, m := range manifests {
		output, err := s.testInstallation.Actions.Kubectl().DeleteFileWithOutput(s.ctx, m)
		s.testInstallation.Assertions.ExpectObjectDeleted(m, err, output)
	}
}

// Test that enabling PROXY protocol causes plain HTTP (no PROXY header) to be rejected.
func (s *testingSuite) TestProxyProtocol() {
	// Attempt a normal HTTP request; expect curl to error (connection closed/empty reply).
	s.testInstallation.Assertions.AssertEventualCurlError(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithHostHeader("example.com"),
			curl.WithPort(8080),
		},
		0, // accept any curl error code
	)

	// test with PROXY protocol header; expect 200 OK
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithHostHeader("example.com"),
			curl.WithPort(8080),
			curl.WithProxyProto(),
		},
		&matchers.HttpResponse{StatusCode: http.StatusOK})
}

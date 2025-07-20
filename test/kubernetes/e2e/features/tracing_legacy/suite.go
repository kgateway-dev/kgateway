//go:build ignore

package tracing

import (
	"context"
	"net/http"
	"time"

	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	gloo_defaults "github.com/kgateway-dev/kgateway/v2/internal/gloo/pkg/defaults"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	testdefaults "github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/defaults"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

type testingSuite struct {
	suite.Suite

	ctx context.Context

	testInstallation *e2e.TestInstallation
}

func NewTestingSuite(
	ctx context.Context,
	testInst *e2e.TestInstallation,
) suite.TestingSuite {
	return &testingSuite{
		ctx:              ctx,
		testInstallation: testInst,
	}
}

/*
Overview of tracing tests:

1. install echo-server (upstream) and curl in SetupSuite (this can be done
once)

2. install/reinstall otelcol in BeforeTest - this avoids contamination between
tests by ensuring the console output is clean for each test.

3. send requests to the gateway-proxy so envoy sends traces to otelcol

4. parse stdout from otelcol to see if the trace contains the data that we want
*/

func (s *testingSuite) SetupSuite() {
	var err error

	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, testdefaults.CurlPodManifest)
	s.NoError(err, "can apply CurlPodManifest")
	s.testInstallation.Assertions.EventuallyPodsRunning(
		s.ctx,
		testdefaults.CurlPod.GetObjectMeta().GetNamespace(),
		metav1.ListOptions{
			LabelSelector: "app.kubernetes.io/name=curl",
		},
	)

	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, testdefaults.HttpEchoPodManifest)
	s.NoError(err, "can apply HttpEchoPodManifest")
	s.testInstallation.Assertions.EventuallyPodsRunning(
		s.ctx,
		testdefaults.HttpEchoPod.GetObjectMeta().GetNamespace(),
		metav1.ListOptions{
			LabelSelector: "app.kubernetes.io/name=http-echo",
		},
	)

	// Previously, we would create/delete the Service for each test. However, this would occasionally lead to:
	// * Hostname gateway-proxy-tracing.gloo-gateway-edge-test.svc.cluster.local was found in DNS cache
	//*   Trying 10.96.181.139:18080...
	//* Connection timed out after 3001 milliseconds
	//
	// The suspicion is that the rotation of the Service meant that the DNS cache became out of date,
	// and we would curl the old IP.
	// The workaround to that is to create the service just once at the beginning of the suite.
	// This mirrors how Services are typically managed in Gloo Gateway, where they are tied
	// to an installation, and not dynamically updated
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, gatewayProxyServiceManifest,
		"-n", s.testInstallation.Metadata.InstallNamespace)
	s.NoError(err, "can apply service/gateway-proxy-tracing")
}

func (s *testingSuite) TearDownSuite() {
	var err error

	err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, testdefaults.CurlPodManifest)
	s.Assertions.NoError(err, "can delete curl pod")

	err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, testdefaults.HttpEchoPodManifest)
	s.Assertions.NoError(err, "can delete echo server")

	err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, gatewayProxyServiceManifest,
		"-n", s.testInstallation.Metadata.InstallNamespace)
	s.NoError(err, "can delete service/gateway-proxy-tracing")
}

func (s *testingSuite) BeforeTest(string, string) {
	var err error

	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, setupOtelcolManifest)
	s.NoError(err, "can apply opentelemetry collector")
	s.testInstallation.Assertions.EventuallyPodsRunning(
		s.ctx,
		otelcolPod.GetObjectMeta().GetNamespace(),
		otelcolSelector,
	)

	// Technical Debt!!
	// https://github.com/kgateway-dev/kgateway/issues/10293
	// There is a bug in the Control Plane that results in an Error reported on the status
	// when the Upstream of the Tracing Collector is not found. This results in the VirtualService
	// that references that Upstream being rejected. What should occur is a Warning is reported,
	// and the resource is accepted since validation.allowWarnings=true is set.
	// We have plans to fix this in the code itself. But for a short-term solution, to reduce the
	// noise in CI/CD of this test flaking, we perform some simple retry logic here.
	s.EventuallyWithT(func(c *assert.CollectT) {
		err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, tracingConfigManifest)
		assert.NoError(c, err, "can apply gloo tracing resources")
	}, time.Second*5, time.Second*1, "can apply tracing resources")

	// accept the upstream
	// Upstreams no longer report status if they have not been translated at all to avoid conflicting with
	// other syncers that have translated them, so we can only detect that the objects exist here
	s.testInstallation.Assertions.EventuallyResourceExists(
		func() (resources.Resource, error) {
			return s.testInstallation.ResourceClients.UpstreamClient().Read(
				otelcolUpstream.Namespace, otelcolUpstream.Name, clients.ReadOpts{Ctx: s.ctx})
		},
	)

	// accept the virtual service
	s.testInstallation.Assertions.EventuallyResourceStatusMatchesState(
		func() (resources.InputResource, error) {
			return s.testInstallation.ResourceClients.VirtualServiceClient().Read(
				tracingVs.Namespace, tracingVs.Name, clients.ReadOpts{Ctx: s.ctx})
		},
		core.Status_Accepted,
		gloo_defaults.GlooReporter,
	)

	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, gatewayConfigManifest,
		"-n", s.testInstallation.Metadata.InstallNamespace)
	s.NoError(err, "can create gateway and service")
	s.testInstallation.Assertions.EventuallyResourceStatusMatchesState(
		func() (resources.InputResource, error) {
			return s.testInstallation.ResourceClients.GatewayClient().Read(
				s.testInstallation.Metadata.InstallNamespace, "gateway-proxy-tracing", clients.ReadOpts{Ctx: s.ctx})
		},
		core.Status_Accepted,
		gloo_defaults.GlooReporter,
	)
}

func (s *testingSuite) AfterTest(string, string) {
	var err error
	err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, setupOtelcolManifest)
	s.Assertions.NoError(err, "can delete otel collector")

	err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, tracingConfigManifest)
	s.Assertions.NoError(err, "can delete gloo tracing config")

	err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, gatewayConfigManifest,
		"-n", s.testInstallation.Metadata.InstallNamespace)
	s.Assertions.NoError(err, "can delete gateway config")
}

func (s *testingSuite) TestSpanNameTransformationsWithoutRouteDecorator() {
	testHostname := "test-really-cool-hostname.com"
	s.testInstallation.Assertions.AssertEventuallyConsistentCurlResponse(s.ctx, testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(metav1.ObjectMeta{
				Name:      gatewayProxyHost,
				Namespace: s.testInstallation.Metadata.InstallNamespace,
			})),
			curl.WithHostHeader(testHostname),
			curl.WithPort(gatewayProxyPort),
			curl.WithPath(pathWithoutRouteDescriptor),
			// We are asserting that a request is consistent. To prevent flakes with that assertion,
			// we should have some basic retries built into the request
			curl.WithRetryConnectionRefused(true),
			curl.WithRetries(3, 0, 10),
			curl.Silent(),
		},
		&matchers.HttpResponse{
			StatusCode: http.StatusOK,
		},
		5*time.Second, 30*time.Second,
	)

	s.EventuallyWithT(func(c *assert.CollectT) {
		logs, err := s.testInstallation.Actions.Kubectl().GetContainerLogs(s.ctx, otelcolPod.ObjectMeta.GetNamespace(), otelcolPod.ObjectMeta.GetName())
		assert.NoError(c, err, "can get otelcol logs")
		// Looking for a line like this:
		// Name       : <value of host header>
		assert.Regexp(c, "Name *: "+testHostname, logs)
	}, time.Second*30, time.Second*3, "otelcol logs contain span with name == hostname")
}

func (s *testingSuite) TestSpanNameTransformationsWithRouteDecorator() {
	s.testInstallation.Assertions.AssertEventuallyConsistentCurlResponse(s.ctx, testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(metav1.ObjectMeta{
				Name:      gatewayProxyHost,
				Namespace: s.testInstallation.Metadata.InstallNamespace,
			})),
			curl.WithHostHeader("example.com"),
			curl.WithPort(gatewayProxyPort),
			curl.WithPath(pathWithRouteDescriptor),
			// We are asserting that a request is consistent. To prevent flakes with that assertion,
			// we should have some basic retries built into the request
			curl.WithRetryConnectionRefused(true),
			curl.WithRetries(3, 0, 10),
			curl.Silent(),
		},
		&matchers.HttpResponse{
			StatusCode: http.StatusOK,
		},
		5*time.Second, 30*time.Second,
	)

	s.EventuallyWithT(func(c *assert.CollectT) {
		logs, err := s.testInstallation.Actions.Kubectl().GetContainerLogs(s.ctx, otelcolPod.ObjectMeta.GetNamespace(), otelcolPod.ObjectMeta.GetName())
		assert.NoError(c, err, "can get otelcol logs")
		// Looking for a line like this:
		// Name       : <value of routeDescriptorSpanName>
		assert.Regexp(c, "Name *: "+routeDescriptorSpanName, logs)
	}, time.Second*30, time.Second*3, "otelcol logs contain span with name == routeDescriptor")
}

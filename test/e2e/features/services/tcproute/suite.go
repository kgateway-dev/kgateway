//go:build e2e

package tcproute

import (
	"context"

	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils/kubectl"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
	"github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

// testingSuite is the entire suite of tests for testing K8s Service-specific features/fixes
type testingSuite struct {
	*base.BaseTestingSuite
	cancel context.CancelFunc
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	tcpRouteCtx, cancel := context.WithTimeout(ctx, ctxTimeout)
	return &testingSuite{
		BaseTestingSuite: base.NewBaseTestingSuite(tcpRouteCtx, testInst, setup, testCases,
			base.WithMinGwApiVersion(base.GwApiRequireTcpRoutes),
		),
		cancel: cancel,
	}
}

// SetupSuite registers the suite context's cancel before delegating to the base
// suite. Registration must happen here rather than in TearDownSuite: the base
// SetupSuite skips the whole suite when the cluster's Gateway API predates
// GwApiRequireTcpRoutes, and testify only registers its TearDownSuite deferral
// after SetupSuite returns — so a skip would leave the timeout context alive.
func (s *testingSuite) SetupSuite() {
	testutils.Cleanup(s.T(), s.cancel)
	s.BaseTestingSuite.SetupSuite()
}

var (
	// setup holds the Gateway and curl client shared by every subtest. Each subtest
	// attaches to its own listener, so it only creates and deletes routes and backends.
	setup = base.TestCase{
		Manifests: []string{setupManifest},
	}

	testCases = map[string]*base.TestCase{
		"TestConfigureTCPRouteBackingDestinations": {},
	}
)

type tcpRouteTestCase struct {
	name string
	// namespaceManifests create the namespaces the subtest owns. Deleting them
	// removes everything else the subtest applied.
	namespaceManifests  []string
	manifests           []string
	routeNs             string
	expectedResponses   []*matchers.HttpResponse
	expectedErrorCode   int
	ports               []int
	listenerNames       []gwv1.SectionName
	expectedRouteCounts []int32
	tcpRouteNames       []string
	curlOptions         []curl.Option
}

func (s *testingSuite) TestConfigureTCPRouteBackingDestinations() {
	testCases := []tcpRouteTestCase{
		{
			name:               "SingleServiceTCPRoute",
			namespaceManifests: []string{singleSvcNsManifest},
			manifests:          []string{singleSvcBackendManifest, singleSvcTcpRouteManifest},
			routeNs:            singleSvcNsName,
			expectedResponses: []*matchers.HttpResponse{
				expectedSingleSvcResp,
			},
			ports: []int{8087},
			listenerNames: []gwv1.SectionName{
				gwv1.SectionName(singleSvcListenerName8087),
			},
			expectedRouteCounts: []int32{1},
			tcpRouteNames:       []string{singleSvcTCPRouteName},
		},
		{
			name:               "MultiServicesTCPRoute",
			namespaceManifests: []string{multiSvcNsManifest},
			manifests:          []string{multiSvcBackendManifest, multiSvcTcpRouteManifest},
			routeNs:            multiSvcNsName,
			expectedResponses: []*matchers.HttpResponse{
				expectedMultiSvc1Resp,
				expectedMultiSvc2Resp,
			},
			ports: []int{8088, 8089},
			listenerNames: []gwv1.SectionName{
				gwv1.SectionName(multiSvcListenerName8088),
				gwv1.SectionName(multiSvcListenerName8089),
			},
			expectedRouteCounts: []int32{1, 1},
			tcpRouteNames:       []string{multiSvcTCPRouteName1, multiSvcTCPRouteName2},
		},
		{
			name:               tlsListenerSameNsTestName,
			namespaceManifests: []string{tlsListenerNsManifest},
			manifests:          []string{tlsListenerBackendManifest, tlsListenerTcpRouteManifest},
			routeNs:            tlsListenerNsName,
			expectedResponses: []*matchers.HttpResponse{
				expectedTlsListenerResp,
			},
			ports: []int{8443},
			listenerNames: []gwv1.SectionName{
				gwv1.SectionName(tlsListenerListenerName),
			},
			expectedRouteCounts: []int32{1},
			tcpRouteNames:       []string{tlsListenerTCPRouteName},
			curlOptions: []curl.Option{
				curl.WithScheme("https"),
				curl.WithCaFile("/etc/server-certs/tls.crt"),
				curl.WithSni("example.com"),
			},
		},
		{
			name:               crossNsTestName,
			namespaceManifests: []string{crossNsClientNsManifest, crossNsBackendNsManifest},
			manifests:          []string{crossNsBackendSvcManifest, crossNsRefGrantManifest, crossNsTCPRouteManifest},
			routeNs:            crossNsClientName,
			expectedResponses: []*matchers.HttpResponse{
				expectedCrossNsResp,
			},
			ports: []int{8080},
			listenerNames: []gwv1.SectionName{
				gwv1.SectionName(crossNsListenerName),
			},
			expectedRouteCounts: []int32{1},
			tcpRouteNames:       []string{crossNsTCPRouteName},
		},
		{
			name:               crossNsNoRefGrantTestName,
			namespaceManifests: []string{crossNsNoRefGrantClientNsManifest, crossNsNoRefGrantBackendNsManifest},
			// ReferenceGrant not applied
			manifests:         []string{crossNsNoRefGrantBackendSvcManifest, crossNsNoRefGrantTCPRouteManifest},
			routeNs:           crossNsNoRefGrantClientNsName,
			expectedErrorCode: 56,
			ports:             []int{8081},
			listenerNames: []gwv1.SectionName{
				gwv1.SectionName(crossNsNoRefGrantListenerName),
			},
			expectedRouteCounts: []int32{1},
			tcpRouteNames:       []string{crossNsNoRefGrantTCPRouteName},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			// No subtest shares a namespace or listener with another, so the next one can
			// start while these namespaces finish terminating.
			testutils.Cleanup(s.T(), func() {
				for _, manifest := range tc.namespaceManifests {
					err := s.TestInstallation.Actions.Kubectl().DeleteFileSafe(s.Ctx, manifest, "--wait=false")
					s.Require().NoError(err, "Failed to delete manifest "+manifest)
				}
			})

			s.applyManifests(tc.namespaceManifests...)
			s.applyManifests(tc.manifests...)

			// Set the expected status conditions based on the test case
			expected := metav1.ConditionTrue
			if tc.name == crossNsNoRefGrantTestName {
				expected = metav1.ConditionFalse
			}

			// Assert TCPRoute conditions
			for _, tcpRouteName := range tc.tcpRouteNames {
				s.TestInstallation.AssertionsT(s.T()).EventuallyTCPRouteCondition(s.Ctx, tcpRouteName, tc.routeNs, gwv1.RouteConditionAccepted, metav1.ConditionTrue, timeout)
				s.TestInstallation.AssertionsT(s.T()).EventuallyTCPRouteCondition(s.Ctx, tcpRouteName, tc.routeNs, gwv1.RouteConditionResolvedRefs, expected, timeout)
			}

			// Assert gateway programmed condition
			s.TestInstallation.AssertionsT(s.T()).EventuallyGatewayCondition(s.Ctx, gatewayName, gatewayNs, gwv1.GatewayConditionProgrammed, metav1.ConditionTrue, timeout)

			// Assert listener attached routes
			for i, listenerName := range tc.listenerNames {
				expectedRouteCount := tc.expectedRouteCounts[i]
				s.TestInstallation.AssertionsT(s.T()).EventuallyGatewayListenerAttachedRoutes(s.Ctx, gatewayName, gatewayNs, listenerName, expectedRouteCount, timeout)
			}

			// Assert expected responses
			for i, port := range tc.ports {
				curlOpts := []curl.Option{
					curl.WithHost(kubeutils.ServiceFQDN(proxyObjectMeta)),
					curl.WithPort(port),
					curl.VerboseOutput(),
				}

				if len(tc.curlOptions) > 0 {
					curlOpts = append(curlOpts, tc.curlOptions...)
				}

				if tc.expectedErrorCode != 0 {
					s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlError(
						s.Ctx,
						s.execOpts(gatewayNs),
						curlOpts,
						tc.expectedErrorCode,
						timeout)
				} else {
					s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
						s.Ctx,
						s.execOpts(gatewayNs),
						curlOpts,
						tc.expectedResponses[i],
						timeout)
				}
			}
		})
	}
}

func (s *testingSuite) applyManifests(manifests ...string) {
	for _, manifest := range manifests {
		err := s.TestInstallation.Actions.Kubectl().ApplyFile(s.Ctx, manifest)
		s.Require().NoError(err, "Failed to apply manifest "+manifest)
	}
}

func (s *testingSuite) execOpts(ns string) kubectl.PodExecOptions {
	opts := defaults.CurlPodExecOpt
	opts.Namespace = ns
	return opts
}

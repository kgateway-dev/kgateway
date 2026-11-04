//go:build e2e

package tlsroute

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

var (
	// setup holds the Gateway and curl client shared by every subtest. Each subtest
	// attaches to its own listener, so it only creates and deletes routes and backends.
	setup = base.TestCase{
		Manifests: []string{setupManifest},
	}

	// No testCases since we handle everything manually in the test method
	testCases = map[string]*base.TestCase{}
)

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	tlsRouteCtx, cancel := context.WithTimeout(ctx, ctxTimeout)
	return &testingSuite{
		BaseTestingSuite: base.NewBaseTestingSuite(tlsRouteCtx, testInst, setup, testCases,
			base.WithMinGwApiVersion(base.GwApiRequireTlsRoutes),
		),
		cancel: cancel,
	}
}

// SetupSuite registers the suite context's cancel before delegating to the base
// suite. Registration must happen here rather than in TearDownSuite: the base
// SetupSuite skips the whole suite when the cluster's Gateway API predates
// GwApiRequireTlsRoutes, and testify only registers its TearDownSuite deferral
// after SetupSuite returns — so a skip would leave the timeout context alive.
func (s *testingSuite) SetupSuite() {
	testutils.Cleanup(s.T(), s.cancel)
	s.BaseTestingSuite.SetupSuite()
}

type tlsRouteTestCase struct {
	name string
	// namespaceManifests create the namespaces the subtest owns. Deleting them
	// removes everything else the subtest applied.
	namespaceManifests []string
	manifests          []string
	routeNs            string
	// backendNs gets the TLS secret the backends serve with.
	backendNs           string
	expectedResponses   []*matchers.HttpResponse
	expectedErrorCode   int
	ports               []int
	listenerNames       []gwv1.SectionName
	expectedRouteCounts []int32
	tlsRouteNames       []string
}

func (s *testingSuite) TestConfigureTLSRouteBackingDestinations() {
	testCases := []tlsRouteTestCase{
		{
			name:               "SingleServiceTLSRoute",
			namespaceManifests: []string{singleSvcNsManifest},
			manifests:          []string{singleSvcBackendManifest, singleSvcTLSRouteManifest},
			routeNs:            singleSvcNsName,
			backendNs:          singleSvcNsName,
			expectedResponses: []*matchers.HttpResponse{
				expectedSingleSvcResp,
			},
			ports: []int{6443},
			listenerNames: []gwv1.SectionName{
				gwv1.SectionName(singleSvcListenerName6443),
			},
			expectedRouteCounts: []int32{1},
			tlsRouteNames:       []string{singleSvcTLSRouteName},
		},
		{
			name:               "MultiServicesTLSRoute",
			namespaceManifests: []string{multiSvcNsManifest},
			manifests:          []string{multiSvcBackendManifest, multiSvcTlsRouteManifest},
			routeNs:            multiSvcNsName,
			backendNs:          multiSvcNsName,
			expectedResponses: []*matchers.HttpResponse{
				expectedMultiSvc1Resp,
				expectedMultiSvc2Resp,
			},
			ports: []int{6444, 8443},
			listenerNames: []gwv1.SectionName{
				gwv1.SectionName(multiSvcListenerName6444),
				gwv1.SectionName(multiSvcListenerName8443),
			},
			expectedRouteCounts: []int32{1, 1},
			tlsRouteNames:       []string{multiSvcTLSRouteName1, multiSvcTLSRouteName2},
		},
		{
			name:               crossNsTestName,
			namespaceManifests: []string{crossNsClientNsManifest, crossNsBackendNsManifest},
			manifests:          []string{crossNsBackendSvcManifest, crossNsRefGrantManifest, crossNsTLSRouteManifest},
			routeNs:            crossNsClientName,
			backendNs:          crossNsBackendNsName,
			expectedResponses: []*matchers.HttpResponse{
				expectedCrossNsResp,
			},
			ports: []int{8444},
			listenerNames: []gwv1.SectionName{
				gwv1.SectionName(crossNsListenerName),
			},
			expectedRouteCounts: []int32{1},
			tlsRouteNames:       []string{crossNsTLSRouteName},
		},
		{
			name:               crossNsNoRefGrantTestName,
			namespaceManifests: []string{crossNsNoRefGrantClientNsManifest, crossNsNoRefGrantBackendNsManifest},
			// ReferenceGrant not applied
			manifests:         []string{crossNsNoRefGrantBackendSvcManifest, crossNsNoRefGrantTLSRouteManifest},
			routeNs:           crossNsNoRefGrantClientNsName,
			backendNs:         crossNsNoRefGrantBackendNsName,
			expectedErrorCode: 56,
			ports:             []int{8445},
			listenerNames: []gwv1.SectionName{
				gwv1.SectionName(crossNsNoRefGrantListenerName),
			},
			expectedRouteCounts: []int32{1},
			tlsRouteNames:       []string{crossNsNoRefGrantTLSRouteName},
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

			s.applyManifests("", tc.namespaceManifests...)
			s.applyManifests(tc.backendNs, singleSecretManifest)
			s.applyManifests("", tc.manifests...)

			// Set the expected status conditions based on the test case
			expected := metav1.ConditionTrue
			if tc.name == crossNsNoRefGrantTestName {
				expected = metav1.ConditionFalse
			}

			// Assert TLSRoute conditions
			for _, tlsRouteName := range tc.tlsRouteNames {
				s.TestInstallation.AssertionsT(s.T()).EventuallyTLSRouteCondition(s.Ctx, tlsRouteName, tc.routeNs, gwv1.RouteConditionAccepted, metav1.ConditionTrue, timeout)
				s.TestInstallation.AssertionsT(s.T()).EventuallyTLSRouteCondition(s.Ctx, tlsRouteName, tc.routeNs, gwv1.RouteConditionResolvedRefs, expected, timeout)
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
				if tc.expectedErrorCode != 0 {
					s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlError(
						s.Ctx,
						s.execOpts(gatewayNs),
						[]curl.Option{
							curl.WithHost(kubeutils.ServiceFQDN(proxyObjectMeta)),
							curl.WithPort(port),
							curl.VerboseOutput(),
						},
						tc.expectedErrorCode)
				} else {
					s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
						s.Ctx,
						s.execOpts(gatewayNs),
						[]curl.Option{
							curl.WithHost(kubeutils.ServiceFQDN(proxyObjectMeta)),
							curl.WithPort(port),
							curl.WithCaFile("/etc/server-certs/tls.crt"),
							curl.WithScheme("https"),
							curl.WithSni("example.com"),
							curl.VerboseOutput(),
						},
						tc.expectedResponses[i])
				}
			}
		})
	}
}

// applyManifests applies the manifests into ns, or into the namespaces they declare when ns is empty.
func (s *testingSuite) applyManifests(ns string, manifests ...string) {
	var args []string
	if ns != "" {
		args = []string{"-n", ns}
	}
	for _, manifest := range manifests {
		err := s.TestInstallation.Actions.Kubectl().ApplyFile(s.Ctx, manifest, args...)
		s.Require().NoError(err, "Failed to apply manifest "+manifest)
	}
}

func (s *testingSuite) execOpts(ns string) kubectl.PodExecOptions {
	opts := defaults.CurlPodExecOpt
	opts.Namespace = ns
	return opts
}

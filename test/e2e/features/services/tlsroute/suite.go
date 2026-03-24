//go:build e2e

package tlsroute

import (
	"context"
	"fmt"

	"github.com/stretchr/testify/suite"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
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
}

var (
	setup = base.TestCase{}

	// No testCases since we handle everything manually in the test method
	testCases = map[string]*base.TestCase{}
)

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	tlsRouteCtx, _ := context.WithTimeout(ctx, ctxTimeout)
	return &testingSuite{
		BaseTestingSuite: base.NewBaseTestingSuite(tlsRouteCtx, testInst, setup, testCases,
			base.WithMinGwApiVersion(base.GwApiRequireTlsRoutes),
		),
	}
}

type tlsRouteTestCase struct {
	name                string
	nsManifest          string
	gtwName             string
	gtwNs               string
	gtwManifest         string
	svcManifest         string
	tlsRouteManifest    string
	tlsSecretManifest   string
	proxyService        *corev1.Service
	proxyDeployment     *appsv1.Deployment
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
			name:              "SingleServiceTLSRoute",
			nsManifest:        singleSvcNsManifest,
			gtwName:           singleSvcGatewayName,
			gtwNs:             singleSvcNsName,
			gtwManifest:       singleSvcGatewayAndClientManifest,
			svcManifest:       singleSvcBackendManifest,
			tlsRouteManifest:  singleSvcTLSRouteManifest,
			tlsSecretManifest: singleSecretManifest,
			proxyService:      singleSvcProxyService,
			proxyDeployment:   singleSvcProxyDeployment,
			expectedResponses: []*matchers.HttpResponse{
				expectedSingleSvcResp,
			},
			ports: []int{6443},
			listenerNames: []gwv1.SectionName{
				gwv1.SectionName(singleSvcListenerName443),
			},
			expectedRouteCounts: []int32{1},
			tlsRouteNames:       []string{singleSvcTLSRouteName},
		},
		{
			name:              "MultiServicesTLSRoute",
			nsManifest:        multiSvcNsManifest,
			gtwName:           multiSvcGatewayName,
			gtwNs:             multiSvcNsName,
			gtwManifest:       multiSvcGatewayAndClientManifest,
			svcManifest:       multiSvcBackendManifest,
			tlsRouteManifest:  multiSvcTlsRouteManifest,
			tlsSecretManifest: singleSecretManifest,
			proxyService:      multiProxyService,
			proxyDeployment:   multiProxyDeployment,
			expectedResponses: []*matchers.HttpResponse{
				expectedMultiSvc1Resp,
				expectedMultiSvc2Resp,
			},
			ports: []int{6443, 8443},
			listenerNames: []gwv1.SectionName{
				gwv1.SectionName(multiSvcListenerName6443),
				gwv1.SectionName(multiSvcListenerName8443),
			},
			expectedRouteCounts: []int32{1, 1},
			tlsRouteNames:       []string{multiSvcTLSRouteName1, multiSvcTLSRouteName2},
		},
		{
			name:              crossNsTestName,
			nsManifest:        crossNsClientNsManifest,
			gtwName:           crossNsGatewayName,
			gtwNs:             crossNsClientName,
			gtwManifest:       crossNsGatewayManifest,
			svcManifest:       crossNsBackendSvcManifest,
			tlsRouteManifest:  crossNsTLSRouteManifest,
			tlsSecretManifest: singleSecretManifest,
			proxyService:      crossNsProxyService,
			proxyDeployment:   crossNsProxyDeployment,
			expectedResponses: []*matchers.HttpResponse{
				expectedCrossNsResp,
			},
			ports: []int{8443},
			listenerNames: []gwv1.SectionName{
				gwv1.SectionName(crossNsListenerName),
			},
			expectedRouteCounts: []int32{1},
			tlsRouteNames:       []string{crossNsTLSRouteName},
		},
		{
			name:              crossNsNoRefGrantTestName,
			nsManifest:        crossNsNoRefGrantClientNsManifest,
			gtwName:           crossNsNoRefGrantGatewayName,
			gtwNs:             crossNsNoRefGrantClientNsName,
			gtwManifest:       crossNsNoRefGrantGatewayManifest,
			svcManifest:       crossNsNoRefGrantBackendSvcManifest,
			tlsRouteManifest:  crossNsNoRefGrantTLSRouteManifest,
			tlsSecretManifest: singleSecretManifest,
			proxyService:      crossNsNoRefGrantProxyService,
			proxyDeployment:   crossNsNoRefGrantProxyDeployment,
			expectedErrorCode: 56,
			ports:             []int{8443},
			listenerNames: []gwv1.SectionName{
				gwv1.SectionName(crossNsNoRefGrantListenerName),
			},
			expectedRouteCounts: []int32{1},
			tlsRouteNames:       []string{crossNsNoRefGrantTLSRouteName},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			// Cleanup function
			testutils.Cleanup(s.T(), func() {
				s.deleteManifests(tc.nsManifest)

				// Delete additional namespaces if any
				if tc.name == "CrossNamespaceTLSRouteWithReferenceGrant" {
					s.deleteManifests(crossNsBackendNsManifest)
				}

				if tc.name == crossNsNoRefGrantTestName {
					s.deleteManifests(crossNsNoRefGrantBackendNsManifest)
				}

				s.TestInstallation.AssertionsT(s.T()).EventuallyObjectsNotExist(s.Ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: tc.gtwNs}})
			})

			// Setup environment for ReferenceGrant test cases
			if tc.name == crossNsTestName {
				s.applyManifests(crossNsBackendNsName, crossNsBackendNsManifest)
				s.applyManifests(crossNsBackendNsName, crossNsBackendSvcManifest)
				s.applyManifests(crossNsBackendNsName, crossNsRefGrantManifest)
				s.applyManifests(crossNsBackendNsName, singleSecretManifest)
			}

			if tc.name == crossNsNoRefGrantTestName {
				s.applyManifests(crossNsNoRefGrantBackendNsName, crossNsNoRefGrantBackendNsManifest)
				s.applyManifests(crossNsNoRefGrantBackendNsName, crossNsNoRefGrantBackendSvcManifest)
				s.applyManifests(crossNsNoRefGrantBackendNsName, singleSecretManifest)
				// ReferenceGrant not applied
			}

			// Setup environment
			s.setupTestEnvironment(
				tc.nsManifest,
				tc.gtwName,
				tc.gtwNs,
				tc.gtwManifest,
				tc.svcManifest,
				tc.proxyService,
				tc.proxyDeployment,
			)

			fmt.Println("Applying TLS Secret manifest")
			fmt.Println(tc.tlsSecretManifest)
			s.applyManifests(tc.gtwNs, tc.tlsSecretManifest)

			// Apply TLSRoute manifest
			s.applyManifests(tc.gtwNs, tc.tlsRouteManifest)

			// Set the expected status conditions based on the test case
			expected := metav1.ConditionTrue
			if tc.name == crossNsNoRefGrantTestName {
				expected = metav1.ConditionFalse
			}

			// Assert TLSRoute conditions
			for _, tlsRouteName := range tc.tlsRouteNames {
				s.TestInstallation.AssertionsT(s.T()).EventuallyTLSRouteCondition(s.Ctx, tlsRouteName, tc.gtwNs, gwv1.RouteConditionAccepted, metav1.ConditionTrue, timeout)
				s.TestInstallation.AssertionsT(s.T()).EventuallyTLSRouteCondition(s.Ctx, tlsRouteName, tc.gtwNs, gwv1.RouteConditionResolvedRefs, expected, timeout)
			}

			// Assert gateway programmed condition
			s.TestInstallation.AssertionsT(s.T()).EventuallyGatewayCondition(s.Ctx, tc.gtwName, tc.gtwNs, gwv1.GatewayConditionProgrammed, metav1.ConditionTrue, timeout)

			// Assert listener attached routes
			for i, listenerName := range tc.listenerNames {
				expectedRouteCount := tc.expectedRouteCounts[i]
				s.TestInstallation.AssertionsT(s.T()).EventuallyGatewayListenerAttachedRoutes(s.Ctx, tc.gtwName, tc.gtwNs, listenerName, expectedRouteCount, timeout)
			}

			// Assert curl pod is running
			s.TestInstallation.AssertionsT(s.T()).EventuallyPodsRunning(s.Ctx, tc.gtwNs, metav1.ListOptions{
				LabelSelector: "app=curl",
			})

			// Assert expected responses
			for i, port := range tc.ports {
				if tc.expectedErrorCode != 0 {
					s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlError(
						s.Ctx,
						s.execOpts(tc.gtwNs),
						[]curl.Option{
							curl.WithHost(kubeutils.ServiceFQDN(tc.proxyService.ObjectMeta)),
							curl.WithPort(port),
							curl.VerboseOutput(),
						},
						tc.expectedErrorCode)
				} else {
					s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
						s.Ctx,
						s.execOpts(tc.gtwNs),
						[]curl.Option{
							curl.WithHost(kubeutils.ServiceFQDN(tc.proxyService.ObjectMeta)),
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

func (s *testingSuite) setupTestEnvironment(nsManifest, gtwName, gtwNs, gtwManifest, svcManifest string, proxySvc *corev1.Service, proxyDeploy *appsv1.Deployment) {
	s.applyManifests(gtwNs, nsManifest)

	s.applyManifests(gtwNs, gtwManifest)
	s.TestInstallation.AssertionsT(s.T()).EventuallyGatewayCondition(s.Ctx, gtwName, gtwNs, gwv1.GatewayConditionAccepted, metav1.ConditionTrue, timeout)

	s.applyManifests(gtwNs, svcManifest)
	s.TestInstallation.AssertionsT(s.T()).EventuallyObjectsExist(s.Ctx, proxySvc, proxyDeploy)

	s.TestInstallation.AssertionsT(s.T()).EventuallyPodsRunning(s.Ctx, proxyDeploy.GetNamespace(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", defaults.WellKnownAppLabel, proxyDeploy.GetName()),
	})
}

func (s *testingSuite) applyManifests(ns string, manifests ...string) {
	for _, manifest := range manifests {
		err := s.TestInstallation.Actions.Kubectl().ApplyFile(s.Ctx, manifest, "-n", ns)
		s.Require().NoError(err, fmt.Sprintf("Failed to apply manifest %s", manifest))
	}
}

func (s *testingSuite) deleteManifests(manifests ...string) {
	for _, manifest := range manifests {
		err := s.TestInstallation.Actions.Kubectl().DeleteFileSafe(s.Ctx, manifest)
		s.Require().NoError(err, fmt.Sprintf("Failed to delete manifest %s", manifest))
	}
}

func (s *testingSuite) execOpts(ns string) kubectl.PodExecOptions {
	opts := defaults.CurlPodExecOpt
	opts.Namespace = ns
	return opts
}

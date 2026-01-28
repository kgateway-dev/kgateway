//go:build e2e

package tcproute

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

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	tcpRouteCtx, _ := context.WithTimeout(ctx, ctxTimeout)
	return &testingSuite{
		BaseTestingSuite: base.NewBaseTestingSuite(tcpRouteCtx, testInst, setup, testCases,
			base.WithMinGwApiVersion(base.GwApiRequireTcpRoutes),
		),
	}
}

var (
	setup = base.TestCase{}

	testCases = map[string]*base.TestCase{
		"TestConfigureTCPRouteBackingDestinations": {},
	}
)

type tcpRouteTestCase struct {
	name                string
	nsManifest          string
	gtwName             string
	gtwNs               string
	gtwManifest         string
	svcManifest         string
	tcpRouteManifest    string
	preGatewayManifests []string
	proxyService        *corev1.Service
	proxyDeployment     *appsv1.Deployment
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
			name:             "SingleServiceTCPRoute",
			nsManifest:       singleSvcNsManifest,
			gtwName:          singleSvcGatewayName,
			gtwNs:            singleSvcNsName,
			gtwManifest:      singleSvcGatewayAndClientManifest,
			svcManifest:      singleSvcBackendManifest,
			tcpRouteManifest: singleSvcTcpRouteManifest,
			proxyService:     singleSvcProxyService,
			proxyDeployment:  singleSvcProxyDeployment,
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
			name:             "MultiServicesTCPRoute",
			nsManifest:       multiSvcNsManifest,
			gtwName:          multiSvcGatewayName,
			gtwNs:            multiSvcNsName,
			gtwManifest:      multiSvcGatewayAndClientManifest,
			svcManifest:      multiSvcBackendManifest,
			tcpRouteManifest: multiSvcTcpRouteManifest,
			proxyService:     multiProxyService,
			proxyDeployment:  multiProxyDeployment,
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
			name:             tlsListenerSameNsTestName,
			nsManifest:       tlsListenerNsManifest,
			gtwName:          tlsListenerGatewayName,
			gtwNs:            tlsListenerNsName,
			gtwManifest:      tlsListenerGatewayAndClientManifest,
			svcManifest:      tlsListenerBackendManifest,
			tcpRouteManifest: tlsListenerTcpRouteManifest,
			preGatewayManifests: []string{
				tlsListenerTlsSecretManifest,
			},
			proxyService:    tlsListenerProxyService,
			proxyDeployment: tlsListenerProxyDeployment,
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
			name:             crossNsTestName,
			nsManifest:       crossNsClientNsManifest,
			gtwName:          crossNsGatewayName,
			gtwNs:            crossNsClientName,
			gtwManifest:      crossNsGatewayManifest,
			svcManifest:      crossNsBackendSvcManifest,
			tcpRouteManifest: crossNsTCPRouteManifest,
			proxyService:     crossNsProxyService,
			proxyDeployment:  crossNsProxyDeployment,
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
			name:              crossNsNoRefGrantTestName,
			nsManifest:        crossNsNoRefGrantClientNsManifest,
			gtwName:           crossNsNoRefGrantGatewayName,
			gtwNs:             crossNsNoRefGrantClientNsName,
			gtwManifest:       crossNsNoRefGrantGatewayManifest,
			svcManifest:       crossNsNoRefGrantBackendSvcManifest,
			tcpRouteManifest:  crossNsNoRefGrantTCPRouteManifest,
			proxyService:      crossNsNoRefGrantProxyService,
			proxyDeployment:   crossNsNoRefGrantProxyDeployment,
			expectedErrorCode: 56,
			ports:             []int{8080},
			listenerNames: []gwv1.SectionName{
				gwv1.SectionName(crossNsNoRefGrantListenerName),
			},
			expectedRouteCounts: []int32{1},
			tcpRouteNames:       []string{crossNsNoRefGrantTCPRouteName},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			// Cleanup function
			testutils.Cleanup(s.T(), func() {
				s.deleteManifests(tc.nsManifest)

				// Delete additional namespaces if any
				if tc.name == "CrossNamespaceTCPRouteWithReferenceGrant" {
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
			}

			if tc.name == crossNsNoRefGrantTestName {
				s.applyManifests(crossNsNoRefGrantBackendNsName, crossNsNoRefGrantBackendNsManifest)
				s.applyManifests(crossNsNoRefGrantBackendNsName, crossNsNoRefGrantBackendSvcManifest)
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
				tc.preGatewayManifests...,
			)

			// Apply TCPRoute manifest
			s.applyManifests(tc.gtwNs, tc.tcpRouteManifest)

			// Set the expected status conditions based on the test case
			expected := metav1.ConditionTrue
			if tc.name == crossNsNoRefGrantTestName {
				expected = metav1.ConditionFalse
			}

			// Assert TCPRoute conditions
			for _, tcpRouteName := range tc.tcpRouteNames {
				s.TestInstallation.AssertionsT(s.T()).EventuallyTCPRouteCondition(s.Ctx, tcpRouteName, tc.gtwNs, gwv1.RouteConditionAccepted, metav1.ConditionTrue, timeout)
				s.TestInstallation.AssertionsT(s.T()).EventuallyTCPRouteCondition(s.Ctx, tcpRouteName, tc.gtwNs, gwv1.RouteConditionResolvedRefs, expected, timeout)
			}

			// Assert gateway programmed condition
			s.TestInstallation.AssertionsT(s.T()).EventuallyGatewayCondition(s.Ctx, tc.gtwName, tc.gtwNs, gwv1.GatewayConditionProgrammed, metav1.ConditionTrue, timeout)

			// Assert listener attached routes
			for i, listenerName := range tc.listenerNames {
				expectedRouteCount := tc.expectedRouteCounts[i]
				s.TestInstallation.AssertionsT(s.T()).EventuallyGatewayListenerAttachedRoutes(s.Ctx, tc.gtwName, tc.gtwNs, listenerName, expectedRouteCount, timeout)
			}

			// Assert expected responses
			for i, port := range tc.ports {
				curlOpts := []curl.Option{
					curl.WithHost(kubeutils.ServiceFQDN(tc.proxyService.ObjectMeta)),
					curl.WithPort(port),
					curl.VerboseOutput(),
				}

				if len(tc.curlOptions) > 0 {
					curlOpts = append(curlOpts, tc.curlOptions...)
				}

				if tc.expectedErrorCode != 0 {
					s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlError(
						s.Ctx,
						s.execOpts(tc.gtwNs),
						curlOpts,
						tc.expectedErrorCode,
						timeout)
				} else {
					s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
						s.Ctx,
						s.execOpts(tc.gtwNs),
						curlOpts,
						tc.expectedResponses[i],
						timeout)
				}
			}
		})
	}
}

func (s *testingSuite) setupTestEnvironment(nsManifest, gtwName, gtwNs, gtwManifest, svcManifest string, proxySvc *corev1.Service, proxyDeploy *appsv1.Deployment, preGatewayManifests ...string) {
	s.applyManifests(gtwNs, nsManifest)

	if len(preGatewayManifests) > 0 {
		s.applyManifests(gtwNs, preGatewayManifests...)
	}

	s.applyManifests(gtwNs, gtwManifest)
	s.TestInstallation.AssertionsT(s.T()).EventuallyGatewayCondition(s.Ctx, gtwName, gtwNs, gwv1.GatewayConditionAccepted, metav1.ConditionTrue, timeout)

	s.applyManifests(gtwNs, svcManifest)
	s.TestInstallation.AssertionsT(s.T()).EventuallyObjectsExist(s.Ctx, proxySvc, proxyDeploy)
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

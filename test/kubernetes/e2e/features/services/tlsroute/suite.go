package tlsroute

import (
	"context"
	"fmt"
	"os"

	"github.com/stretchr/testify/suite"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils/kubectl"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/defaults"
)

// testingSuite is the entire suite of tests for testing K8s Service-specific features/fixes
type testingSuite struct {
	suite.Suite

	ctx context.Context

	// testInstallation contains all the metadata/utilities necessary to execute a series of tests
	// against an installation of kgateway
	testInstallation *e2e.TestInstallation
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		ctx:              ctx,
		testInstallation: testInst,
	}
}

func (s *testingSuite) SetupSuite() {
	var cancel context.CancelFunc
	s.ctx, cancel = context.WithTimeout(context.Background(), ctxTimeout)
	s.T().Cleanup(cancel)

	manifests := []string{
		singleSvcNsManifest,
		singleSvcGatewayAndClientManifest,
		singleSvcBackendManifest,
		singleSvcTLSRouteManifest,
		multiSvcNsManifest,
		multiSvcGatewayAndClientManifest,
		multiSvcBackendManifest,
		multiSvcTlsRouteManifest,
	}
	for _, file := range manifests {
		s.Require().NoError(validateManifestFile(file), "Invalid manifest file: %s", file)
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
	listenerNames       []v1.SectionName
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
			listenerNames: []v1.SectionName{
				v1.SectionName(singleSvcListenerName443),
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
			listenerNames: []v1.SectionName{
				v1.SectionName(multiSvcListenerName6443),
				v1.SectionName(multiSvcListenerName8443),
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
			listenerNames: []v1.SectionName{
				v1.SectionName(crossNsListenerName),
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
			listenerNames: []v1.SectionName{
				v1.SectionName(crossNsNoRefGrantListenerName),
			},
			expectedRouteCounts: []int32{1},
			tlsRouteNames:       []string{crossNsNoRefGrantTLSRouteName},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			// Cleanup function
			s.T().Cleanup(func() {
				s.deleteManifests(tc.nsManifest)

				// Delete additional namespaces if any
				if tc.name == "CrossNamespaceTLSRouteWithReferenceGrant" {
					s.deleteManifests(crossNsBackendNsManifest)
				}

				if tc.name == crossNsNoRefGrantTestName {
					s.deleteManifests(crossNsNoRefGrantBackendNsManifest)
				}

				s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: tc.gtwNs}})
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
				s.testInstallation.Assertions.EventuallyTLSRouteCondition(s.ctx, tlsRouteName, tc.gtwNs, v1.RouteConditionAccepted, metav1.ConditionTrue, timeout)
				s.testInstallation.Assertions.EventuallyTLSRouteCondition(s.ctx, tlsRouteName, tc.gtwNs, v1.RouteConditionResolvedRefs, expected, timeout)
			}

			// Assert gateway programmed condition
			s.testInstallation.Assertions.EventuallyGatewayCondition(s.ctx, tc.gtwName, tc.gtwNs, v1.GatewayConditionProgrammed, metav1.ConditionTrue, timeout)

			// Assert listener attached routes
			for i, listenerName := range tc.listenerNames {
				expectedRouteCount := tc.expectedRouteCounts[i]
				s.testInstallation.Assertions.EventuallyGatewayListenerAttachedRoutes(s.ctx, tc.gtwName, tc.gtwNs, listenerName, expectedRouteCount, timeout)
			}

			// Assert curl pod is running
			s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, tc.gtwNs, metav1.ListOptions{
				LabelSelector: "app=curl",
			})

			// Assert expected responses
			for i, port := range tc.ports {
				if tc.expectedErrorCode != 0 {
					s.testInstallation.Assertions.AssertEventualCurlError(
						s.ctx,
						s.execOpts(tc.gtwNs),
						[]curl.Option{
							curl.WithHost(kubeutils.ServiceFQDN(tc.proxyService.ObjectMeta)),
							curl.WithPort(port),
							curl.VerboseOutput(),
						},
						tc.expectedErrorCode)
				} else {
					s.testInstallation.Assertions.AssertEventualCurlResponse(
						s.ctx,
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

func validateManifestFile(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("Manifest file not found: %s", path)
	}
	return nil
}

func (s *testingSuite) setupTestEnvironment(nsManifest, gtwName, gtwNs, gtwManifest, svcManifest string, proxySvc *corev1.Service, proxyDeploy *appsv1.Deployment) {
	s.applyManifests(gtwNs, nsManifest)

	s.applyManifests(gtwNs, gtwManifest)
	s.testInstallation.Assertions.EventuallyGatewayCondition(s.ctx, gtwName, gtwNs, v1.GatewayConditionAccepted, metav1.ConditionTrue, timeout)

	s.applyManifests(gtwNs, svcManifest)
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, proxySvc, proxyDeploy)

	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, proxyDeploy.GetNamespace(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app.kubernetes.io/name=%s", proxyDeploy.GetName()),
	})
}

func (s *testingSuite) applyManifests(ns string, manifests ...string) {
	for _, manifest := range manifests {
		err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, manifest, "-n", ns)
		s.Require().NoError(err, fmt.Sprintf("Failed to apply manifest %s", manifest))
	}
}

func (s *testingSuite) deleteManifests(manifests ...string) {
	for _, manifest := range manifests {
		err := s.testInstallation.Actions.Kubectl().DeleteFileSafe(s.ctx, manifest)
		s.Require().NoError(err, fmt.Sprintf("Failed to delete manifest %s", manifest))
	}
}

func (s *testingSuite) execOpts(ns string) kubectl.PodExecOptions {
	opts := defaults.CurlPodExecOpt
	opts.Namespace = ns
	return opts
}

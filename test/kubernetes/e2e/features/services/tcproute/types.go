package tcproute

import (
	"fmt"
	"net/http"
	"path/filepath"
	"time"

	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"

	"github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// Constants used by TestConfigureTCPRouteBackingDestinationsWithSingleService
	singleSvcNsName           = "single-tcp-route"
	singleSvcGatewayName      = "single-tcp-gateway"
	singleSvcListenerName8087 = "listener-8087"
	singleSvcName             = "single-svc"
	singleSvcTCPRouteName     = "single-tcp-route"

	// Constants used by TestConfigureTCPRouteBackingDestinationsWithMultiServices
	multiSvcNsName           = "multi-tcp-route"
	multiSvcGatewayName      = "multi-tcp-gateway"
	multiSvcListenerName8089 = "listener-8089"
	multiSvcListenerName8088 = "listener-8088"
	multiSvc1Name            = "multi-svc-1"
	multiSvc2Name            = "multi-svc-2"
	multiSvcTCPRouteName1    = "tcp-route-1"
	multiSvcTCPRouteName2    = "tcp-route-2"

	// Constants for CrossNamespaceTCPRouteWithReferenceGrant
	crossNsTestName           = "CrossNamespaceTCPRouteWithReferenceGrant"
	crossNsClientName         = "cross-namespace-allowed-client-ns"
	crossNsBackendNsName      = "cross-namespace-allowed-backend-ns"
	crossNsGatewayName        = "gateway"
	crossNsListenerName       = "listener-8080"
	crossNsBackendSvcName     = "backend-svc"
	crossNsTCPRouteName       = "tcp-route"
	crossNsReferenceGrantName = "reference-grant"

	// Constants for CrossNamespaceTCPRouteWithoutReferenceGrant
	crossNsNoRefGrantTestName       = "CrossNamespaceTCPRouteWithoutReferenceGrant"
	crossNsNoRefGrantClientNsName   = "client-ns-no-refgrant"
	crossNsNoRefGrantBackendNsName  = "backend-ns-no-refgrant"
	crossNsNoRefGrantGatewayName    = "gateway"
	crossNsNoRefGrantListenerName   = "listener-8080"
	crossNsNoRefGrantBackendSvcName = "backend-svc"
	crossNsNoRefGrantTCPRouteName   = "tcp-route"
)

var (
	// Variables used by TestConfigureTCPRouteBackingDestinationsWithSingleService
	multiSvcNsManifest               = filepath.Join(fsutils.MustGetThisDir(), "testdata", "multi-ns.yaml")
	multiSvcGatewayAndClientManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "multi-listener-gateway-and-client.yaml")
	multiSvcBackendManifest          = filepath.Join(fsutils.MustGetThisDir(), "testdata", "multi-backend-service.yaml")
	multiSvcTcpRouteManifest         = filepath.Join(fsutils.MustGetThisDir(), "testdata", "multi-tcproute.yaml")

	// Variables used by TestConfigureTCPRouteBackingDestinationsWithMultiServices
	singleSvcNsManifest               = filepath.Join(fsutils.MustGetThisDir(), "testdata", "single-ns.yaml")
	singleSvcGatewayAndClientManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "single-listener-gateway-and-client.yaml")
	singleSvcBackendManifest          = filepath.Join(fsutils.MustGetThisDir(), "testdata", "single-backend-service.yaml")
	singleSvcTcpRouteManifest         = filepath.Join(fsutils.MustGetThisDir(), "testdata", "single-tcproute.yaml")

	// Manifests for CrossNamespaceTCPRouteWithReferenceGrant
	crossNsClientNsManifest   = filepath.Join(fsutils.MustGetThisDir(), "testdata", "cross-ns-client-ns.yaml")
	crossNsBackendNsManifest  = filepath.Join(fsutils.MustGetThisDir(), "testdata", "cross-ns-backend-ns.yaml")
	crossNsGatewayManifest    = filepath.Join(fsutils.MustGetThisDir(), "testdata", "cross-ns-gateway-and-client.yaml")
	crossNsBackendSvcManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "cross-ns-backend-service.yaml")
	crossNsTCPRouteManifest   = filepath.Join(fsutils.MustGetThisDir(), "testdata", "cross-ns-tcproute.yaml")
	crossNsRefGrantManifest   = filepath.Join(fsutils.MustGetThisDir(), "testdata", "cross-ns-referencegrant.yaml")

	// Manifests for CrossNamespaceTCPRouteWithoutReferenceGrant
	crossNsNoRefGrantClientNsManifest   = filepath.Join(fsutils.MustGetThisDir(), "testdata", "cross-ns-no-refgrant-client-ns.yaml")
	crossNsNoRefGrantBackendNsManifest  = filepath.Join(fsutils.MustGetThisDir(), "testdata", "cross-ns-no-refgrant-backend-ns.yaml")
	crossNsNoRefGrantGatewayManifest    = filepath.Join(fsutils.MustGetThisDir(), "testdata", "cross-ns-no-refgrant-gateway-and-client.yaml")
	crossNsNoRefGrantBackendSvcManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "cross-ns-no-refgrant-backend-service.yaml")
	crossNsNoRefGrantTCPRouteManifest   = filepath.Join(fsutils.MustGetThisDir(), "testdata", "cross-ns-no-refgrant-tcproute.yaml")

	// Assertion test timers
	ctxTimeout = 10 * time.Minute
	timeout    = 60 * time.Second

	singleGlooProxy = metav1.ObjectMeta{
		Name:      "single-tcp-gateway",
		Namespace: singleSvcNsName,
	}
	singleSvcProxyDeployment = &appsv1.Deployment{ObjectMeta: singleGlooProxy}
	singleSvcProxyService    = &corev1.Service{ObjectMeta: singleGlooProxy}

	multiGlooProxy = metav1.ObjectMeta{
		Name:      "multi-tcp-gateway",
		Namespace: multiSvcNsName,
	}
	multiProxyDeployment = &appsv1.Deployment{ObjectMeta: multiGlooProxy}
	multiProxyService    = &corev1.Service{ObjectMeta: multiGlooProxy}

	// Expected curl responses from tests
	expectedSingleSvcResp = &testmatchers.HttpResponse{
		StatusCode: http.StatusOK,
		Body: gomega.SatisfyAll(
			gomega.MatchRegexp(fmt.Sprintf(`"namespace"\s*:\s*"%s"`, singleSvcNsName)),
			gomega.MatchRegexp(`"service"\s*:\s*"single-svc"`),
		),
	}

	crossNsGlooProxy = metav1.ObjectMeta{
		Name:      "gateway",
		Namespace: crossNsClientName,
	}
	crossNsProxyDeployment = &appsv1.Deployment{ObjectMeta: crossNsGlooProxy}
	crossNsProxyService    = &corev1.Service{ObjectMeta: crossNsGlooProxy}

	crossNsNoRefGrantGlooProxy = metav1.ObjectMeta{
		Name:      "gateway",
		Namespace: crossNsNoRefGrantClientNsName,
	}
	crossNsNoRefGrantProxyDeployment = &appsv1.Deployment{ObjectMeta: crossNsNoRefGrantGlooProxy}
	crossNsNoRefGrantProxyService    = &corev1.Service{ObjectMeta: crossNsNoRefGrantGlooProxy}

	expectedMultiSvc1Resp = &testmatchers.HttpResponse{
		StatusCode: http.StatusOK,
		Body: gomega.SatisfyAll(
			gomega.MatchRegexp(fmt.Sprintf(`"namespace"\s*:\s*"%s"`, multiSvcNsName)),
			gomega.MatchRegexp(fmt.Sprintf(`"service"\s*:\s*"%s"`, multiSvc1Name)),
		),
	}

	expectedMultiSvc2Resp = &testmatchers.HttpResponse{
		StatusCode: http.StatusOK,
		Body: gomega.SatisfyAll(
			gomega.MatchRegexp(fmt.Sprintf(`"namespace"\s*:\s*"%s"`, multiSvcNsName)),
			gomega.MatchRegexp(fmt.Sprintf(`"service"\s*:\s*"%s"`, multiSvc2Name)),
		),
	}

	expectedCrossNsResp = &testmatchers.HttpResponse{
		StatusCode: http.StatusOK,
		Body: gomega.SatisfyAll(
			gomega.MatchRegexp(fmt.Sprintf(`"namespace"\s*:\s*"%s"`, crossNsBackendNsName)),
			gomega.MatchRegexp(fmt.Sprintf(`"service"\s*:\s*"%s"`, crossNsBackendSvcName)),
		),
	}
)

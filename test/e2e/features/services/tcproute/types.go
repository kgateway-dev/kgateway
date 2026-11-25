//go:build e2e

package tcproute

import (
	"fmt"
	"net/http"
	"path/filepath"
	"time"

	"github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
)

const (
	// The Gateway and curl client shared by every subtest, from setup.yaml
	gatewayName = "tcp-gateway"
	gatewayNs   = "tcp-route"

	// Constants used by TestConfigureTCPRouteBackingDestinationsWithSingleService
	singleSvcNsName           = "single-tcp-route"
	singleSvcListenerName8087 = "listener-8087"
	singleSvcTCPRouteName     = "single-tcp-route"

	// Constants used by TestConfigureTCPRouteBackingDestinationsWithMultiServices
	multiSvcNsName           = "multi-tcp-route"
	multiSvcListenerName8089 = "listener-8089"
	multiSvcListenerName8088 = "listener-8088"
	multiSvc1Name            = "multi-svc-1"
	multiSvc2Name            = "multi-svc-2"
	multiSvcTCPRouteName1    = "tcp-route-1"
	multiSvcTCPRouteName2    = "tcp-route-2"

	// Constants for CrossNamespaceTCPRouteWithReferenceGrant
	crossNsTestName       = "CrossNamespaceTCPRouteWithReferenceGrant"
	crossNsClientName     = "cross-namespace-allowed-client-ns"
	crossNsBackendNsName  = "cross-namespace-allowed-backend-ns"
	crossNsListenerName   = "listener-8080"
	crossNsBackendSvcName = "backend-svc"
	crossNsTCPRouteName   = "tcp-route"

	// Constants for CrossNamespaceTCPRouteWithoutReferenceGrant
	crossNsNoRefGrantTestName      = "CrossNamespaceTCPRouteWithoutReferenceGrant"
	crossNsNoRefGrantClientNsName  = "client-ns-no-refgrant"
	crossNsNoRefGrantBackendNsName = "backend-ns-no-refgrant"
	crossNsNoRefGrantListenerName  = "listener-8081"
	crossNsNoRefGrantTCPRouteName  = "tcp-route"

	// Constants for TCPRoute with TLS listener in the same namespace
	tlsListenerSameNsTestName = "TCPRouteWithTLSListenerSameNamespace"
	tlsListenerNsName         = "tcp-route-tls"
	tlsListenerListenerName   = "listener-8443"
	tlsListenerServiceName    = "tls-listener-svc"
	tlsListenerTCPRouteName   = "tls-listener-tcproute"
)

var (
	setupManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "setup.yaml")

	// Variables used by TestConfigureTCPRouteBackingDestinationsWithSingleService
	multiSvcNsManifest       = filepath.Join(fsutils.MustGetThisDir(), "testdata", "multi-ns.yaml")
	multiSvcBackendManifest  = filepath.Join(fsutils.MustGetThisDir(), "testdata", "multi-backend-service.yaml")
	multiSvcTcpRouteManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "multi-tcproute.yaml")

	// Variables used by TestConfigureTCPRouteBackingDestinationsWithMultiServices
	singleSvcNsManifest       = filepath.Join(fsutils.MustGetThisDir(), "testdata", "single-ns.yaml")
	singleSvcBackendManifest  = filepath.Join(fsutils.MustGetThisDir(), "testdata", "single-backend-service.yaml")
	singleSvcTcpRouteManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "single-tcproute.yaml")

	// Manifests for CrossNamespaceTCPRouteWithReferenceGrant
	crossNsClientNsManifest   = filepath.Join(fsutils.MustGetThisDir(), "testdata", "cross-ns-client-ns.yaml")
	crossNsBackendNsManifest  = filepath.Join(fsutils.MustGetThisDir(), "testdata", "cross-ns-backend-ns.yaml")
	crossNsBackendSvcManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "cross-ns-backend-service.yaml")
	crossNsTCPRouteManifest   = filepath.Join(fsutils.MustGetThisDir(), "testdata", "cross-ns-tcproute.yaml")
	crossNsRefGrantManifest   = filepath.Join(fsutils.MustGetThisDir(), "testdata", "cross-ns-referencegrant.yaml")

	// Manifests for CrossNamespaceTCPRouteWithoutReferenceGrant
	crossNsNoRefGrantClientNsManifest   = filepath.Join(fsutils.MustGetThisDir(), "testdata", "cross-ns-no-refgrant-client-ns.yaml")
	crossNsNoRefGrantBackendNsManifest  = filepath.Join(fsutils.MustGetThisDir(), "testdata", "cross-ns-no-refgrant-backend-ns.yaml")
	crossNsNoRefGrantBackendSvcManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "cross-ns-no-refgrant-backend-service.yaml")
	crossNsNoRefGrantTCPRouteManifest   = filepath.Join(fsutils.MustGetThisDir(), "testdata", "cross-ns-no-refgrant-tcproute.yaml")

	// Manifests for TLS listener TCPRoute same-namespace test
	tlsListenerNsManifest       = filepath.Join(fsutils.MustGetThisDir(), "testdata", "tls-tcp-ns.yaml")
	tlsListenerBackendManifest  = filepath.Join(fsutils.MustGetThisDir(), "testdata", "tls-tcp-backend-service.yaml")
	tlsListenerTcpRouteManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "tls-tcp-tcproute.yaml")

	// Assertion test timers
	ctxTimeout = 10 * time.Minute
	timeout    = 60 * time.Second

	proxyObjectMeta = metav1.ObjectMeta{
		Name:      gatewayName,
		Namespace: gatewayNs,
	}

	// Expected curl responses from tests
	expectedSingleSvcResp = &testmatchers.HttpResponse{
		StatusCode: http.StatusOK,
		Body: gomega.SatisfyAll(
			gomega.MatchRegexp(fmt.Sprintf(`"namespace"\s*:\s*"%s"`, singleSvcNsName)),
			gomega.MatchRegexp(`"service"\s*:\s*"single-svc"`),
		),
	}

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

	expectedTlsListenerResp = &testmatchers.HttpResponse{
		StatusCode: http.StatusOK,
		Body: gomega.SatisfyAll(
			gomega.MatchRegexp(fmt.Sprintf(`"namespace"\s*:\s*"%s"`, tlsListenerNsName)),
			gomega.MatchRegexp(fmt.Sprintf(`"service"\s*:\s*"%s"`, tlsListenerServiceName)),
		),
	}
)

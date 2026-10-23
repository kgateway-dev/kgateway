//go:build e2e

package tlsroute

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
	gatewayName = "tls-gateway"
	gatewayNs   = "tls-route"

	// Constants used by TestConfigureTLSRouteBackingDestinationsWithSingleService
	singleSvcNsName           = "single-tls-route"
	singleSvcListenerName6443 = "listener-6443"
	singleSvcTLSRouteName     = "single-tls-route"

	// Constants used by TestConfigureTLSRouteBackingDestinationsWithMultiServices
	multiSvcNsName           = "multi-tls-route"
	multiSvcListenerName6444 = "listener-6444"
	multiSvcListenerName8443 = "listener-8443"
	multiSvc1Name            = "multi-svc-1"
	multiSvc2Name            = "multi-svc-2"
	multiSvcTLSRouteName1    = "tls-route-1"
	multiSvcTLSRouteName2    = "tls-route-2"

	// Constants for CrossNamespaceTLSRouteWithReferenceGrant
	crossNsTestName       = "CrossNamespaceTLSRouteWithReferenceGrant"
	crossNsClientName     = "cross-namespace-allowed-client-ns"
	crossNsBackendNsName  = "cross-namespace-allowed-backend-ns"
	crossNsListenerName   = "listener-8444"
	crossNsBackendSvcName = "backend-svc"
	crossNsTLSRouteName   = "tls-route"

	// Constants for CrossNamespaceTLSRouteWithoutReferenceGrant
	crossNsNoRefGrantTestName      = "CrossNamespaceTLSRouteWithoutReferenceGrant"
	crossNsNoRefGrantClientNsName  = "client-ns-no-refgrant"
	crossNsNoRefGrantBackendNsName = "backend-ns-no-refgrant"
	crossNsNoRefGrantListenerName  = "listener-8445"
	crossNsNoRefGrantTLSRouteName  = "tls-route"
)

var (
	setupManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "setup.yaml")

	// Variables used by TestConfigureTCPRouteBackingDestinationsWithSingleService
	multiSvcNsManifest       = filepath.Join(fsutils.MustGetThisDir(), "testdata", "multi-ns.yaml")
	multiSvcBackendManifest  = filepath.Join(fsutils.MustGetThisDir(), "testdata", "multi-backend-service.yaml")
	multiSvcTlsRouteManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "multi-tlsroute.yaml")

	// Variables used by TestConfigureTCPRouteBackingDestinationsWithMultiServices
	singleSvcNsManifest       = filepath.Join(fsutils.MustGetThisDir(), "testdata", "single-ns.yaml")
	singleSvcBackendManifest  = filepath.Join(fsutils.MustGetThisDir(), "testdata", "single-backend-service.yaml")
	singleSvcTLSRouteManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "single-tlsroute.yaml")
	singleSecretManifest      = filepath.Join(fsutils.MustGetThisDir(), "testdata", "tls-secret.yaml")

	// Manifests for CrossNamespaceTLSRouteWithReferenceGrant
	crossNsClientNsManifest   = filepath.Join(fsutils.MustGetThisDir(), "testdata", "cross-ns-client-ns.yaml")
	crossNsBackendNsManifest  = filepath.Join(fsutils.MustGetThisDir(), "testdata", "cross-ns-backend-ns.yaml")
	crossNsBackendSvcManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "cross-ns-backend-service.yaml")
	crossNsTLSRouteManifest   = filepath.Join(fsutils.MustGetThisDir(), "testdata", "cross-ns-tlsroute.yaml")
	crossNsRefGrantManifest   = filepath.Join(fsutils.MustGetThisDir(), "testdata", "cross-ns-referencegrant.yaml")

	// Manifests for CrossNamespaceTCPRouteWithoutReferenceGrant
	crossNsNoRefGrantClientNsManifest   = filepath.Join(fsutils.MustGetThisDir(), "testdata", "cross-ns-no-refgrant-client-ns.yaml")
	crossNsNoRefGrantBackendNsManifest  = filepath.Join(fsutils.MustGetThisDir(), "testdata", "cross-ns-no-refgrant-backend-ns.yaml")
	crossNsNoRefGrantBackendSvcManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "cross-ns-no-refgrant-backend-service.yaml")
	crossNsNoRefGrantTLSRouteManifest   = filepath.Join(fsutils.MustGetThisDir(), "testdata", "cross-ns-no-refgrant-tlsroute.yaml")

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
)

//go:build e2e

package listener_policy

import (
	"path/filepath"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils/kubectl"
)

// In-pod paths the curl-mtls pod mounts the alice client cert and CA bundle to.
// Must match the volumeMounts on the curl-mtls Pod in testdata/setup.yaml.
const (
	forwardClientCertAliceCertPath = "/etc/forward-client-cert/client/tls.crt"
	forwardClientCertAliceKeyPath  = "/etc/forward-client-cert/client/tls.key"
	forwardClientCertCAPath        = "/etc/forward-client-cert/ca/ca.crt"
)

// Ports of the per-test listeners on the shared gw-mtls Gateway. Must match
// testdata/forward-client-cert/gateway.yaml.
const (
	fccBaselinePort           = 8443
	fccSanitizeSetDefaultPort = 8444
	fccSanitizeSetAllPort     = 8445
	fccAppendForwardPort      = 8446
	fccSanitizePort           = 8447
	fccForwardOnlyPort        = 8448
)

var (
	setupManifest                           = filepath.Join(fsutils.MustGetThisDir(), "testdata", "setup.yaml")
	gatewayManifest                         = filepath.Join(fsutils.MustGetThisDir(), "testdata", "gateway.yaml")
	httpRouteManifest                       = filepath.Join(fsutils.MustGetThisDir(), "testdata", "httproute.yaml")
	rawHeaderEchoManifest                   = filepath.Join(fsutils.MustGetThisDir(), "testdata", "raw-header-echo.yaml")
	allFieldsManifest                       = filepath.Join(fsutils.MustGetThisDir(), "testdata", "listener-policy-all-fields.yaml")
	serverHeaderManifest                    = filepath.Join(fsutils.MustGetThisDir(), "testdata", "listener-policy-server-header.yaml")
	preserveHttp1HeaderCaseManifest         = filepath.Join(fsutils.MustGetThisDir(), "testdata", "preserve-http1-header-case.yaml")
	accessLogManifest                       = filepath.Join(fsutils.MustGetThisDir(), "testdata", "listener-policy-access-log.yaml")
	httpListenerPolicyMissingTargetManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "listener-policy-missing-target.yaml")
	earlyHeaderMutationManifest             = filepath.Join(fsutils.MustGetThisDir(), "testdata", "listener-policy-early-header-route-match.yaml")
	http2ProtocolOptionsManifest            = filepath.Join(fsutils.MustGetThisDir(), "testdata", "listener-policy-http2-protocol-options.yaml")
	proxyProtocolManifest                   = filepath.Join(fsutils.MustGetThisDir(), "testdata", "listener-policy-proxy-protocol.yaml")
	maxRequestsPerConnectionManifest        = filepath.Join(fsutils.MustGetThisDir(), "testdata", "listener-policy-max-requests-per-connection.yaml")
	maxHeadersCountManifest                 = filepath.Join(fsutils.MustGetThisDir(), "testdata", "listener-policy-max-headers-count.yaml")
	stripHostPortAnyPortManifest            = filepath.Join(fsutils.MustGetThisDir(), "testdata", "listener-policy-strip-host-port-any-port.yaml")
	stripHostPortMatchingPortManifest       = filepath.Join(fsutils.MustGetThisDir(), "testdata", "listener-policy-strip-host-port-matching-port.yaml")

	localReplyConfigManifest    = filepath.Join(fsutils.MustGetThisDir(), "testdata", "listener-policy-local-reply-config.yaml")
	localReplyHttpRouteManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "local-reply-httproute.yaml")

	// RequestID test manifests for testing the new RequestID configuration feature
	listenerPolicyRequestIdManifest                = filepath.Join(fsutils.MustGetThisDir(), "testdata", "listener-policy-request-id.yaml")
	requestIdEchoManifest                          = filepath.Join(fsutils.MustGetThisDir(), "testdata", "request-id-echo.yaml")
	requestIdEchoRouteManifest                     = filepath.Join(fsutils.MustGetThisDir(), "testdata", "request-id-echo-route.yaml")
	listenerPolicyRequestIdPackTraceReasonManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "listener-policy-request-id-pack-trace-reason.yaml")

	// forwardClientCertDetails test manifests. Everything but the per-test
	// forward-client-cert policies is applied at suite setup time.
	forwardClientCertGateway        = filepath.Join(fsutils.MustGetThisDir(), "testdata", "forward-client-cert", "gateway.yaml")
	forwardClientCertServerSecret   = filepath.Join(fsutils.MustGetThisDir(), "testdata", "forward-client-cert", "server-cert-secret.yaml")
	forwardClientCertCASecret       = filepath.Join(fsutils.MustGetThisDir(), "testdata", "forward-client-cert", "client-ca-secret.yaml")
	forwardClientCertAliceSecret    = filepath.Join(fsutils.MustGetThisDir(), "testdata", "forward-client-cert", "client-alice-secret.yaml")
	forwardClientCertRouteManifest  = filepath.Join(fsutils.MustGetThisDir(), "testdata", "forward-client-cert", "route.yaml")
	forwardClientCertMtlsValidation = filepath.Join(fsutils.MustGetThisDir(), "testdata", "forward-client-cert", "mtls-validation.yaml")
	forwardClientCertSanitizeSetDef = filepath.Join(fsutils.MustGetThisDir(), "testdata", "forward-client-cert", "policy-sanitize-set-default.yaml")
	forwardClientCertSanitizeSetAll = filepath.Join(fsutils.MustGetThisDir(), "testdata", "forward-client-cert", "policy-sanitize-set-all.yaml")
	forwardClientCertAppendForward  = filepath.Join(fsutils.MustGetThisDir(), "testdata", "forward-client-cert", "policy-append-forward.yaml")
	forwardClientCertSanitize       = filepath.Join(fsutils.MustGetThisDir(), "testdata", "forward-client-cert", "policy-sanitize.yaml")
	forwardClientCertForwardOnly    = filepath.Join(fsutils.MustGetThisDir(), "testdata", "forward-client-cert", "policy-forward-only.yaml")

	// Cert-mounted curl pod for outgoing mTLS requests in TestForwardClientCert*.
	curlMtlsPodExecOpt = kubectl.PodExecOptions{
		Name:      "curl-mtls",
		Namespace: "curl",
		Container: "curl",
	}

	// When we apply the setup file, we expect resources to be created with this metadata
	proxyObjectMeta = metav1.ObjectMeta{
		Name:      "gw",
		Namespace: "default",
	}
	proxyService    = &corev1.Service{ObjectMeta: proxyObjectMeta}
	proxyDeployment = &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gw",
			Namespace: "default",
		},
	}

	// Service of the gw-mtls Gateway shared by the TestForwardClientCert* tests.
	mtlsProxyService = &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "gw-mtls", Namespace: "default"}}
)

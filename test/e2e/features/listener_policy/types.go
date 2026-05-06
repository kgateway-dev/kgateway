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

var (
	setupManifest                           = filepath.Join(fsutils.MustGetThisDir(), "testdata", "setup.yaml")
	gatewayManifest                         = filepath.Join(fsutils.MustGetThisDir(), "testdata", "gateway.yaml")
	httpRouteManifest                       = filepath.Join(fsutils.MustGetThisDir(), "testdata", "httproute.yaml")
	allFieldsManifest                       = filepath.Join(fsutils.MustGetThisDir(), "testdata", "listener-policy-all-fields.yaml")
	serverHeaderManifest                    = filepath.Join(fsutils.MustGetThisDir(), "testdata", "listener-policy-server-header.yaml")
	preserveHttp1HeaderCaseManifest         = filepath.Join(fsutils.MustGetThisDir(), "testdata", "preserve-http1-header-case.yaml")
	accessLogManifest                       = filepath.Join(fsutils.MustGetThisDir(), "testdata", "listener-policy-access-log.yaml")
	httpListenerPolicyMissingTargetManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "listener-policy-missing-target.yaml")
	earlyHeaderMutationManifest             = filepath.Join(fsutils.MustGetThisDir(), "testdata", "listener-policy-early-header-route-match.yaml")
	proxyProtocolManifest                   = filepath.Join(fsutils.MustGetThisDir(), "testdata", "listener-policy-proxy-protocol.yaml")
	// RequestID test manifests for testing the new RequestID configuration feature
	listenerPolicyRequestIdManifest     = filepath.Join(fsutils.MustGetThisDir(), "testdata", "listener-policy-request-id.yaml")
	requestIdEchoManifest               = filepath.Join(fsutils.MustGetThisDir(), "testdata", "request-id-echo.yaml")
	httpListenerPolicyRequestIdManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "httplistener-policy-request-id.yaml")

	// forwardClientCertDetails test manifests. The three Secret YAMLs are
	// applied at suite setup time so the gw mtls-https listener and the
	// curl-mtls pod always have their cert material available.
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
	curlMtlsPod = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "curl-mtls",
			Namespace: "curl",
		},
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
	nginxPod = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx",
			Namespace: "default",
		},
	}
	exampleSvc = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-svc",
			Namespace: "default",
		},
	}
	echoService = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "raw-header-echo",
			Namespace: "default",
		},
	}
	echoDeployment = &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "raw-header-echo",
			Namespace: "default",
		},
	}
	requestIdEchoService = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "request-id-echo",
			Namespace: "default",
		},
	}
	requestIdEchoDeployment = &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "request-id-echo",
			Namespace: "default",
		},
	}
)

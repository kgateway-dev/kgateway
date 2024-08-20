package server_tls

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/onsi/gomega"
	kubev1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1/kube/apis/gateway.solo.io/v1"
	"github.com/solo-io/gloo/test/gomega/matchers"
	"github.com/solo-io/skv2/codegen/util"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	tlsSecret1Manifest      = func() ([]byte, error) { return manifestFromFile("tls-secret-1.yaml") }
	tlsSecret2Manifest      = func() ([]byte, error) { return manifestFromFile("tls-secret-2.yaml") }
	tlsSecretWithCaManifest = func() ([]byte, error) { return manifestFromFile("tls-secret-with-ca.yaml") }
	vs1Manifest             = func() ([]byte, error) { return manifestFromFile("vs-1.yaml") }
	vs2Manifest             = func() ([]byte, error) { return manifestFromFile("vs-2.yaml") }
	vsWithOneWayManifest    = func() ([]byte, error) { return manifestFromFile("vs-with-oneway.yaml") }
	vsWithoutOneWayManifest = func() ([]byte, error) { return manifestFromFile("vs-without-oneway.yaml") }

	// When we apply the deployer-provision.yaml file, we expect resources to be created with this metadata
	glooProxyObjectMeta = func(ns string) metav1.ObjectMeta {
		return metav1.ObjectMeta{
			Name:      "gloo-proxy-gw",
			Namespace: ns,
		}
	}
	proxyDeployment = func(ns string) *appsv1.Deployment {
		return &appsv1.Deployment{ObjectMeta: glooProxyObjectMeta(ns)}
	}
	proxyService = func(ns string) *corev1.Service {
		return &corev1.Service{ObjectMeta: glooProxyObjectMeta(ns)}
	}

	vs1 = func(ns string) *kubev1.VirtualService {
		return &kubev1.VirtualService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "vs-1",
				Namespace: ns,
			},
		}
	}
	vs2 = func(ns string) *kubev1.VirtualService {
		return &kubev1.VirtualService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "vs-2",
				Namespace: ns,
			},
		}
	}
	vsWithOneWay = func(ns string) *kubev1.VirtualService {
		return &kubev1.VirtualService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "vs-with-oneway",
				Namespace: ns,
			},
		}
	}
	vsWithoutOneWay = func(ns string) *kubev1.VirtualService {
		return &kubev1.VirtualService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "vs-without-oneway",
				Namespace: ns,
			},
		}
	}
	tlsSecret1 = func(ns string) *corev1.Secret {
		return &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "tls-secret-1",
				Namespace: ns,
			},
		}
	}
	tlsSecret2 = func(ns string) *corev1.Secret {
		return &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "tls-secret-2",
				Namespace: ns,
			},
		}
	}
	tlsSecretWithCa = func(ns string) *corev1.Secret {
		return &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "tls-secret-with-ca",
				Namespace: ns,
			},
		}
	}

	expectedHealthyResponse1 = &matchers.HttpResponse{
		StatusCode: http.StatusOK,
		Body:       gomega.ContainSubstring("success from vs-1"),
	}
	expectedHealthyResponse2 = &matchers.HttpResponse{
		StatusCode: http.StatusOK,
		Body:       gomega.ContainSubstring("success from vs-2"),
	}
	expectedHealthyResponseWithOneWay = &matchers.HttpResponse{
		StatusCode: http.StatusOK,
		Body:       gomega.ContainSubstring("success from vs-with-oneway"),
	}

	coreSecretGVK = schema.GroupVersionKind{
		Version: "v1",
		Group:   "",
		Kind:    "Secret",
	}
)

const (
	// These codes are defined at https://curl.se/libcurl/c/libcurl-errors.html.
	// These were determined experimentally.
	expectedFailedResponseCodeInvalidVs = 16
	expectedFailedResponseCertRequested = 35
)

func manifestFromFile(fname string) ([]byte, error) {
	return withSubstitutions(filepath.Join(util.MustGetThisDir(), "testdata", fname))
}
func withSubstitutions(fname string) ([]byte, error) {
	// VS with secret should be accepted, need to substitute the secret ns
	raw, err := os.ReadFile(fname)
	if err != nil {
		return nil, err
	}

	// Replace environment variables placeholders with their values
	return []byte(os.ExpandEnv(string(raw))), nil
}

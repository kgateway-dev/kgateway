package client_tls

import (
	"net/http"
	"path/filepath"

	"github.com/onsi/gomega"
	kubev1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1/kube/apis/gateway.solo.io/v1"
	"github.com/solo-io/gloo/test/gomega/matchers"
	"github.com/solo-io/skv2/codegen/util"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	tlsSecret1ManifestFile      = filepath.Join(util.MustGetThisDir(), "testdata", "tls-secret-1.yaml")
	tlsSecret2ManifestFile      = filepath.Join(util.MustGetThisDir(), "testdata", "tls-secret-2.yaml")
	tlsSecretWithCaManifestFile = filepath.Join(util.MustGetThisDir(), "testdata", "tls-secret-with-ca.yaml")
	vs1ManifestFile             = filepath.Join(util.MustGetThisDir(), "testdata", "vs-1.yaml")
	vs2ManifestFile             = filepath.Join(util.MustGetThisDir(), "testdata", "vs-2.yaml")
	vsWithOneWayManifestFile    = filepath.Join(util.MustGetThisDir(), "testdata", "vs-with-oneway.yaml")
	vsWithoutOneWayManifestFile = filepath.Join(util.MustGetThisDir(), "testdata", "vs-without-oneway.yaml")

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
	expectedHealthyResponseWithoutOneWay = &matchers.HttpResponse{
		StatusCode: http.StatusOK,
		Body:       gomega.ContainSubstring("success from vs-without-oneway"),
	}
	expectedCertVerifyFailedResponse = &matchers.HttpResponse{
		StatusCode: http.StatusServiceUnavailable,
		Body:       gomega.ContainSubstring("CERTIFICATE_VERIFY_FAILED"),
	}
	expectedNoFilterChainFailedResponse = &matchers.HttpResponse{
		StatusCode: http.StatusServiceUnavailable,
		Body:       gomega.ContainSubstring("OpenSSL SSL_connect: SSL_ERROR_SYSCALL in connection"),
	}
)

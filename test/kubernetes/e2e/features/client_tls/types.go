//go:build ignore

package client_tls

import (
	"net/http"
	"path/filepath"

	"github.com/onsi/gomega"
	"github.com/solo-io/skv2/codegen/util"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/test/gomega/matchers"
)

var (
	annotatedNginxSvcManifestFile       = filepath.Join(util.MustGetThisDir(), "testdata", "annotated-nginx-svc.yaml")
	annotatedNginxOneWaySvcManifestFile = filepath.Join(util.MustGetThisDir(), "testdata", "annotated-oneway-nginx-svc.yaml")
	nginxUpstreamManifestFile           = filepath.Join(util.MustGetThisDir(), "testdata", "nginx-upstream.yaml")
	nginxOneWayUpstreamManifestFile     = filepath.Join(util.MustGetThisDir(), "testdata", "nginx-oneway-upstream.yaml")
	tlsSecretManifestFile               = filepath.Join(util.MustGetThisDir(), "testdata", "tls-secret.yaml")

	// When we apply the deployer-provision.yaml file, we expect resources to be created with this metadata
	glooProxyObjectMeta = func(ns string) metav1.ObjectMeta {
		return metav1.ObjectMeta{
			Name:      "gloo-proxy-gw",
			Namespace: "default",
		}
	}
	proxyDeployment = func(ns string) *appsv1.Deployment {
		return &appsv1.Deployment{ObjectMeta: glooProxyObjectMeta(ns)}
	}
	proxyService = func(ns string) *corev1.Service {
		return &corev1.Service{ObjectMeta: glooProxyObjectMeta(ns)}
	}

	tlsSecret = &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-tls",
			Namespace: "nginx",
		},
	}

	expectedHealthyResponse = &matchers.HttpResponse{
		StatusCode: http.StatusOK,
		Body:       gomega.ContainSubstring("Welcome to nginx!"),
	}
	expectedCertVerifyFailedResponse = &matchers.HttpResponse{
		StatusCode: http.StatusServiceUnavailable,
		Body:       gomega.ContainSubstring("CERTIFICATE_VERIFY_FAILED"),
	}
)

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
	annotatedNginxSvcManifestFile       = filepath.Join(util.MustGetThisDir(), "testdata", "annotated-nginx-svc.yaml")
	annotatedNginxOneWaySvcManifestFile = filepath.Join(util.MustGetThisDir(), "testdata", "annotated-oneway-nginx-svc.yaml")
	nginxUpstreamManifestFile           = filepath.Join(util.MustGetThisDir(), "testdata", "nginx-upstream.yaml")
	nginxOneWayUpstreamManifestFile     = filepath.Join(util.MustGetThisDir(), "testdata", "nginx-oneway-upstream.yaml")
	tlsSecretManifestFile               = filepath.Join(util.MustGetThisDir(), "testdata", "tls-secret.yaml")
	vsTargetingKubeManifestFile         = filepath.Join(util.MustGetThisDir(), "testdata", "vs-targeting-kube.yaml")
	vsTargetingUpstreamManifestFile     = filepath.Join(util.MustGetThisDir(), "testdata", "vs-targeting-upstream.yaml")

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

	vsTargetingKube = func(ns string) *kubev1.VirtualService {
		return &kubev1.VirtualService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "vs-targeting-kube",
				Namespace: ns,
			},
		}
	}
	vsTargetingUpstream = func(ns string) *kubev1.VirtualService {
		return &kubev1.VirtualService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "vs-targeting-upstream",
				Namespace: ns,
			},
		}
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

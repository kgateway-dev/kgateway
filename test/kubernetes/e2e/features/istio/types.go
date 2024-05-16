package istio

import (
	"net/http"
	"path/filepath"

	"github.com/onsi/gomega"
	testmatchers "github.com/solo-io/gloo/test/gomega/matchers"
	"github.com/solo-io/skv2/codegen/util"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/solo-io/gloo/pkg/utils/kubeutils/kubectl"
)

var (
	setupManifest = filepath.Join(util.MustGetThisDir(), "testdata", "setup.yaml")

	strictPeerAuthManifest     = filepath.Join(util.MustGetThisDir(), "testdata", "strict-peer-auth.yaml")
	permissivePeerAuthManifest = filepath.Join(util.MustGetThisDir(), "testdata", "permissive-peer-auth.yaml")
	disablePeerAuthManifest    = filepath.Join(util.MustGetThisDir(), "testdata", "disable-peer-auth.yaml")

	k8sRoutingSvcManifest      = filepath.Join(util.MustGetThisDir(), "testdata", "k8s-routing-svc.yaml")
	k8sRoutingUpstreamManifest = filepath.Join(util.MustGetThisDir(), "testdata", "k8s-routing-upstream.yaml")

	// When we apply the fault injection manifest files, we expect resources to be created with this metadata
	glooProxyObjectMeta = metav1.ObjectMeta{
		Name:      "gloo-proxy-gw",
		Namespace: "default",
	}
	proxyDeployment = &appsv1.Deployment{ObjectMeta: glooProxyObjectMeta}
	proxyService    = &corev1.Service{ObjectMeta: glooProxyObjectMeta}

	// httpbinDeployment is the Deployment that is in the Istio mesh
	httpbinDeployment = &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "httpbin",
			Namespace: "httpbin",
		},
	}

	// curlPod is the Pod that will be used to execute curl requests, and is defined in the fault injection manifest files
	curlPodExecOpt = kubectl.PodExecOptions{
		Name:      "curl",
		Namespace: "curl",
		Container: "curl",
	}

	expectedMtlsResponse = &testmatchers.HttpResponse{
		StatusCode: http.StatusOK,
		Body:       gomega.ContainSubstring("X-Forwarded-Client-Cert"),
	}

	expectedPlaintextResponse = &testmatchers.HttpResponse{
		StatusCode: http.StatusOK,
		Body:       gomega.Not(gomega.ContainSubstring("X-Forwarded-Client-Cert")),
	}

	expectedServiceUnavailableResponse = &testmatchers.HttpResponse{
		StatusCode: http.StatusServiceUnavailable,
		Body:       gomega.ContainSubstring("upstream connect error or disconnect/reset before headers"),
	}
)

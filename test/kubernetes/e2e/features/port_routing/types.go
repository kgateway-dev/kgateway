package port_routing

import (
	"net/http"
	"path/filepath"

	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/utils/kubeutils/kubectl"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/solo-io/skv2/codegen/util"

	testmatchers "github.com/solo-io/gloo/test/gomega/matchers"
)

var (
	setupManifest = filepath.Join(util.MustGetThisDir(), "testdata/setup.yaml")

	invalidPortAndValidTargetportManifest   = filepath.Join(util.MustGetThisDir(), "testdata/invalid-port-and-valid-targetport.yaml")
	invalidPortAndInvalidTargetportManifest = filepath.Join(util.MustGetThisDir(), "testdata/invalid-port-and-invalid-targetport.yaml")
	matchPodPortWithoutTargetportManifest   = filepath.Join(util.MustGetThisDir(), "testdata/match-pod-port-without-targetport.yaml")
	matchPortandTargetportManifest          = filepath.Join(util.MustGetThisDir(), "testdata/match-port-and-targetport.yaml")
	invalidPortWithoutTargetportManifest    = filepath.Join(util.MustGetThisDir(), "testdata/invalid-port-without-targetport.yaml")

	// When we apply the setup.yaml file, we expect resources to be created with this metadata
	glooProxyObjectMeta = metav1.ObjectMeta{
		Name:      "gloo-proxy-gw",
		Namespace: "default",
	}
	proxyDeployment = &appsv1.Deployment{ObjectMeta: glooProxyObjectMeta}
	proxyService    = &corev1.Service{ObjectMeta: glooProxyObjectMeta}

	// curlPod is the Pod that will be used to execute curl requests, and is defined in the port routing setup.yaml manifest files
	curlPodExecOpt = kubectl.PodExecOptions{
		Name:      "curl",
		Namespace: "curl",
		Container: "curl",
	}

	expectedHealthyResponse = &testmatchers.HttpResponse{
		StatusCode: http.StatusOK,
		Body:       ContainSubstring("Welcome to nginx!"),
	}

	expectedServiceUnavailableResponse = &testmatchers.HttpResponse{
		StatusCode: http.StatusServiceUnavailable,
		Body:       ContainSubstring("upstream connect error or disconnect/reset before headers. reset reason: remote connection failure, transport failure reason: delayed connect error"),
	}
)

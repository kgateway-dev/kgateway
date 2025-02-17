//go:build ignore

package upstreams

import (
	"net/http"
	"path/filepath"

	"github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils/kubectl"

	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
)

var (
	routeWithUpstreamManifest = filepath.Join(fsutils.MustGetThisDir(), "inputs/route-with-upstream.yaml")
	upstreamManifest          = filepath.Join(fsutils.MustGetThisDir(), "inputs/upstream-for-route.yaml")

	// Proxy resource to be translated
	glooProxyObjectMeta = metav1.ObjectMeta{
		Name:      "gw",
		Namespace: "default",
	}
	proxyDeployment = &appsv1.Deployment{ObjectMeta: glooProxyObjectMeta}
	proxyService    = &corev1.Service{ObjectMeta: glooProxyObjectMeta}

	// curlPod is the Pod that will be used to execute curl requests, and is defined in the upstream manifest files
	curlPodExecOpt = kubectl.PodExecOptions{
		Name:      "curl",
		Namespace: "curl",
		Container: "curl",
	}

	expectedUpstreamResp = &testmatchers.HttpResponse{
		StatusCode: http.StatusOK,
		Body:       gomega.ContainSubstring("sunt aut facere repellat provident occaecati excepturi optio reprehenderit"),
	}

	// Upstream resource to be created
	upstreamMeta = metav1.ObjectMeta{
		Name:      "json-upstream",
		Namespace: "default",
	}
)

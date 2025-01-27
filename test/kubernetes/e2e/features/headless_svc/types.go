package headless_svc

import (
	"net/http"

	"github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/solo-io/gloo/pkg/utils/kubeutils/kubectl"
	testmatchers "github.com/solo-io/gloo/test/gomega/matchers"
)

var (
	// When we apply the manifest file, we expect resources to be created with this metadata
	k8sApiProxyObjectMeta = metav1.ObjectMeta{
		Name:      "gloo-proxy-gw",
		Namespace: "default",
	}
	k8sApiProxyDeployment = &appsv1.Deployment{ObjectMeta: k8sApiProxyObjectMeta}
	k8sApiProxyService    = &corev1.Service{ObjectMeta: k8sApiProxyObjectMeta}

	headlessService = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "headless-example-svc",
			Namespace: "default",
		},
	}

	curlPodExecOpt = kubectl.PodExecOptions{
		Name:      "curl",
		Namespace: "curl",
		Container: "curl",
	}

	expectedHealthyResponse = &testmatchers.HttpResponse{
		StatusCode: http.StatusOK,
		Body:       gomega.ContainSubstring("Welcome to nginx!"),
	}
)

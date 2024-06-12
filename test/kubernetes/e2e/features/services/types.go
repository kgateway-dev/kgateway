package services

import (
	"net/http"
	"path/filepath"

	"github.com/onsi/gomega/gstruct"
	"github.com/solo-io/gloo/test/kubernetes/e2e/defaults"

	"github.com/solo-io/skv2/codegen/util"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	testmatchers "github.com/solo-io/gloo/test/gomega/matchers"
)

var (
	routeWithServiceManifest = filepath.Join(util.MustGetThisDir(), "inputs/route-with-service.yaml")
	serviceManifest          = filepath.Join(util.MustGetThisDir(), "inputs/service-for-route.yaml")

	// Proxy resource to be translated
	glooProxyObjectMeta = metav1.ObjectMeta{
		Name:      "gloo-proxy-gw",
		Namespace: "default",
	}
	proxyDeployment = &appsv1.Deployment{ObjectMeta: glooProxyObjectMeta}
	proxyService    = &corev1.Service{ObjectMeta: glooProxyObjectMeta}

	// curlPod is the Pod that will be used to execute curl requests, and is defined in the upstream manifest files
	curlPodExecOpt = defaults.CurlPodExecOpt

	expectedSvcResp = &testmatchers.HttpResponse{
		StatusCode: http.StatusOK,
		Body:       gstruct.Ignore(),
	}
)

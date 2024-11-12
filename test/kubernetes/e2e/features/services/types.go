package services

import (
	"net/http"
	"path/filepath"

	testmatchers "github.com/solo-io/gloo/test/gomega/matchers"

	"github.com/solo-io/skv2/codegen/util"

	"github.com/onsi/gomega/gstruct"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	routeWithServiceManifest = filepath.Join(util.MustGetThisDir(), "testdata", "route-with-service.yaml")
	serviceManifest          = filepath.Join(util.MustGetThisDir(), "testdata", "service-for-route.yaml")

	// Proxy resource to be translated
	glooProxyObjectMeta = metav1.ObjectMeta{
		Name:      "gloo-proxy-gw",
		Namespace: "default",
	}
	proxyDeployment = &appsv1.Deployment{ObjectMeta: glooProxyObjectMeta}
	proxyService    = &corev1.Service{ObjectMeta: glooProxyObjectMeta}

	expectedSvcResp = &testmatchers.HttpResponse{
		StatusCode: http.StatusOK,
		Body:       gstruct.Ignore(),
	}
)

package tcproute

import (
	"net/http"
	"path/filepath"

	testmatchers "github.com/solo-io/gloo/test/gomega/matchers"

	"github.com/solo-io/skv2/codegen/util"

	"github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	multiListenerGatewayAndClientManifest  = filepath.Join(util.MustGetThisDir(), "testdata", "multi-listener-gateway-and-client.yaml")
	multiBackendServiceManifest            = filepath.Join(util.MustGetThisDir(), "testdata", "multi-backend-service.yaml")
	multiTcpRouteManifest                  = filepath.Join(util.MustGetThisDir(), "testdata", "multi-tcproute.yaml")
	singleListenerGatewayAndClientManifest = filepath.Join(util.MustGetThisDir(), "testdata", "single-listener-gateway-and-client.yaml")
	singleTcpRouteManifest                 = filepath.Join(util.MustGetThisDir(), "testdata", "single-tcproute.yaml")

	// Proxy resource to be translated
	glooProxyObjectMeta = metav1.ObjectMeta{
		Name:      "gloo-proxy-tcp-gateway",
		Namespace: "default",
	}
	proxyDeployment = &appsv1.Deployment{ObjectMeta: glooProxyObjectMeta}
	proxyService    = &corev1.Service{ObjectMeta: glooProxyObjectMeta}

	expectedTcpFooSvcResp = &testmatchers.HttpResponse{
		StatusCode: http.StatusOK,
		Body: gomega.SatisfyAll(
			gomega.MatchRegexp(`"namespace"\s*:\s*"default"`),
			gomega.MatchRegexp(`"service"\s*:\s*"foo"`),
		),
	}

	expectedTcpBarSvcResp = &testmatchers.HttpResponse{
		StatusCode: http.StatusOK,
		Body: gomega.SatisfyAll(
			gomega.MatchRegexp(`"namespace"\s*:\s*"default"`),
			gomega.MatchRegexp(`"service"\s*:\s*"bar"`),
		),
	}
)

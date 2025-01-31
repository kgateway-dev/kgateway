//go:build ignore

package httproute

import (
	"net/http"
	"path/filepath"

	testmatchers "github.com/kgateway-dev/kgateway/test/gomega/matchers"

	"github.com/kgateway-dev/kgateway/pkg/utils/fsutils"

	"github.com/kgateway-dev/kgateway/projects/gateway2/crds"
	"github.com/onsi/gomega/gstruct"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	routeWithServiceManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "route-with-service.yaml")
	serviceManifest          = filepath.Join(fsutils.MustGetThisDir(), "testdata", "service-for-route.yaml")
	tcpRouteCrdManifest      = filepath.Join(crds.AbsPathToCrd("tcproute-crd.yaml"))

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

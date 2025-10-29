//go:build e2e

package httproute

import (
	"net/http"
	"path/filepath"

	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"

	"github.com/onsi/gomega/gstruct"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/crds"
)

var (
	routeWithServiceManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "route-with-service.yaml")
	serviceManifest          = filepath.Join(fsutils.MustGetThisDir(), "testdata", "service-for-route.yaml")
	tcpRouteCrdManifest      = filepath.Join(crds.AbsPathToCrd("tcproute-crd.yaml"))

	// Proxy resource to be translated
	proxyObjectMeta = metav1.ObjectMeta{
		Name:      "gw",
		Namespace: "default",
	}
	proxyDeployment = &appsv1.Deployment{ObjectMeta: proxyObjectMeta}
	proxyService    = &corev1.Service{ObjectMeta: proxyObjectMeta}

	expectedSvcResp = &testmatchers.HttpResponse{
		StatusCode: http.StatusOK,
		Body:       gstruct.Ignore(),
	}

	nginxMeta = metav1.ObjectMeta{
		Name:      "nginx",
		Namespace: "default",
	}
)

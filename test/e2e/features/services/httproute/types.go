//go:build e2e

package httproute

import (
	"net/http"
	"path/filepath"

	"github.com/onsi/gomega/gstruct"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/crds"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
)

const (
	kgatewayControllerName = wellknown.DefaultGatewayControllerName
	otherControllerName    = "other-controller.example.com/controller"
)

var (
	routeWithServiceManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "route-with-service.yaml")
	routeWithGwManifest      = filepath.Join(fsutils.MustGetThisDir(), "testdata", "route-with-gw.yaml")
	routeMissingGwManifest   = filepath.Join(fsutils.MustGetThisDir(), "testdata", "route-missing-gw.yaml")
	serviceManifest          = filepath.Join(fsutils.MustGetThisDir(), "testdata", "service-for-route.yaml")
	tcpRouteCrdManifest      = filepath.Join(crds.AbsPathToCrd("tcproute-crd.yaml"))

	expectedSvcResp = &testmatchers.HttpResponse{
		StatusCode: http.StatusOK,
		Body:       gstruct.Ignore(),
	}

	nginxMeta = metav1.ObjectMeta{
		Name:      "nginx",
		Namespace: "default",
	}
)

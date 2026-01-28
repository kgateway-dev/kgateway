//go:build e2e

package extproc

import (
	"path/filepath"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
)

var (
	gateway = metav1.ObjectMeta{
		Name:      "gw",
		Namespace: "default",
	}

	gatewayService = metav1.ObjectMeta{
		Name:      "gw",
		Namespace: "default",
	}

	timeout = 60 * time.Second

	gatewayManifest                  = getTestFile("gateway.yaml")
	routeWithTargetReferenceManifest = getTestFile("httproute-targetref.yaml")
	gatewayTargetReferenceManifest   = getTestFile("gateway-targetref.yaml")
	backendWithServiceManifest       = getTestFile("backend-service.yaml")
)

func getTestFile(filename string) string {
	return filepath.Join(fsutils.MustGetThisDir(), "testdata", filename)
}

package transformation

import (
	"path/filepath"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
)

var (
	// manifests
	simpleServiceManifest            = filepath.Join(fsutils.MustGetThisDir(), "testdata", "service.yaml")
	gatewayManifest                  = filepath.Join(fsutils.MustGetThisDir(), "testdata", "gateway.yaml")
	transformForHeadersManifest      = filepath.Join(fsutils.MustGetThisDir(), "testdata", "transform-for-headers.yaml")
	transformForBodyManifest         = filepath.Join(fsutils.MustGetThisDir(), "testdata", "transform-for-body.yaml")
	gatewayAttachedTransformManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "gateway-attached-transform.yaml")

	// objects from gateway manifest
	gateway = &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gw",
			Namespace: "default",
		},
	}

	// objects created by deployer after applying gateway manifest
	proxyObjectMeta = metav1.ObjectMeta{
		Name:      "gw",
		Namespace: "default",
	}

	// timeouts
	timeout = 1 * time.Minute
)

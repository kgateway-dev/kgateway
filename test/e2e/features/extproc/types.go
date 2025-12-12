//go:build e2e

package extproc

import (
	"path/filepath"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
)

var (
	// manifests
	setupManifest              = filepath.Join(fsutils.MustGetThisDir(), "testdata", "setup.yaml")
	gatewayTargetRefManifest   = filepath.Join(fsutils.MustGetThisDir(), "testdata", "gateway-targetref.yaml")
	httpRouteTargetRefManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "httproute-targetref.yaml")
	singleRouteManifest        = filepath.Join(fsutils.MustGetThisDir(), "testdata", "single-route.yaml")
	backendFilterManifest      = filepath.Join(fsutils.MustGetThisDir(), "testdata", "backend-filter.yaml")

	// Core infrastructure objects that we need to track
	gatewayObjectMeta = metav1.ObjectMeta{
		Name:      "gw",
		Namespace: "default",
	}
	gatewayService = &corev1.Service{ObjectMeta: gatewayObjectMeta}
)

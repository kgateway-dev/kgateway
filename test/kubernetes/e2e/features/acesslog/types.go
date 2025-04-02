package acesslog

import (
	"path/filepath"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
)

var (
	// manifests
	setupManifest    = filepath.Join(fsutils.MustGetThisDir(), "testdata", "setup.yaml")
	fileSinkManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "filesink.yaml")

	// Core infrastructure objects that we need to track
	gatewayObjectMeta = metav1.ObjectMeta{
		Name:      "gw",
		Namespace: "default",
	}
	gatewayService    = &corev1.Service{ObjectMeta: gatewayObjectMeta}
	gatewayDeployment = &appsv1.Deployment{ObjectMeta: gatewayObjectMeta}

	httpbinObjectMeta = metav1.ObjectMeta{
		Name:      "httpbin",
		Namespace: "httpbin",
	}
	httpbinDeployment = &appsv1.Deployment{ObjectMeta: httpbinObjectMeta}
)

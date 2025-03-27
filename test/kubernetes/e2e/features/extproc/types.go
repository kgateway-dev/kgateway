package extproc

import (
	"path/filepath"

	appsv1 "k8s.io/api/apps/v1"
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
	gatewayService    = &corev1.Service{ObjectMeta: gatewayObjectMeta}
	gatewayDeployment = &appsv1.Deployment{ObjectMeta: gatewayObjectMeta}

	// ExtProc service and deployment
	extProcService = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ext-proc-grpc",
			Namespace: "default",
		},
	}
	extProcDeployment = &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ext-proc-grpc",
			Namespace: "default",
		},
	}

	// Backend service and deployment
	backendService = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "backend",
			Namespace: "default",
		},
	}
	backendDeployment = &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "backend-0",
			Namespace: "default",
		},
	}
)

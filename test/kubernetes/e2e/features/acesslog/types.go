package acesslog

import (
	"path/filepath"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
)

var (
	// manifests
	setupManifest       = filepath.Join(fsutils.MustGetThisDir(), "testdata", "setup.yaml")
	fileSinkManifest    = filepath.Join(fsutils.MustGetThisDir(), "testdata", "filesink.yaml")
	grpcServiceManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "grpc.yaml")
	// Core infrastructure objects that we need to track
	gatewayObjectMeta = metav1.ObjectMeta{
		Name:      "gw",
		Namespace: "default",
	}
	gatewayService    = &corev1.Service{ObjectMeta: gatewayObjectMeta}
	gatewayDeployment = &appsv1.Deployment{ObjectMeta: gatewayObjectMeta}

	accessLoggerObjectMeta = metav1.ObjectMeta{
		Name:      "gateway-proxy-access-logger",
		Namespace: "default",
	}
	accessLoggerDeployment = &appsv1.Deployment{ObjectMeta: accessLoggerObjectMeta}
	accessLoggerService    = &corev1.Service{ObjectMeta: accessLoggerObjectMeta}

	httpbinObjectMeta = metav1.ObjectMeta{
		Name:      "httpbin",
		Namespace: "httpbin",
	}
	httpbinDeployment = &appsv1.Deployment{ObjectMeta: httpbinObjectMeta}

	// HTTPListenerPolicy objects
	fileSinkConfig = &v1alpha1.HTTPListenerPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "access-logs",
			Namespace: "default",
		},
	}
)

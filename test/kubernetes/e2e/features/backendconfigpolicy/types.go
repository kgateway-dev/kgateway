package backendconfigpolicy

import (
	"path/filepath"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
)

var (
	// manifests
	setupManifest            = filepath.Join(fsutils.MustGetThisDir(), "testdata", "setup.yaml")
	nginxManifest            = filepath.Join(fsutils.MustGetThisDir(), "testdata", "nginx.yaml")
	tlsInsecureManifest      = filepath.Join(fsutils.MustGetThisDir(), "testdata", "tls-insecure.yaml")
	simpleTLSManifest        = filepath.Join(fsutils.MustGetThisDir(), "testdata", "simple-tls.yaml")
	outlierDetectionManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "outlierdetection.yaml")
	// objects
	proxyObjectMeta = metav1.ObjectMeta{
		Name:      "gw",
		Namespace: "default",
	}
	proxyDeployment     = &appsv1.Deployment{ObjectMeta: proxyObjectMeta}
	proxyService        = &corev1.Service{ObjectMeta: proxyObjectMeta}
	proxyServiceAccount = &corev1.ServiceAccount{ObjectMeta: proxyObjectMeta}

	exampleSvc = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-svc",
			Namespace: "default",
		},
	}
	nginxPod = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx",
			Namespace: "default",
		},
	}
	httpbinMeta = metav1.ObjectMeta{
		Name:      "httpbin",
		Namespace: "default",
	}
	httpbinDeployment = &appsv1.Deployment{ObjectMeta: httpbinMeta}
)

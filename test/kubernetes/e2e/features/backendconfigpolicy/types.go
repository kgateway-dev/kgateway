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
	systemCAManifest         = filepath.Join(fsutils.MustGetThisDir(), "testdata", "system-ca.yaml")
	outlierDetectionManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "outlierdetection.yaml")
	// objects
	proxyObjectMeta = metav1.ObjectMeta{
		Name:      "gw",
		Namespace: "default",
	}
	proxyService = &corev1.Service{ObjectMeta: proxyObjectMeta}

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

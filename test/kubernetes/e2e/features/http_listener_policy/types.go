package http_listener_policy

import (
	"path/filepath"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
)

var (
	setupManifest                   = filepath.Join(fsutils.MustGetThisDir(), "testdata", "setup.yaml")
	gatewayManifest                 = filepath.Join(fsutils.MustGetThisDir(), "testdata", "gateway.yaml")
	httpRouteManifest               = filepath.Join(fsutils.MustGetThisDir(), "testdata", "httproute.yaml")
	allFieldsManifest               = filepath.Join(fsutils.MustGetThisDir(), "testdata", "http-listener-policy-all-fields.yaml")
	serverHeaderManifest            = filepath.Join(fsutils.MustGetThisDir(), "testdata", "http-listener-policy-server-header.yaml")
	preserveHttp1HeaderCaseManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "preserve-http1-header-case.yaml")
	accessLogManifest               = filepath.Join(fsutils.MustGetThisDir(), "testdata", "http-listener-policy-access-log.yaml")

	// When we apply the setup file, we expect resources to be created with this metadata
	proxyObjectMeta = metav1.ObjectMeta{
		Name:      "gw",
		Namespace: "default",
	}
	proxyService    = &corev1.Service{ObjectMeta: proxyObjectMeta}
	proxyDeployment = &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gw",
			Namespace: "default",
		},
	}
	nginxPod = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx",
			Namespace: "default",
		},
	}
	exampleSvc = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-svc",
			Namespace: "default",
		},
	}
	echoService = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "raw-header-echo",
			Namespace: "default",
		},
	}
	echoDeployment = &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "raw-header-echo",
			Namespace: "default",
		},
	}
)

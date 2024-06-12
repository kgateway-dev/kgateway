package http_listener_options

import (
	"path/filepath"

	"github.com/solo-io/skv2/codegen/util"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	setupManifest             = filepath.Join(util.MustGetThisDir(), "testdata", "setup.yaml")
	gatewayManifest           = filepath.Join(util.MustGetThisDir(), "testdata", "gateway.yaml")
	basicLisOptManifest       = filepath.Join(util.MustGetThisDir(), "testdata", "basic-http-lis-opt.yaml")
	notAttachedLisOptManifest = filepath.Join(util.MustGetThisDir(), "testdata", "not-attached-http-lis-opt.yaml")

	// When we apply the setup file, we expect resources to be created with this metadata
	glooProxyObjectMeta = metav1.ObjectMeta{
		Name:      "gloo-proxy-gw",
		Namespace: "default",
	}
	proxyService    = &corev1.Service{ObjectMeta: glooProxyObjectMeta}
	proxyDeployment = &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gloo-proxy-gw",
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
)

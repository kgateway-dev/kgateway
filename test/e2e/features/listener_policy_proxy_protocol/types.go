//go:build e2e

package listener_policy_proxy_protocol

import (
	"path/filepath"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
)

var (
	setupManifest          = filepath.Join(fsutils.MustGetThisDir(), "testdata", "setup.yaml")
	gatewayManifest        = filepath.Join(fsutils.MustGetThisDir(), "testdata", "gateway.yaml")
	httpRouteManifest      = filepath.Join(fsutils.MustGetThisDir(), "testdata", "httproute.yaml")
	listenerPolicyManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "listener-policy-proxy-protocol.yaml")

	proxyObjectMeta = metav1.ObjectMeta{
		Name:      "gw",
		Namespace: "default",
	}
	proxyService    = &corev1.Service{ObjectMeta: proxyObjectMeta}
	proxyDeployment = &appsv1.Deployment{ObjectMeta: proxyObjectMeta}

	curlPod = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "curl",
			Namespace: "curl",
		},
	}
	exampleSvc = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-svc",
			Namespace: "default",
		},
	}
)

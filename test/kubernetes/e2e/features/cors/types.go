package cors

import (
	"path/filepath"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
)

var (
	// manifests
	simpleServiceManifest  = filepath.Join(fsutils.MustGetThisDir(), "testdata", "service.yaml")
	commonManifest         = filepath.Join(fsutils.MustGetThisDir(), "testdata", "common.yaml")
	httpRoutesManifest     = filepath.Join(fsutils.MustGetThisDir(), "testdata", "httproutes.yaml")
	corsHttpRoutesManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "httproutes-cors.yaml")

	// traffic policies with cors configuration
	gwCorsTrafficPolicyManifest    = filepath.Join(fsutils.MustGetThisDir(), "testdata", "tp-gw-cors.yaml")
	routeCorsTrafficPolicyManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "tp-route-cors.yaml")

	// objects from gateway manifest
	gateway = &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gw",
			Namespace: "default",
		},
	}
	route = &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "svc-route",
			Namespace: "default",
		},
	}
	route2 = &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "svc-route-2",
			Namespace: "default",
		},
	}
	// objects created by deployer after applying gateway manifest
	proxyObjectMeta = metav1.ObjectMeta{
		Name:      "gw",
		Namespace: "default",
	}
	proxyDeployment     = &appsv1.Deployment{ObjectMeta: proxyObjectMeta}
	proxyService        = &corev1.Service{ObjectMeta: proxyObjectMeta}
	proxyServiceAccount = &corev1.ServiceAccount{ObjectMeta: proxyObjectMeta}

	// objects from service manifest
	simpleSvc = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple-svc",
			Namespace: "default",
		},
	}
	simpleDeployment = &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "backend-0",
			Namespace: "default",
		},
	}

	gwCorsTrafficPolicy = &v1alpha1.TrafficPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gw-cors-policy",
			Namespace: "default",
		},
	}
	routeCorsTrafficPolicy = &v1alpha1.TrafficPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "route-cors-policy",
			Namespace: "default",
		},
	}
)

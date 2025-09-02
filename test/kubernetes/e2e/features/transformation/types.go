package transformation

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
	simpleServiceManifest            = filepath.Join(fsutils.MustGetThisDir(), "testdata", "service.yaml")
	gatewayManifest                  = filepath.Join(fsutils.MustGetThisDir(), "testdata", "gateway.yaml")
	transformForHeadersManifest      = filepath.Join(fsutils.MustGetThisDir(), "testdata", "transform-for-headers.yaml")
	transformForBodyJsonManifest     = filepath.Join(fsutils.MustGetThisDir(), "testdata", "transform-for-body-json.yaml")
	transformForBodyAsStringManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "transform-for-body-as-string.yaml")
	gatewayAttachedTransformManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "gateway-attached-transform.yaml")

	// objects from gateway manifest
	gateway = &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gw",
			Namespace: "default",
		},
	}
	gwp = &v1alpha1.GatewayParameters{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gw-params",
			Namespace: "default",
		},
	}

	routeForName = func(name string) *gwv1.HTTPRoute {
		return &gwv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: "default",
			},
		}
	}
	policyForName = func(name string) *v1alpha1.TrafficPolicy {
		return &v1alpha1.TrafficPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: "default",
			},
		}
	}

	routeBasic           = routeForName("example-route")
	routeForHeaders      = routeForName("example-route-for-headers")
	routeForBodyJson     = routeForName("example-route-for-body-json")
	routeForBodyAsString = routeForName("example-route-for-body-as-string")

	trafficPolicyForHeaders                  = policyForName("example-traffic-policy-for-headers")
	trafficPolicyForBodyJson                 = policyForName("example-traffic-policy-for-body-json")
	trafficPolicyForBodyAsString             = policyForName("example-traffic-policy-for-body-as-string")
	trafficPolicyForGatewayAttachedTransform = policyForName("example-traffic-policy-for-gateway-attached-transform")

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
)

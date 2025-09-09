package deployer

import (
	"path/filepath"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
)

var (
	// manifests
	gatewayWithoutParameters = filepath.Join(fsutils.MustGetThisDir(), "testdata", "gateway-without-parameters.yaml")
	gatewayWithParameters    = filepath.Join(fsutils.MustGetThisDir(), "testdata", "gateway-with-parameters.yaml")
	gatewayParametersCustom  = filepath.Join(fsutils.MustGetThisDir(), "testdata", "gatewayparameters-custom.yaml")
	// TODO add back when we re-enable istio suite
	//istioGatewayParameters   = filepath.Join(fsutils.MustGetThisDir(), "testdata", "istio-gateway-parameters.yaml")
	selfManagedGateway = filepath.Join(fsutils.MustGetThisDir(), "testdata", "self-managed-gateway.yaml")

	// objects
	proxyObjectMeta = metav1.ObjectMeta{
		Name:      "gw",
		Namespace: "default",
	}
	proxyDeployment     = &appsv1.Deployment{ObjectMeta: proxyObjectMeta}
	proxyService        = &corev1.Service{ObjectMeta: proxyObjectMeta}
	proxyServiceAccount = &corev1.ServiceAccount{ObjectMeta: proxyObjectMeta}

	gwParamsDefault = &v1alpha1.GatewayParameters{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gw-params",
			Namespace: "default",
		},
	}

	gwParamsCustom = &v1alpha1.GatewayParameters{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gw-params-custom",
			Namespace: "default",
		},
	}

	gw = &gwapiv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gw",
			Namespace: "default",
		},
	}
	route = &gwapiv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-route",
			Namespace: "default",
		},
	}
)

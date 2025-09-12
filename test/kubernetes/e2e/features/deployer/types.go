package deployer

import (
	"path/filepath"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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

	gwParamsDefaultObjectMeta = metav1.ObjectMeta{
		Name:      "gw-params",
		Namespace: "default",
	}

	gwParamsCustomObjectMeta = metav1.ObjectMeta{
		Name:      "gw-params-custom",
		Namespace: "default",
	}
)

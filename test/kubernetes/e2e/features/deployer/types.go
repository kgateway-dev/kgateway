package deployer

import (
	"path/filepath"

	"github.com/solo-io/skv2/codegen/util"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/solo-io/gloo/projects/gateway2/pkg/api/gateway.gloo.solo.io/v1alpha1"
)

var (
	gwParametersManifestFile      = filepath.Join(util.MustGetThisDir(), "gateway-parameters.yaml")
	deployerProvisionManifestFile = filepath.Join(util.MustGetThisDir(), "deployer-provision.yaml")

	// When we apply the deployer-provision.yaml file, we expect resources to be created with this metadata
	glooProxyObjectMeta = metav1.ObjectMeta{
		Name:      "gloo-proxy-gw",
		Namespace: "default",
	}
	proxyDeployment = &appsv1.Deployment{ObjectMeta: glooProxyObjectMeta}
	proxyService    = &corev1.Service{ObjectMeta: glooProxyObjectMeta}

	gwParams = &v1alpha1.GatewayParameters{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gw-params",
			Namespace: "default",
		},
	}
)

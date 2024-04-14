package deployer

import (
	"context"

	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/testutils/operations"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	manifestFile = e2e.FeatureManifestFile("deployer-provision.yaml")

	// When we apply the deployer-provision.yaml file, we expect resources to be created with this metadata
	glooProxyObjectMeta = metav1.ObjectMeta{
		Name:      "gloo-proxy-gw",
		Namespace: "default",
	}
	proxyDeployment = &appsv1.Deployment{ObjectMeta: glooProxyObjectMeta}
	proxyService    = &corev1.Service{ObjectMeta: glooProxyObjectMeta}
)

var ProvisionDeploymentAndService = e2e.Test{
	Name:        "Deployer.ProvisionDeploymentAndService",
	Description: "the deployer will provision a deployment and service for a defined gateway",

	Test: func(ctx context.Context, installation *e2e.TestInstallation) {
		provisionResourcesOp := operations.ReversibleOperation{
			Do: installation.Operations.KubeCtl().NewApplyManifestOperation(
				manifestFile,
				installation.Assertions.ObjectsExist(proxyService, proxyDeployment),
			),
			Undo: installation.Operations.KubeCtl().NewDeleteManifestOperation(
				manifestFile,
				installation.Assertions.ObjectsNotExist(proxyService, proxyDeployment),
			),
		}

		err := installation.Operator.ExecuteReversibleOperations(ctx, provisionResourcesOp)
		Expect(err).NotTo(HaveOccurred())
	},
}

// TODO
// Add Test cases for routing traffic

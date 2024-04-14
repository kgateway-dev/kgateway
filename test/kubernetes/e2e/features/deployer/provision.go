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
			Do: installation.OperationsProvider.KubeCtl().NewApplyManifestOperation(
				manifestFile,
				installation.AssertionsProvider.ObjectsExist(proxyService, proxyDeployment),
			),
			Undo: installation.OperationsProvider.KubeCtl().NewDeleteManifestOperation(
				manifestFile,
				installation.AssertionsProvider.ObjectsNotExist(proxyService, proxyDeployment),
			),
		}

		err := installation.Operator.ExecuteReversibleOperations(ctx, provisionResourcesOp)
		Expect(err).NotTo(HaveOccurred())
	},
}

var RouteIngressTraffic = e2e.Test{
	Name:        "Deployer.RouteIngressTraffic",
	Description: "traffic can be routed through services provisioned by the gateway",
	Test: func(ctx context.Context, installation *e2e.TestInstallation) {
		createResourcesOp := installation.OperationsProvider.KubeCtl().NewApplyManifestOperation(
			manifestFile,
			installation.AssertionsProvider.ObjectsExist(proxyService, proxyDeployment),
			installation.AssertionsProvider.RunningReplicas(glooProxyObjectMeta, 1),
		)
		deleteResourcesOp := installation.OperationsProvider.KubeCtl().NewDeleteManifestOperation(
			manifestFile,
			installation.AssertionsProvider.ObjectsNotExist(proxyService, proxyDeployment),
		)

		err := installation.Operator.ExecuteReversibleOperations(ctx, operations.ReversibleOperation{
			Do:   createResourcesOp,
			Undo: deleteResourcesOp,
		})
		Expect(err).To(HaveOccurred(), "The gloo-proxy-gw that is deployed has the tag 1.0.0-ci which will fail to start")
	},
}

// TODO
// Other use cases:
// create resources
// check they are there
// check behavior for rate limiting
// 1 operation

// then i want to change the defintion
// assert new behavior
// 2 operation

// operation.ExecuteOperation(1,2)
// do-1, do-2, undo-2, undo-1

// TODOs:
// 1. PR that intorduces the framework with
// include more complex tests to demonstrate the framework
// get PR reviewed for framework
// 1 suite completely re-written, as a separate PR off this
// get PR for framework merged
// get suite re-write PR merged
//

//
// create an httproute and httpgateway
// send traffic

// then you create a gw parameters
// expect resources to change
// test traffic

// cleanup both
// side-effect of creating the gw, the deployer creates some resources on your behalf,
// those should get deleted?
//

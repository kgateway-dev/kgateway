package deployer

import (
	"context"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/utils/envoyutils/admincli"
	"github.com/solo-io/gloo/pkg/utils/kubeutils/portforward"
	"github.com/solo-io/gloo/pkg/utils/requestutils/curl"
	"github.com/solo-io/gloo/projects/gateway2/pkg/api/gateway.gloo.solo.io/v1alpha1"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/testutils/operations"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"time"
)

var (
	gwParametersManifestFile = e2e.FeatureManifestFile("gateway-parameters.yaml")

	gwParams = &v1alpha1.GatewayParameters{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gw-params",
			Namespace: "default",
		},
	}
)

var ConfigureProxiesFromGatewayParameters = e2e.Test{
	Name:        "Deployer.ConfigureProxiesFromGatewayParameters",
	Description: "the deployer will provision a deployment and service for a defined gateway, and configure it based on the GatewayParameters CR",

	Test: func(ctx context.Context, installation *e2e.TestInstallation) {
		provisionResourcesOp := operations.ReversibleOperation{
			Do: installation.OperationsProvider.KubeCtl().NewApplyManifestOperation(
				manifestFile,
				installation.AssertionsProvider.ObjectsExist(proxyService, proxyDeployment),
			),
			// We rely on the --ignore-not-found flag in the deletion command, because we have 2 manifests
			// that manage the same resource (manifestFile, gwParametersManifestFile).
			// So when we perform Undo of configureGatewayParametersOp, it will delete the Gateway CR,
			// and then this operation  will also attempt to delete the same resource.
			// Ideally, we do not include the same resource in multiple manifests that are used by a test
			// But this is an example of ways to solve that problem if it occurs
			Undo: installation.OperationsProvider.KubeCtl().NewDeleteManifestIgnoreNotFoundOperation(
				manifestFile,
				installation.AssertionsProvider.ObjectsNotExist(proxyService, proxyDeployment),
			),
		}

		configureGatewayParametersOp := operations.ReversibleOperation{
			Do: installation.OperationsProvider.KubeCtl().NewApplyManifestOperation(
				gwParametersManifestFile,

				// We applied a manifest containing the GatewayParameters CR
				installation.AssertionsProvider.ObjectsExist(gwParams),

				// We configure the GatewayParameters CR to provision workloads with a specific image that should exist
				installation.AssertionsProvider.RunningReplicas(proxyDeployment.ObjectMeta, 1),

				// This is an example of a custom assertion
				// It's the type of assertion that likely warrants being a re-usable utility
				func(ctx context.Context) {
					portForwarder, err := installation.OperationsProvider.KubeCtl().Client().StartPortForward(ctx,
						portforward.WithDeployment(proxyDeployment.GetName(), proxyDeployment.GetNamespace()),
						portforward.WithPorts(admincli.DefaultAdminPort, admincli.DefaultAdminPort),
					)
					Expect(err).NotTo(HaveOccurred())

					portForwarder.Address()
					defer func() {
						portForwarder.Close()
						portForwarder.WaitForStop()
					}()

					adminClient := admincli.NewClient().WithCurlOptions(curl.WithPort(admincli.DefaultAdminPort))

					Eventually(func(g Gomega) {
						serverInfo, err := adminClient.GetServerInfo(ctx)
						g.Expect(err).NotTo(HaveOccurred())
						g.Expect(serverInfo.GetCommandLineOptions().GetLogLevel()).To(Equal("debug"), "defined on the GatewayParameters CR")
					}).
						WithContext(ctx).
						WithTimeout(time.Second * 10).
						WithPolling(time.Millisecond * 200).
						Should(Succeed())
				},
			),
			Undo: installation.OperationsProvider.KubeCtl().NewDeleteManifestOperation(
				gwParametersManifestFile,
				installation.AssertionsProvider.ObjectsNotExist(gwParams),
			),
		}

		err := installation.Operator.ExecuteReversibleOperations(ctx, provisionResourcesOp, configureGatewayParametersOp)
		Expect(err).NotTo(HaveOccurred())
	},
}

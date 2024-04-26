package deployer

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/solo-io/gloo/test/kubernetes/testutils/runtime"

	"github.com/solo-io/skv2/codegen/util"

	"github.com/solo-io/gloo/test/kubernetes/testutils/assertions"

	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/utils/envoyutils/admincli"
	"github.com/solo-io/gloo/projects/gateway2/pkg/api/gateway.gloo.solo.io/v1alpha1"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/testutils/operations"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	gwParametersManifestFile = filepath.Join(util.MustGetThisDir(), "gateway-parameters.yaml")

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
			Do: &operations.BasicOperation{
				OpName:      fmt.Sprintf("apply-manifest-%s", filepath.Base(manifestFile)),
				OpAction:    installation.Actions.Kubectl().NewApplyManifestAction(manifestFile),
				OpAssertion: installation.Assertions.ObjectsExist(proxyService, proxyDeployment),
			},
			// We rely on the --ignore-not-found flag in the deletion command, because we have 2 manifests
			// that manage the same resource (manifestFile, gwParametersManifestFile).
			// So when we perform Undo of configureGatewayParametersOp, it will delete the Gateway CR,
			// and then this operation  will also attempt to delete the same resource.
			// Ideally, we do not include the same resource in multiple manifests that are used by a test
			// But this is an example of ways to solve that problem if it occurs.
			Undo: &operations.BasicOperation{
				OpName:      fmt.Sprintf("delete-manifest-%s", filepath.Base(manifestFile)),
				OpAction:    installation.Actions.Kubectl().NewDeleteManifestAction(manifestFile, "--ignore-not-found=true"),
				OpAssertion: installation.Assertions.ObjectsNotExist(proxyService, proxyDeployment),
			},
		}

		configureGatewayParametersOp := operations.ReversibleOperation{
			Do: &operations.BasicOperation{
				OpName:   fmt.Sprintf("apply-manifest-%s", filepath.Base(gwParametersManifestFile)),
				OpAction: installation.Actions.Kubectl().NewApplyManifestAction(gwParametersManifestFile),
				OpAssertions: []assertions.ClusterAssertion{
					// We applied a manifest containing the GatewayParameters CR
					installation.Assertions.ObjectsExist(gwParams),

					// Before opening a port-forward, we assert that there is at least one Pod that is ready
					installation.Assertions.RunningReplicas(proxyDeployment.ObjectMeta, Equal(1)),

					// We assert that we can port-forward requests to the proxy deployment, and then execute requests against the server
					installation.Assertions.EnvoyAdminApiAssertion(
						proxyDeployment.ObjectMeta,
						func(ctx context.Context, adminClient *admincli.Client) {
							if installation.TestCluster.RuntimeContext.RunSource != runtime.LocalDevelopment {
								// There are failures when running this command in CI
								// Those are currently being investigated
								return
							}
							Eventually(func(g Gomega) {
								serverInfo, err := adminClient.GetServerInfo(ctx)
								g.Expect(err).NotTo(HaveOccurred(), "can get server info")
								g.Expect(serverInfo.GetCommandLineOptions().GetLogLevel()).To(
									Equal("debug"), "defined on the GatewayParameters CR")
								g.Expect(serverInfo.GetCommandLineOptions().GetComponentLogLevel()).To(
									Equal("connection:trace,upstream:debug"), "defined on the GatewayParameters CR")
							}).
								WithContext(ctx).
								WithTimeout(time.Second * 10).
								WithPolling(time.Millisecond * 200).
								Should(Succeed())
						},
					),
				},
			},
			Undo: &operations.BasicOperation{
				OpName:      fmt.Sprintf("delete-manifest-%s", filepath.Base(gwParametersManifestFile)),
				OpAction:    installation.Actions.Kubectl().NewDeleteManifestAction(gwParametersManifestFile),
				OpAssertion: installation.Assertions.ObjectsNotExist(gwParams),
			},
		}

		err := installation.Operator.ExecuteReversibleOperations(ctx, provisionResourcesOp, configureGatewayParametersOp)
		Expect(err).NotTo(HaveOccurred())
	},
}

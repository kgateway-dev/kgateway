package route_options

import (
	"context"
	"fmt"
	"path/filepath"

	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/utils/kubeutils/kubectl"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/testutils/assertions"
	"github.com/solo-io/gloo/test/kubernetes/testutils/operations"
	"github.com/solo-io/go-utils/threadsafe"
	"github.com/solo-io/skv2/codegen/util"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	targetRefManifest      = filepath.Join(util.MustGetThisDir(), "inputs/fault-injection-targetref.yaml")
	filterExtensioManifest = filepath.Join(util.MustGetThisDir(), "inputs/fault-injection-filter-extension.yaml")

	// When we apply the deployer-provision.yaml file, we expect resources to be created with this metadata
	glooProxyObjectMeta = metav1.ObjectMeta{
		Name:      "gloo-proxy-gw",
		Namespace: "default",
	}
	proxyDeployment = &appsv1.Deployment{ObjectMeta: glooProxyObjectMeta}
	proxyService    = &corev1.Service{ObjectMeta: glooProxyObjectMeta}

	curlDeployment = &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "curl",
			Namespace: "curl",
		},
	}

	// RouteOption resource to be created
	routeOptionMeta = metav1.ObjectMeta{
		Name:      "teapot-fault-injection",
		Namespace: "default",
	}
)

var ConfigureRouteOptionsWithTargetRef = e2e.Test{
	Name:        "RouteOptions.ConfigureRouteOptionsWithTargetRef",
	Description: "the RouteOptions will configure fault inject with a targetRef",
	Test: func(ctx context.Context, installation *e2e.TestInstallation) {
		trafficRefRoutingOp := operations.ReversibleOperation{
			Do: &operations.BasicOperation{
				OpName:   fmt.Sprintf("apply-manifest-%s", filepath.Base(targetRefManifest)),
				OpAction: installation.Actions.Kubectl().NewApplyManifestAction(targetRefManifest),
				OpAssertion: func(ctx context.Context) {
					// First check resources are created for Gateay
					installation.Assertions.ObjectsExist(proxyService, proxyDeployment)

					// Check fault injection is applied
					CheckFaultInjectionFromCluster()(ctx)

					// Check status on solo-apis client object
					Eventually(func(g Gomega) {
						routeOption, err := installation.RouteOptionClient.Read(routeOptionMeta.GetNamespace(), routeOptionMeta.GetName(), clients.ReadOpts{})
						g.Expect(err).NotTo(HaveOccurred())
						g.Expect(routeOption.GetNamespacedStatuses()).ToNot(BeNil())
						g.Expect(routeOption.GetNamespacedStatuses().GetStatuses()).ToNot(BeEmpty())
						g.Expect(routeOption.GetNamespacedStatuses().GetStatuses()[installation.Namespace].GetReportedBy()).To(Equal("gloo-kube-gateway"))
						g.Expect(routeOption.GetNamespacedStatuses().GetStatuses()[installation.Namespace].GetState()).To(Equal(core.Status_Accepted))
					}, "5s", ".1s").Should(Succeed())
				},
			},
			Undo: &operations.BasicOperation{
				OpName:   fmt.Sprintf("delete-manifest-%s", filepath.Base(targetRefManifest)),
				OpAction: installation.Actions.Kubectl().NewDeleteManifestAction(targetRefManifest),
				OpAssertion: func(ctx context.Context) {
					// Check resources are deleted for Gateway
					installation.Assertions.ObjectsNotExist(proxyService, proxyDeployment)
				},
			},
		}

		err := installation.Operator.ExecuteReversibleOperations(ctx, trafficRefRoutingOp)
		Expect(err).NotTo(HaveOccurred())
	},
}

var ConfigureRouteOptionsWithFilterExtenstion = e2e.Test{
	Name:        "RouteOptions.ConfigureRouteOptionsWithFilterExtenstion",
	Description: "the RouteOptions will configure fault inject with a filter extension",
	Test: func(ctx context.Context, installation *e2e.TestInstallation) {
		extensionFilterRoutingOp := operations.ReversibleOperation{
			Do: &operations.BasicOperation{
				OpName:   fmt.Sprintf("apply-manifest-%s", filepath.Base(filterExtensioManifest)),
				OpAction: installation.Actions.Kubectl().NewApplyManifestAction(filterExtensioManifest),
				OpAssertion: func(ctx context.Context) {
					// First check resources are created for Gateay
					installation.Assertions.ObjectsExist(proxyService, proxyDeployment)

					// Check fault injection is applied
					CheckFaultInjectionFromCluster()(ctx)

					// TODO(npolshak): Statuses are not supported for filter extensions yet
				},
			},
			Undo: &operations.BasicOperation{
				OpName:   fmt.Sprintf("delete-manifest-%s", filepath.Base(filterExtensioManifest)),
				OpAction: installation.Actions.Kubectl().NewDeleteManifestAction(filterExtensioManifest),
				OpAssertion: func(ctx context.Context) {
					// Check resources are deleted for Gateway
					installation.Assertions.ObjectsNotExist(proxyService, proxyDeployment)
				},
			},
		}

		err := installation.Operator.ExecuteReversibleOperations(ctx, extensionFilterRoutingOp)
		Expect(err).NotTo(HaveOccurred())
	},
}

func CheckFaultInjectionFromCluster() assertions.ClusterAssertion {
	return func(ctx context.Context) {
		fdqnAddr := fmt.Sprintf("http://%s.%s.svc.cluster.local:8080", proxyDeployment.GetName(), proxyDeployment.GetNamespace())

		Eventually(func(g Gomega) {
			// curl gloo-proxy-gw.default.svc.cluster.local:8080 -H "Host: example.com" -v
			resp, err := curl(ctx, curlDeployment.GetNamespace(), curlDeployment.GetName(), "curl", fdqnAddr, "example.com")
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(resp).To(ContainSubstring("fault filter abort"))
			g.Expect(resp).To(ContainSubstring("HTTP/1.1 418"))
		}, "10s", "1s", "curl should eventually return fault injection response").Should(Succeed())
	}
}

func curl(ctx context.Context, ns, fromDeployment, fromContainer, fdqnAddr, host string) (string, error) {
	var buf threadsafe.Buffer
	kubeCli := kubectl.NewCli().WithReceiver(&buf)

	args := []string{
		"exec",
		"-n", ns,
		fmt.Sprintf("deployment/%s", fromDeployment),
		"-c", fromContainer,
		"--", "curl", fdqnAddr,
		"-H", fmt.Sprintf("Host: %s", host),
		"-v",
	}
	err := kubeCli.RunCommand(ctx, args...)
	return buf.String(), err
}

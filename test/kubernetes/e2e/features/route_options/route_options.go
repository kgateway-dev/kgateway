package route_options

import (
	"context"
	"fmt"
	"path/filepath"

	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/utils/kubeutils/kubectl"
	"github.com/solo-io/gloo/pkg/utils/requestutils/curl"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/testutils/assertions"
	"github.com/solo-io/gloo/test/kubernetes/testutils/operations"
	"github.com/solo-io/go-utils/threadsafe"
	"github.com/solo-io/skv2/codegen/util"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
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

	curlPod = &corev1.Pod{
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
				OpAssertions: []assertions.ClusterAssertion{
					// First check resources are created for Gateay
					installation.Assertions.ObjectsExist(proxyService, proxyDeployment),

					// Check fault injection is applied
					checkFaultInjectionFromCluster(),

					// Check status on solo-apis client object
					helpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
						return installation.ResourceClients.RouteOptionClient().Read(routeOptionMeta.GetNamespace(), routeOptionMeta.GetName(), clients.ReadOpts{})
					}, 5, 1)

					// Check correct gateway reports status of route option
					Eventually(func(g Gomega) {
						routeOption, err := installation.ResourceClients.RouteOptionClient().Read(routeOptionMeta.GetNamespace(), routeOptionMeta.GetName(), clients.ReadOpts{})
						g.Expect(err).NotTo(HaveOccurred())
						g.Expect(routeOption.GetNamespacedStatuses()).ToNot(BeNil())
						g.Expect(routeOption.GetNamespacedStatuses().GetStatuses()).ToNot(BeEmpty())
						g.Expect(routeOption.GetNamespacedStatuses().GetStatuses()[installation.Metadata.InstallNamespace].GetReportedBy()).To(Equal("gloo-kube-gateway"))
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
	Name:        "RouteOptions.ConfigureRouteOptionsWithFilterExtension",
	Description: "the RouteOptions will configure fault inject with a filter extension",
	Test: func(ctx context.Context, installation *e2e.TestInstallation) {
		extensionFilterRoutingOp := operations.ReversibleOperation{
			Do: &operations.BasicOperation{
				OpName:   fmt.Sprintf("apply-manifest-%s", filepath.Base(filterExtensioManifest)),
				OpAction: installation.Actions.Kubectl().NewApplyManifestAction(filterExtensioManifest),
				OpAssertions: []assertions.ClusterAssertion{
					// First check resources are created for Gateay
					installation.Assertions.ObjectsExist(proxyService, proxyDeployment),

					// Check fault injection is applied
					checkFaultInjectionFromCluster(),

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

func checkFaultInjectionFromCluster() assertions.ClusterAssertion {
	return func(ctx context.Context) {
		fdqnAddr := fmt.Sprintf("%s.%s.svc.cluster.local", proxyDeployment.GetName(), proxyDeployment.GetNamespace())

		Eventually(func(g Gomega) {
			// curl gloo-proxy-gw.default.svc.cluster.local:8080 -H "Host: example.com" -v
			var buf threadsafe.Buffer
			kubeCli := kubectl.NewCli().WithReceiver(&buf)
			resp := kubeCli.CurlFromEphemeralPod(ctx, curlPod.ObjectMeta,
				curl.WithHost(fdqnAddr),
				curl.WithHostHeader("example.com"),
				curl.VerboseOutput())
			g.Expect(resp).To(ContainSubstring("fault filter abort"))
			g.Expect(resp).To(ContainSubstring("HTTP/1.1 418"))
		}, "10s", "1s", "curl should eventually return fault injection response").Should(Succeed())
	}
}

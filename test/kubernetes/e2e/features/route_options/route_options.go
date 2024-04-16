package route_options

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"

	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/utils/kubeutils/kubectl"
	"github.com/solo-io/gloo/pkg/utils/kubeutils/portforward"
	"github.com/solo-io/gloo/test/gomega/matchers"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/testutils/assertions"
	"github.com/solo-io/gloo/test/kubernetes/testutils/operations"
	"github.com/solo-io/gloo/test/testutils"
	"github.com/solo-io/skv2/codegen/util"
	v1 "github.com/solo-io/solo-apis/pkg/api/gateway.solo.io/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
					FaultInjection()(ctx)

					// Check status on solo-apis client object
					var routeOption = &v1.RouteOption{}
					Eventually(func(g Gomega) {
						g.Expect(routeOption.Status.GetSubresourceStatuses()).ToNot(BeEmpty())
						g.Expect(routeOption.Status.GetSubresourceStatuses()["gloo-system"].GetReportedBy()).To(Equal("gloo-kube-gateway"))
						g.Expect(routeOption.Status.GetSubresourceStatuses()["gloo-system"].GetState()).To(Equal(v1.RouteOptionStatus_Accepted))
					}, "5s", ".1s").Should(Succeed())
					installation.ClusterContext.Client.Get(ctx, client.ObjectKey{Name: routeOptionMeta.GetName(), Namespace: routeOptionMeta.GetNamespace()}, routeOption)

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
		trafficRefRoutingOp := operations.ReversibleOperation{
			Do: &operations.BasicOperation{
				OpName:   fmt.Sprintf("apply-manifest-%s", filepath.Base(filterExtensioManifest)),
				OpAction: installation.Actions.Kubectl().NewApplyManifestAction(filterExtensioManifest),
				OpAssertion: func(ctx context.Context) {
					// First check resources are created for Gateay
					installation.Assertions.ObjectsExist(proxyService, proxyDeployment)

					// Check fault injection is applied
					FaultInjection()(ctx)

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

		err := installation.Operator.ExecuteReversibleOperations(ctx, trafficRefRoutingOp)
		Expect(err).NotTo(HaveOccurred())
	},
}

func FaultInjection() assertions.ClusterAssertion {
	return func(ctx context.Context) {
		// Check that the curl request is successful
		kubeCli := kubectl.NewCli()
		portForwarder, err := kubeCli.StartPortForward(ctx,
			portforward.WithDeployment(proxyDeployment.GetName(), proxyDeployment.GetNamespace()),
			portforward.WithRemotePort(8080),
		)
		Expect(err).NotTo(HaveOccurred())
		defer func() {
			portForwarder.Close()
			portForwarder.WaitForStop()
		}()

		curlGateway := testutils.DefaultRequestBuilder().
			WithPort(8080).
			WithPath("/").
			WithHost("example.com").
			WithHostname(portForwarder.Address()).
			Build()

		Eventually(func(g Gomega) {
			resp, err := http.DefaultClient.Do(curlGateway)
			g.Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			g.Expect(resp).Should(matchers.HaveHttpResponse(&matchers.HttpResponse{
				StatusCode: http.StatusTeapot,
			}))
		}, "5s", ".1s", "curl should eventually return faultinjection response")
	}
}

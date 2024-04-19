package route_options

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"

	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/utils/kubeutils/kubectl"
	"github.com/solo-io/gloo/pkg/utils/requestutils/curl"
	testmatchers "github.com/solo-io/gloo/test/gomega/matchers"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/testutils/assertions"
	"github.com/solo-io/gloo/test/kubernetes/testutils/operations"
	"github.com/solo-io/go-utils/threadsafe"
	"github.com/solo-io/skv2/codegen/util"
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

	curlFromPod = func(ctx context.Context) func() string {
		proxyFdqnAddr := fmt.Sprintf("%s.%s.svc.cluster.local", proxyDeployment.GetName(), proxyDeployment.GetNamespace())
		curlOpts := []curl.Option{
			curl.WithHost(proxyFdqnAddr),
			curl.WithHostHeader("example.com"),
		}

		return func() string {
			var buf threadsafe.Buffer
			kubeCli := kubectl.NewCli().WithReceiver(&buf)
			return kubeCli.CurlFromEphemeralPod(ctx, curlPod.ObjectMeta, curlOpts...)
		}
	}

	expectedFaultInjectionResp = &testmatchers.HttpResponse{
		StatusCode: http.StatusTeapot,
		Body:       ContainSubstring("fault filter abort"),
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
					assertions.CurlEventuallyRespondsAssertion(curlFromPod(ctx), expectedFaultInjectionResp),

					// TODO(npolshak) Check status on solo-apis client object once route option status support is added
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
					assertions.CurlEventuallyRespondsAssertion(curlFromPod(ctx), expectedFaultInjectionResp),

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

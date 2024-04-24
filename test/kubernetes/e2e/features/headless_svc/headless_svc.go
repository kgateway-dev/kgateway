package headless_svc

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"

	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/utils/requestutils/curl"
	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	testmatchers "github.com/solo-io/gloo/test/gomega/matchers"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/testutils/assertions"
	"github.com/solo-io/gloo/test/kubernetes/testutils/operations"
	"github.com/solo-io/skv2/codegen/util"
	v1 "github.com/solo-io/solo-apis/pkg/api/gloo.solo.io/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	headlessSvcSetupManifest  = filepath.Join(util.MustGetThisDir(), "inputs/setup.yaml")
	k8sApiRoutingManifest     = filepath.Join(util.MustGetThisDir(), "inputs/k8s_api.yaml")
	classicApiRoutingManifest = filepath.Join(util.MustGetThisDir(), "inputs/classic_api.yaml")

	// When we apply the manifest file, we expect resources to be created with this metadata
	k8sApiProxyObjectMeta = metav1.ObjectMeta{
		Name:      "gloo-proxy-gw",
		Namespace: "default",
	}
	k8sApiProxyDeployment = &appsv1.Deployment{ObjectMeta: k8sApiProxyObjectMeta}
	k8sApiproxyService    = &corev1.Service{ObjectMeta: k8sApiProxyObjectMeta}

	headlessService = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "headless-example-svc",
			Namespace: "default",
		},
	}

	curlPod = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "curl",
			Namespace: "curl",
		},
	}

	expectedHealthyResponse = &testmatchers.HttpResponse{
		StatusCode: http.StatusOK,
		Body:       ContainSubstring("Welcome to nginx!"),
	}
)

var ConfigureRoutingHeadlessSvc = func(useK8sApi bool) e2e.Test {
	return e2e.Test{
		Name:        "HeadlessSvc.ConfigureRoutingHeadlessSvc",
		Description: "routes to headless services",
		Test: func(ctx context.Context, installation *e2e.TestInstallation) {
			commonSetup := operations.ReversibleOperation{
				Do: &operations.BasicOperation{
					OpName:   fmt.Sprintf("apply-manifest-%s", filepath.Base(headlessSvcSetupManifest)),
					OpAction: installation.Actions.Kubectl().NewApplyManifestAction(headlessSvcSetupManifest),
					OpAssertions: []assertions.ClusterAssertion{
						// First check resources are created for headless svc
						installation.Assertions.ObjectsExist(headlessService),
					},
				},
				Undo: &operations.BasicOperation{
					OpName:   fmt.Sprintf("delete-manifest-%s", filepath.Base(headlessSvcSetupManifest)),
					OpAction: installation.Actions.Kubectl().NewDeleteManifestAction(headlessSvcSetupManifest),
					OpAssertion: func(ctx context.Context) {
						// Check resources are deleted for headless svc
						installation.Assertions.ObjectsNotExist(headlessService)
					},
				},
			}

			var routingResourceOp operations.ReversibleOperation
			if useK8sApi {
				routingResourceOp = operations.ReversibleOperation{
					Do: &operations.BasicOperation{
						OpName:   fmt.Sprintf("apply-manifest-%s", filepath.Base(k8sApiRoutingManifest)),
						OpAction: installation.Actions.Kubectl().NewApplyManifestAction(k8sApiRoutingManifest),
						OpAssertions: []assertions.ClusterAssertion{
							// First check resources are created for Gateway
							installation.Assertions.ObjectsExist(k8sApiproxyService, k8sApiProxyDeployment),

							// Check headless svc can be reached
							installation.Assertions.EphemeralCurlEventuallyResponds(curlPod, []curl.Option{
								curl.WithHost(fmt.Sprintf("%s.%s.svc.cluster.local", k8sApiProxyDeployment.GetName(), k8sApiProxyDeployment.GetNamespace())),
								curl.WithHostHeader("headless.example.com"),
								curl.WithPort(80),
							}, expectedHealthyResponse),
						},
					},
					Undo: &operations.BasicOperation{
						OpName:   fmt.Sprintf("delete-manifest-%s", filepath.Base(k8sApiRoutingManifest)),
						OpAction: installation.Actions.Kubectl().NewDeleteManifestAction(k8sApiRoutingManifest),
						OpAssertion: func(ctx context.Context) {
							// Check resources are deleted for Gateway
							installation.Assertions.ObjectsNotExist(k8sApiproxyService, k8sApiProxyDeployment)
						},
					},
				}
			} else {
				routingResourceOp = operations.ReversibleOperation{
					Do: &operations.BasicOperation{
						OpName:   fmt.Sprintf("apply-manifest-%s", filepath.Base(classicApiRoutingManifest)),
						OpAction: installation.Actions.Kubectl().NewApplyManifestAction(classicApiRoutingManifest),
						OpAssertions: []assertions.ClusterAssertion{
							// Check headless svc can be reached
							installation.Assertions.EphemeralCurlEventuallyResponds(curlPod, []curl.Option{
								curl.WithHost(fmt.Sprintf("%s.%s.svc.cluster.local", defaults.GatewayProxyName, installation.Metadata.InstallNamespace)),
								curl.WithHostHeader("headless.example.com"),
								curl.WithPort(80),
							}, expectedHealthyResponse),
						},
					},
					Undo: &operations.BasicOperation{
						OpName:   fmt.Sprintf("delete-manifest-%s", filepath.Base(classicApiRoutingManifest)),
						OpAction: installation.Actions.Kubectl().NewDeleteManifestAction(classicApiRoutingManifest),
						OpAssertion: func(ctx context.Context) {
							// Check classic edge resources are deleted
							installation.Assertions.ObjectsNotExist(
								&v1.Upstream{
									ObjectMeta: metav1.ObjectMeta{
										Namespace: "headless-example-svc",
										Name:      "gloo-system",
									},
								},
							)
						},
					},
				}
			}

			err := installation.Operator.ExecuteReversibleOperations(ctx, commonSetup, routingResourceOp)
			Expect(err).NotTo(HaveOccurred())
		},
	}
}

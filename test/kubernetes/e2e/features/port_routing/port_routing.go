package port_routing

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"

	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/utils/kubeutils"
	"github.com/solo-io/gloo/pkg/utils/requestutils/curl"
	testmatchers "github.com/solo-io/gloo/test/gomega/matchers"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/testutils/assertions"
	"github.com/solo-io/gloo/test/kubernetes/testutils/operations"
	"github.com/solo-io/skv2/codegen/util"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	setupManifest = filepath.Join(util.MustGetThisDir(), "inputs/setup.yaml")

	invalidPortAndValidTargetportManifest   = filepath.Join(util.MustGetThisDir(), "inputs/invalid-port-and-valid-targetport.yaml")
	invalidPortAndInvalidTargetportManifest = filepath.Join(util.MustGetThisDir(), "inputs/invalid-port-and-invalid-targetport.yaml")
	matchPodPortWithoutTargetportManifest   = filepath.Join(util.MustGetThisDir(), "inputs/match-pod-port-without-targetport.yaml")
	matchPortandTargetportManifest          = filepath.Join(util.MustGetThisDir(), "inputs/match-port-and-targetport.yaml")
	invalidPortWithoutTargetportManifest    = filepath.Join(util.MustGetThisDir(), "inputs/invalid-port-without-targetport.yaml")

	// When we apply the deployer-provision.yaml file, we expect resources to be created with this metadata
	glooProxyObjectMeta = metav1.ObjectMeta{
		Name:      "gloo-proxy-gw",
		Namespace: "default",
	}
	proxyDeployment = &appsv1.Deployment{ObjectMeta: glooProxyObjectMeta}
	proxyService    = &corev1.Service{ObjectMeta: glooProxyObjectMeta}

	testService = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-svc",
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

	expectedServiceUnavailableResponse = &testmatchers.HttpResponse{
		StatusCode: http.StatusServiceUnavailable,
		Body:       ContainSubstring("upstream connect error or disconnect/reset before headers. reset reason: remote connection failure, transport failure reason: delayed connect error"),
	}

	setupOp = func(installation *e2e.TestInstallation) operations.ReversibleOperation {
		return operations.ReversibleOperation{
			Do: &operations.BasicOperation{
				OpName:   fmt.Sprintf("apply-manifest-%s", filepath.Base(setupManifest)),
				OpAction: installation.Actions.Kubectl().NewApplyManifestAction(setupManifest),
				OpAssertions: []assertions.ClusterAssertion{
					// First check resources are created for Gateway
					installation.Assertions.ObjectsExist(proxyService, proxyDeployment),
				},
			},
			Undo: &operations.BasicOperation{
				OpName:   fmt.Sprintf("delete-manifest-%s", filepath.Base(setupManifest)),
				OpAction: installation.Actions.Kubectl().NewDeleteManifestAction(setupManifest),
				OpAssertion: func(ctx context.Context) {
					// Check resources are deleted for Gateway
					installation.Assertions.ObjectsNotExist(proxyService, proxyDeployment)
				},
			},
		}
	}
)

var InvalidPortAndValidTargetportManifest = e2e.Test{
	Name:        "PortRouting.InvalidPortAndValidTargetportManifest",
	Description: "with non-matching, yet valid, port and target (app) port",
	Test: func(ctx context.Context, installation *e2e.TestInstallation) {
		portRoutingOp := operations.ReversibleOperation{
			Do: &operations.BasicOperation{
				OpName:   fmt.Sprintf("apply-manifest-%s", filepath.Base(invalidPortAndValidTargetportManifest)),
				OpAction: installation.Actions.Kubectl().NewApplyManifestAction(invalidPortAndValidTargetportManifest),
				OpAssertions: []assertions.ClusterAssertion{
					// First check resources are created for service
					installation.Assertions.ObjectsExist(testService),

					// Check that the valid target port works
					installation.Assertions.EphemeralCurlEventuallyResponds(
						curlPod,
						[]curl.Option{
							curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
							curl.WithHostHeader("example.com"),
						},
						expectedHealthyResponse),
				},
			},
			Undo: &operations.BasicOperation{
				OpName:   fmt.Sprintf("delete-manifest-%s", filepath.Base(invalidPortAndValidTargetportManifest)),
				OpAction: installation.Actions.Kubectl().NewDeleteManifestAction(invalidPortAndValidTargetportManifest),
				OpAssertion: func(ctx context.Context) {
					// Check resources are deleted for service
					installation.Assertions.ObjectsNotExist(testService)
				},
			},
		}

		err := installation.Operator.ExecuteReversibleOperations(ctx, setupOp(installation), portRoutingOp)
		Expect(err).NotTo(HaveOccurred())
	},
}

var MatchPortAndTargetport = e2e.Test{
	Name:        "PortRouting.MatchPortAndTargetport",
	Description: "with matching port and target port",
	Test: func(ctx context.Context, installation *e2e.TestInstallation) {
		portRoutingOp := operations.ReversibleOperation{
			Do: &operations.BasicOperation{
				OpName:   fmt.Sprintf("apply-manifest-%s", filepath.Base(matchPortandTargetportManifest)),
				OpAction: installation.Actions.Kubectl().NewApplyManifestAction(matchPortandTargetportManifest),
				OpAssertions: []assertions.ClusterAssertion{
					// First check resources are created for service
					installation.Assertions.ObjectsExist(testService),

					// Check that the valid target port works
					installation.Assertions.EphemeralCurlEventuallyResponds(
						curlPod,
						[]curl.Option{
							curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
							curl.WithHostHeader("example.com"),
						},
						expectedHealthyResponse),
				},
			},
			Undo: &operations.BasicOperation{
				OpName:   fmt.Sprintf("delete-manifest-%s", filepath.Base(matchPortandTargetportManifest)),
				OpAction: installation.Actions.Kubectl().NewDeleteManifestAction(matchPortandTargetportManifest),
				OpAssertion: func(ctx context.Context) {
					// Check resources are deleted for service
					installation.Assertions.ObjectsNotExist(testService)
				},
			},
		}

		err := installation.Operator.ExecuteReversibleOperations(ctx, setupOp(installation), portRoutingOp)
		Expect(err).NotTo(HaveOccurred())
	},
}

var MatchPodPortWithoutTargetport = e2e.Test{
	Name:        "PortRouting.MatchPodPortWithoutTargetport",
	Description: "without target port, and port matching pod's port",
	Test: func(ctx context.Context, installation *e2e.TestInstallation) {
		portRoutingOp := operations.ReversibleOperation{
			Do: &operations.BasicOperation{
				OpName:   fmt.Sprintf("apply-manifest-%s", filepath.Base(matchPodPortWithoutTargetportManifest)),
				OpAction: installation.Actions.Kubectl().NewApplyManifestAction(matchPodPortWithoutTargetportManifest),
				OpAssertions: []assertions.ClusterAssertion{
					// First check resources are created for service
					installation.Assertions.ObjectsExist(testService),

					// Check that the valid target port works
					installation.Assertions.EphemeralCurlEventuallyResponds(
						curlPod,
						[]curl.Option{
							curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
							curl.WithHostHeader("example.com"),
						},
						expectedHealthyResponse),
				},
			},
			Undo: &operations.BasicOperation{
				OpName:   fmt.Sprintf("delete-manifest-%s", filepath.Base(matchPodPortWithoutTargetportManifest)),
				OpAction: installation.Actions.Kubectl().NewDeleteManifestAction(matchPodPortWithoutTargetportManifest),
				OpAssertion: func(ctx context.Context) {
					// Check resources are deleted for service
					installation.Assertions.ObjectsNotExist(testService)
				},
			},
		}

		err := installation.Operator.ExecuteReversibleOperations(ctx, setupOp(installation), portRoutingOp)
		Expect(err).NotTo(HaveOccurred())
	},
}

var InvalidPortWithoutTargetport = e2e.Test{
	Name:        "PortRouting.InvalidPortWithoutTargetport",
	Description: "without target port, and port **not** matching app's port",
	Test: func(ctx context.Context, installation *e2e.TestInstallation) {
		portRoutingOp := operations.ReversibleOperation{
			Do: &operations.BasicOperation{
				OpName:   fmt.Sprintf("apply-manifest-%s", filepath.Base(invalidPortWithoutTargetportManifest)),
				OpAction: installation.Actions.Kubectl().NewApplyManifestAction(invalidPortWithoutTargetportManifest),
				OpAssertions: []assertions.ClusterAssertion{
					// First check resources are created for service
					installation.Assertions.ObjectsExist(testService),

					// Check that the valid target port works
					installation.Assertions.EphemeralCurlEventuallyResponds(
						curlPod,
						[]curl.Option{
							curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
							curl.WithHostHeader("example.com"),
						},
						expectedServiceUnavailableResponse),
				},
			},
			Undo: &operations.BasicOperation{
				OpName:   fmt.Sprintf("delete-manifest-%s", filepath.Base(invalidPortWithoutTargetportManifest)),
				OpAction: installation.Actions.Kubectl().NewDeleteManifestAction(invalidPortWithoutTargetportManifest),
				OpAssertion: func(ctx context.Context) {
					// Check resources are deleted for service
					installation.Assertions.ObjectsNotExist(testService)
				},
			},
		}

		err := installation.Operator.ExecuteReversibleOperations(ctx, setupOp(installation), portRoutingOp)
		Expect(err).NotTo(HaveOccurred())
	},
}

var InvalidPortAndInvalidTargetportManifest = e2e.Test{
	Name:        "PortRouting.InvalidPortAndInvalidTargetportManifest",
	Description: "pointing to the wrong target port",
	Test: func(ctx context.Context, installation *e2e.TestInstallation) {
		portRoutingOp := operations.ReversibleOperation{
			Do: &operations.BasicOperation{
				OpName:   fmt.Sprintf("apply-manifest-%s", filepath.Base(invalidPortAndInvalidTargetportManifest)),
				OpAction: installation.Actions.Kubectl().NewApplyManifestAction(invalidPortAndInvalidTargetportManifest),
				OpAssertions: []assertions.ClusterAssertion{
					// First check resources are created for service
					installation.Assertions.ObjectsExist(testService),

					// Check that the valid target port works
					installation.Assertions.EphemeralCurlEventuallyResponds(
						curlPod,
						[]curl.Option{
							curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
							curl.WithHostHeader("example.com"),
						},
						expectedServiceUnavailableResponse),
				},
			},
			Undo: &operations.BasicOperation{
				OpName:   fmt.Sprintf("delete-manifest-%s", filepath.Base(invalidPortAndInvalidTargetportManifest)),
				OpAction: installation.Actions.Kubectl().NewDeleteManifestAction(invalidPortAndInvalidTargetportManifest),
				OpAssertion: func(ctx context.Context) {
					// Check resources are deleted for service
					installation.Assertions.ObjectsNotExist(testService)
				},
			},
		}

		err := installation.Operator.ExecuteReversibleOperations(ctx, setupOp(installation), portRoutingOp)
		Expect(err).NotTo(HaveOccurred())
	},
}

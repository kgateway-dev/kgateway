package headless_svc

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
	headlessSvcManifest = filepath.Join(util.MustGetThisDir(), "inputs/setup.yaml")

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

	expectedHealthyResponse = &testmatchers.HttpResponse{
		StatusCode: http.StatusOK,
		Body:       ContainSubstring("Welcome to nginx!"),
	}
)

var ConfigureRoutingHeadlessSvc = e2e.Test{
	Name:        "HeadlessSvc.ConfigureRoutingHeadlessSvc",
	Description: "routes to headless services",
	Test: func(ctx context.Context, installation *e2e.TestInstallation) {
		routeHeadlessSvcOp := operations.ReversibleOperation{
			Do: &operations.BasicOperation{
				OpName:   fmt.Sprintf("apply-manifest-%s", filepath.Base(headlessSvcManifest)),
				OpAction: installation.Actions.Kubectl().NewApplyManifestAction(headlessSvcManifest),
				OpAssertions: []assertions.ClusterAssertion{
					// First check resources are created for Gateay
					installation.Assertions.ObjectsExist(proxyService, proxyDeployment),

					// Check headless svc can be reached
					assertions.CurlEventuallyRespondsAssertion(curlFromPod(ctx), expectedHealthyResponse),
				},
			},
			Undo: &operations.BasicOperation{
				OpName:   fmt.Sprintf("delete-manifest-%s", filepath.Base(headlessSvcManifest)),
				OpAction: installation.Actions.Kubectl().NewDeleteManifestAction(headlessSvcManifest),
				OpAssertion: func(ctx context.Context) {
					// Check resources are deleted for Gateway
					installation.Assertions.ObjectsNotExist(proxyService, proxyDeployment)
				},
			},
		}

		err := installation.Operator.ExecuteReversibleOperations(ctx, routeHeadlessSvcOp)
		Expect(err).NotTo(HaveOccurred())
	},
}

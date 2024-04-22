package virtualhost_options

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	"github.com/solo-io/gloo/pkg/utils/kubeutils/kubectl"
	"github.com/solo-io/gloo/pkg/utils/requestutils/curl"
	"github.com/solo-io/gloo/test/gomega/matchers"
	"github.com/solo-io/gloo/test/gomega/transforms"
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
	targetRefManifest = filepath.Join(util.MustGetThisDir(), "inputs/header-manipulation-targetref.yaml")

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

	// VirtualHostOption resource to be created
	virtualHostOptionMeta = metav1.ObjectMeta{
		Name:      "remove-content-length",
		Namespace: "default",
	}
)

var ConfigureVirtualHostOptionsWithTargetRef = e2e.Test{
	Name:        "VirtualHostOptions.ConfigureVirtualHostOptionsWithTargetRef",
	Description: "the VirtualHostOptions will configure fault inject with a targetRef",
	Test: func(ctx context.Context, installation *e2e.TestInstallation) {
		targetRefRoutingOp := operations.ReversibleOperation{
			Do: &operations.BasicOperation{
				OpName:   fmt.Sprintf("apply-manifest-%s", filepath.Base(targetRefManifest)),
				OpAction: installation.Actions.Kubectl().NewApplyManifestAction(targetRefManifest),
				OpAssertions: []assertions.ClusterAssertion{
					// First check resources are created for Gateway
					installation.Assertions.ObjectsExist(proxyService, proxyDeployment),

					// Check header manipulation is applied
					checkHeaderManipulationFromCluster(installation),

					func(ctx context.Context) {
						// Check status on solo-apis client object
						helpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
							vh, err := installation.ResourceClients.VirtualHostOptionClient().Read(virtualHostOptionMeta.GetNamespace(), virtualHostOptionMeta.GetName(), clients.ReadOpts{})
							installation.Operator.Logf("%s", vh.String())

							return vh, err
						}, 5, 1)

						logged := false
						installation.Operator.Logf("checking for expected status on virtualhost option")
						// Check correct gateway reports status of virtualhost option
						Eventually(func(g Gomega) {
							virtualHostOption, err := installation.ResourceClients.VirtualHostOptionClient().Read(virtualHostOptionMeta.GetNamespace(), virtualHostOptionMeta.GetName(), clients.ReadOpts{})
							g.Expect(err).NotTo(HaveOccurred())
							if !logged {
								installation.Operator.Logf("virtualhost option %s: %s\n", virtualHostOption.Metadata.GetName(), virtualHostOption.String())
								logged = true
							}
							g.Expect(virtualHostOption.GetNamespacedStatuses()).ToNot(BeNil())
							g.Expect(virtualHostOption.GetNamespacedStatuses().GetStatuses()).ToNot(BeEmpty())
							g.Expect(virtualHostOption.GetNamespacedStatuses().GetStatuses()[installation.Metadata.InstallNamespace].GetReportedBy()).To(Equal("gloo-kube-gateway"))
						}, "30s", "15s").Should(Succeed())
					},
				},
			},
			Undo: &operations.BasicOperation{
				OpName:      fmt.Sprintf("delete-manifest-%s", filepath.Base(targetRefManifest)),
				OpAction:    installation.Actions.Kubectl().NewDeleteManifestAction(targetRefManifest),
				OpAssertion: installation.Assertions.ObjectsNotExist(proxyService, proxyDeployment),
			},
		}

		err := installation.Operator.ExecuteReversibleOperations(ctx, targetRefRoutingOp)
		Expect(err).NotTo(HaveOccurred())
	},
}

func checkHeaderManipulationFromCluster(installation *e2e.TestInstallation) assertions.ClusterAssertion {
	return func(ctx context.Context) {
		installation.Operator.Logf("checking routing for header manipulation")
		fdqnAddr := fmt.Sprintf("%s.%s.svc.cluster.local", proxyDeployment.GetName(), proxyDeployment.GetNamespace())

		Eventually(func(g Gomega) {
			// curl gloo-proxy-gw.default.svc.cluster.local:8080 -H "Host: example.com" -v
			var buf threadsafe.Buffer
			kubeCli := kubectl.NewCli().WithReceiver(&buf)
			resp := kubeCli.CurlFromEphemeralPod(ctx, curlPod.ObjectMeta,
				curl.WithHost(fdqnAddr),
				curl.WithHostHeader("example.com"),
				curl.WithHeadersOnly(),
				curl.VerboseOutput())
			g.Expect(resp).To(WithTransform(transforms.WithCurlHttpResponse, matchers.HaveHttpResponse(&matchers.HttpResponse{
				StatusCode: http.StatusOK,
				Custom:     Not(matchers.ContainHeaderKeys([]string{"content-length"})),
				Body:       gstruct.Ignore(),
			})), resp)
		}, "10s", "1s", "curl should eventually return response without content-length").Should(Succeed())
	}
}

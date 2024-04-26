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
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/testutils/assertions"
	"github.com/solo-io/gloo/test/kubernetes/testutils/operations"
	"github.com/solo-io/go-utils/threadsafe"
	"github.com/solo-io/skv2/codegen/util"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	targetRefManifest      = filepath.Join(util.MustGetThisDir(), "inputs/header-manipulation-targetref.yaml")
	sectionNameVhOManifest = filepath.Join(util.MustGetThisDir(), "inputs/section-name-vho.yaml")
	extraVhOManifest       = filepath.Join(util.MustGetThisDir(), "inputs/extra-vho.yaml")

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
	// Extra VirtualHostOption resource to be created
	extraVirtualHostOptionMeta = metav1.ObjectMeta{
		Name:      "remove-content-type",
		Namespace: "default",
	}
	// SectionName VirtualHostOption resource to be created
	sectionNameVirtualHostOptionMeta = metav1.ObjectMeta{
		Name:      "add-foo-header",
		Namespace: "default",
	}
)

var ConfigureVirtualHostOptionsWithTargetRef = e2e.Test{
	Name:        "VirtualHostOptions.ConfigureVirtualHostOptionsWithTargetRef",
	Description: "the VirtualHostOptions will configure header manipulation with a targetRef",
	Test: func(ctx context.Context, installation *e2e.TestInstallation) {
		targetRefRoutingOp := operations.ReversibleOperation{
			Do: &operations.BasicOperation{
				OpName:   fmt.Sprintf("apply-manifest-%s", filepath.Base(targetRefManifest)),
				OpAction: installation.Actions.Kubectl().NewApplyManifestAction(targetRefManifest),
				OpAssertions: []assertions.ClusterAssertion{
					// First check resources are created for Gateway
					installation.Assertions.ObjectsExist(proxyService, proxyDeployment),

					// Check header manipulation is applied
					checkHeaderManipulationFromCluster(installation, expectedResponseWithoutContentType),

					assertions.EventuallyResourceStatusMatchesState(installation.Metadata.InstallNamespace,
						func() (resources.InputResource, error) {
							return installation.ResourceClients.VirtualHostOptionClient().Read(virtualHostOptionMeta.GetNamespace(), virtualHostOptionMeta.GetName(), clients.ReadOpts{})
						},
						core.Status_Accepted,
						"gloo-kube-gateway"),
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

var ConfigureMultipleVirtualHostOptionsWithTargetRef = e2e.Test{
	Name:        "VirtualHostOptions.ConfigureMultipleVirtualHostOptionsWithTargetRef",
	Description: "the VirtualHostOptions will configure header manipulation with a targetRef",
	Test: func(ctx context.Context, installation *e2e.TestInstallation) {
		targetRefRoutingOp := operations.ReversibleOperation{
			Do: &operations.BasicOperation{
				OpName: "apply-manifests",
				OpAction: func(ctx context.Context) error {
					if err := installation.Actions.Kubectl().NewApplyManifestAction(targetRefManifest)(ctx); err != nil {
						return err
					}
					if err := installation.Actions.Kubectl().NewApplyManifestAction(extraVhOManifest)(ctx); err != nil {
						return err
					}
					return nil
				},
				OpAssertions: []assertions.ClusterAssertion{
					// First check resources are created for Gateway
					installation.Assertions.ObjectsExist(proxyService, proxyDeployment),

					// Check header manipulation is applied
					checkHeaderManipulationFromCluster(installation, expectedResponseWithoutContentType),

					assertions.EventuallyResourceStatusMatchesState(installation.Metadata.InstallNamespace,
						func() (resources.InputResource, error) {
							return installation.ResourceClients.VirtualHostOptionClient().Read(virtualHostOptionMeta.GetNamespace(), virtualHostOptionMeta.GetName(), clients.ReadOpts{})
						},
						core.Status_Accepted,
						"gloo-kube-gateway"),

					assertions.EventuallyResourceStatusMatchesWarningReasons(installation.Metadata.InstallNamespace,
						func() (resources.InputResource, error) {
							return installation.ResourceClients.VirtualHostOptionClient().Read(extraVirtualHostOptionMeta.GetNamespace(), extraVirtualHostOptionMeta.GetName(), clients.ReadOpts{})
						},
						[]string{"conflict with more-specific or older VirtualHostOption"},
						"gloo-kube-gateway"),
				},
			},
			Undo: &operations.BasicOperation{
				OpName: "delete-manifests",
				OpAction: func(ctx context.Context) error {
					if err := installation.Actions.Kubectl().NewDeleteManifestAction(targetRefManifest)(ctx); err != nil {
						return err
					}
					if err := installation.Actions.Kubectl().NewDeleteManifestAction(extraVhOManifest)(ctx); err != nil {
						return err
					}
					return nil
				},
				OpAssertion: installation.Assertions.ObjectsNotExist(proxyService, proxyDeployment),
			},
		}

		err := installation.Operator.ExecuteReversibleOperations(ctx, targetRefRoutingOp)
		Expect(err).NotTo(HaveOccurred())
	},
}

var ConfigureVirtualHostOptionsWithTargetRefWithSectionName = e2e.Test{
	Name:        "VirtualHostOptions.ConfigureVirtualHostOptionsWithTargetRefWithSectionName",
	Description: "the VirtualHostOptions will configure header manipulation with a targetRef that has a section name",
	Test: func(ctx context.Context, installation *e2e.TestInstallation) {
		targetRefRoutingOp := operations.ReversibleOperation{
			Do: &operations.BasicOperation{
				OpName: "apply-manifests",
				OpAction: func(ctx context.Context) error {
					if err := installation.Actions.Kubectl().NewApplyManifestAction(targetRefManifest)(ctx); err != nil {
						return err
					}
					if err := installation.Actions.Kubectl().NewApplyManifestAction(extraVhOManifest)(ctx); err != nil {
						return err
					}
					if err := installation.Actions.Kubectl().NewApplyManifestAction(sectionNameVhOManifest)(ctx); err != nil {
						return err
					}
					return nil
				},
				OpAssertions: []assertions.ClusterAssertion{
					// First check resources are created for Gateway
					installation.Assertions.ObjectsExist(proxyService, proxyDeployment),

					// Check header manipulation is applied
					checkHeaderManipulationFromCluster(installation, expectedResponseWithFooHeader),

					assertions.EventuallyResourceStatusMatchesState(installation.Metadata.InstallNamespace,
						func() (resources.InputResource, error) {
							return installation.ResourceClients.VirtualHostOptionClient().Read(virtualHostOptionMeta.GetNamespace(), virtualHostOptionMeta.GetName(), clients.ReadOpts{})
						},
						core.Status_Warning,
						"gloo-kube-gateway"),
					assertions.EventuallyResourceStatusMatchesWarningReasons(installation.Metadata.InstallNamespace,
						func() (resources.InputResource, error) {
							return installation.ResourceClients.VirtualHostOptionClient().Read(virtualHostOptionMeta.GetNamespace(), virtualHostOptionMeta.GetName(), clients.ReadOpts{})
						},
						[]string{"conflict with more-specific or older VirtualHostOption"},
						"gloo-kube-gateway"),
					assertions.EventuallyResourceStatusMatchesState(installation.Metadata.InstallNamespace,
						func() (resources.InputResource, error) {
							return installation.ResourceClients.VirtualHostOptionClient().Read(extraVirtualHostOptionMeta.GetNamespace(), extraVirtualHostOptionMeta.GetName(), clients.ReadOpts{})
						},
						core.Status_Warning,
						"gloo-kube-gateway"),
					assertions.EventuallyResourceStatusMatchesWarningReasons(installation.Metadata.InstallNamespace,
						func() (resources.InputResource, error) {
							return installation.ResourceClients.VirtualHostOptionClient().Read(extraVirtualHostOptionMeta.GetNamespace(), extraVirtualHostOptionMeta.GetName(), clients.ReadOpts{})
						},
						[]string{"conflict with more-specific or older VirtualHostOption"},
						"gloo-kube-gateway"),
					assertions.EventuallyResourceStatusMatchesState(installation.Metadata.InstallNamespace,
						func() (resources.InputResource, error) {
							return installation.ResourceClients.VirtualHostOptionClient().Read(sectionNameVirtualHostOptionMeta.GetNamespace(), sectionNameVirtualHostOptionMeta.GetName(), clients.ReadOpts{})
						},
						core.Status_Accepted,
						"gloo-kube-gateway"),
				},
			},
			Undo: &operations.BasicOperation{
				OpName: "delete-manifests",
				OpAction: func(ctx context.Context) error {
					if err := installation.Actions.Kubectl().NewDeleteManifestAction(targetRefManifest)(ctx); err != nil {
						return err
					}
					if err := installation.Actions.Kubectl().NewDeleteManifestAction(extraVhOManifest)(ctx); err != nil {
						return err
					}
					if err := installation.Actions.Kubectl().NewDeleteManifestAction(sectionNameVhOManifest)(ctx); err != nil {
						return err
					}
					return nil
				},
				OpAssertion: installation.Assertions.ObjectsNotExist(proxyService, proxyDeployment),
			},
		}

		err := installation.Operator.ExecuteReversibleOperations(ctx, targetRefRoutingOp)
		Expect(err).NotTo(HaveOccurred())
	},
}

var expectedResponseWithoutContentType = &matchers.HttpResponse{
	StatusCode: http.StatusOK,
	Custom:     Not(matchers.ContainHeaderKeys([]string{"content-length"})),
	Body:       gstruct.Ignore(),
}

var expectedResponseWithFooHeader = &matchers.HttpResponse{
	StatusCode: http.StatusOK,
	Headers: map[string]interface{}{
		"foo": Equal("bar"),
	},
	// Make sure the content-length isn't being removed as a function of the unwanted VHO
	Custom: matchers.ContainHeaderKeys([]string{"content-length"}),
	Body:   gstruct.Ignore(),
}

func checkHeaderManipulationFromCluster(installation *e2e.TestInstallation, expected *matchers.HttpResponse) assertions.ClusterAssertion {
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
			g.Expect(resp).To(WithTransform(transforms.WithCurlHttpResponse, matchers.HaveHttpResponse(expected)), resp)
		}, "10s", "1s", "curl should eventually return response with expected form").Should(Succeed())
	}
}

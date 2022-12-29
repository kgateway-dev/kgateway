package check_test

import (
	"context"
	"fmt"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/testutils"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Check", func() {

	var (
		ctx    context.Context
		cancel context.CancelFunc
	)

	BeforeEach(func() {
		helpers.UseMemoryClients()
		ctx, cancel = context.WithCancel(context.Background())
	})

	AfterEach(func() {
		cancel()
	})

	Context("glooctl check", func() {
		FIt("should error if resource has no status", func() {

			client := helpers.MustKubeClient()
			client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: defaults.GlooSystem,
				},
			}, metav1.CreateOptions{})

			appName := "default"
			client.AppsV1().Deployments("gloo-system").Create(ctx, &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      appName,
					Namespace: "gloo-system",
				},
				Spec: appsv1.DeploymentSpec{},
			}, metav1.CreateOptions{})

			helpers.MustNamespacedSettingsClient(ctx, "gloo-system").Write(&v1.Settings{
				Metadata: &core.Metadata{
					Name:      "default",
					Namespace: "gloo-system",
				},
			}, clients.WriteOpts{})

			noStatusUpstream := &v1.Upstream{
				Metadata: &core.Metadata{
					Name:      "some-warning-upstream",
					Namespace: "gloo-system",
				},
			}
			_, usErr := helpers.MustNamespacedUpstreamClient(ctx, "gloo-system").Write(noStatusUpstream, clients.WriteOpts{})
			Expect(usErr).NotTo(HaveOccurred())

			_, err := testutils.GlooctlOut("check -x xds-metrics,proxies")
			Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("Found upstream with no status: %s %s", noStatusUpstream.GetMetadata().GetNamespace(), noStatusUpstream.GetMetadata().GetName())))
		})
	})
})

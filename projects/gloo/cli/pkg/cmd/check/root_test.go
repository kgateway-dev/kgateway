package check_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v12 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
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

var _ = Describe("Root", func() {

	BeforeEach(func() {
		helpers.UseMemoryClients()
	})

	Context("With a good kube client", func() {

		It("all checks pass with OK status", func() {

			client := helpers.MustKubeClient()
			client.CoreV1().Namespaces().Create(&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: defaults.GlooSystem,
				},
			})

			appName := "default"
			client.AppsV1().Deployments("gloo-system").Create(&appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      appName,
					Namespace: "gloo-system",
				},
				Spec: appsv1.DeploymentSpec{},
			})

			helpers.MustNamespacedSettingsClient("gloo-system").Write(&v1.Settings{
				Metadata: core.Metadata{
					Name:      "default",
					Namespace: "gloo-system",
				},
			}, clients.WriteOpts{})

			output, err := testutils.GlooctlOut("check -x xds-metrics")
			Expect(err).NotTo(HaveOccurred())

			Expect(output).To(ContainSubstring("Checking deployments... OK"))
			Expect(output).To(ContainSubstring("Checking pods... OK"))
			Expect(output).To(ContainSubstring("Checking upstreams... OK"))
			Expect(output).To(ContainSubstring("Checking upstream groups... OK"))
			Expect(output).To(ContainSubstring("Checking auth configs... OK"))
			Expect(output).To(ContainSubstring("Checking rate limit configs... OK"))
			Expect(output).To(ContainSubstring("Checking secrets... OK"))
			Expect(output).To(ContainSubstring("Checking virtual services... OK"))
			Expect(output).To(ContainSubstring("Checking gateways... OK"))
			Expect(output).To(ContainSubstring("Checking proxies... OK"))
		})

		It("reports multiple errors at one time", func() {
			client := helpers.MustKubeClient()
			client.CoreV1().Namespaces().Create(&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: defaults.GlooSystem,
				},
			})

			appName := "default"
			client.AppsV1().Deployments("gloo-system").Create(&appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      appName,
					Namespace: "gloo-system",
				},
				Spec: appsv1.DeploymentSpec{},
			})

			helpers.MustNamespacedSettingsClient("gloo-system").Write(&v1.Settings{
				Metadata: core.Metadata{
					Name:      "default",
					Namespace: "gloo-system",
				},
			}, clients.WriteOpts{})

			// Creates rejected upstream in the gloo-system namespace

			helpers.MustNamespacedUpstreamClient("gloo-system").Write(&v1.Upstream{
				Metadata: core.Metadata{
					Name:      "some-warning-upstream",
					Namespace: "gloo-system",
				},
				Status: core.Status{
					State:  core.Status_Warning,
					Reason: "I am an upstream with a warning",
				},
			}, clients.WriteOpts{})

			helpers.MustNamespacedUpstreamClient("gloo-system").Write(&v1.Upstream{
				Metadata: core.Metadata{
					Name:      "some-rejected-upstream",
					Namespace: "gloo-system",
				},
				Status: core.Status{
					State:  core.Status_Rejected,
					Reason: "I am a rejected upstream",
				},
			}, clients.WriteOpts{})

			helpers.MustNamespacedVirtualServiceClient("gloo-system").Write(
				&v12.VirtualService{
					Metadata: core.Metadata{Name: "some-bad-vs", Namespace: "gloo-system"},
					Status: core.Status{
						State:  core.Status_Rejected,
						Reason: "I am a rejected vs",
					},
				}, clients.WriteOpts{},
			)
			testutils.Glooctl("check -x xds-metrics")

			output, err := testutils.GlooctlOut("check -x xds-metrics")
			Expect(err).To(HaveOccurred())
			Expect(output).To(ContainSubstring("Checking upstreams... 2 Errors!"))
			Expect(output).To(ContainSubstring("Checking virtual services... 1 Errors!"))
			Expect(output).To(ContainSubstring("Found rejected upstream"))
			Expect(output).To(ContainSubstring("Found upstream with warnings"))
			Expect(output).To(ContainSubstring("Found rejected virtual service"))

		})

	})

	Context("With a custom namespace", func() {

		It("connection fails on incorrect namespace check", func() {

			myNs := "my-namespace"
			client := helpers.MustKubeClient()
			client.CoreV1().Namespaces().Create(&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: myNs,
				},
			})

			appName := "default"
			client.AppsV1().Deployments(myNs).Create(&appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      appName,
					Namespace: myNs,
				},
				Spec: appsv1.DeploymentSpec{},
			})

			helpers.MustNamespacedSettingsClient(myNs).Write(&v1.Settings{
				Metadata: core.Metadata{
					Name:      "default",
					Namespace: myNs,
				},
			}, clients.WriteOpts{})

			output, _ := testutils.GlooctlOut("check -x xds-metrics")
			Expect(output).To(ContainSubstring("1 error occurred:"))
			Expect(output).To(ContainSubstring("namespaces \"gloo-system\" not found"))

			output, _ = testutils.GlooctlOut("check -x xds-metrics -n my-namespace")
			Expect(output).To(ContainSubstring("No problems detected."))

		})
	})

})

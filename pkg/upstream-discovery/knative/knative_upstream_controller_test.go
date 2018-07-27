package knative_test

import (
	"path/filepath"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"os"
	"time"

	. "github.com/solo-io/gloo/pkg/upstream-discovery/knative"

	knativeplugin "github.com/solo-io/gloo/pkg/plugins/knative"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/solo-io/gloo/test/helpers"
)

var _ = Describe("Knative Service Discovery", func() {

	var (
		namespace string
		clients   helpers.Clients
	)

	BeforeEach(func() {
		if os.Getenv("RUN_KUBE_TESTS") != "1" {
			Skip("This test creates kubernetes resources and is disabled by default. To enable, set RUN_KUBE_TESTS=1 in your env.")
		}

		clients = helpers.GetClients(namespace)

		var options metav1.GetOptions
		svc, err := clients.Kube.CoreV1().Services("istio-system").Get("knative-ingressgateway", options)
		if err != nil || svc == nil {
			Skip("this test must be run with knative pre-installed")
		}

		namespace = helpers.RandString(8)
		err = helpers.SetupKubeForTest(namespace)
		Expect(err).NotTo(HaveOccurred())

		err = helpers.Kubectl("label", "namespace", namespace, "istio-injection=enabled", "--overwrite")

		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		helpers.TeardownKube(namespace)

	})

	var stopCh chan struct{}
	BeforeEach(func() {
		discovery, err := NewUpstreamController(clients.RestConfig, clients.Gloo, time.Minute)
		Expect(err).NotTo(HaveOccurred())
		stopCh = make(chan struct{})
		go func() {
			defer GinkgoRecover()
			discovery.Run(stopCh)
		}()
		Expect(err).NotTo(HaveOccurred())
	})
	AfterEach(func() {
		close(stopCh)
		stopCh = nil
	})

	Context("a service exists", func() {

		It("Gets an upstream that was created", func() {
			helpers.Kubectl("apply", "-n", namespace, "-f", filepath.Join(helpers.KubeResourcesDirectory(), "knative.yaml"))
			helpers.Kubectl("get", "-n", namespace, "-f", filepath.Join(helpers.KubeResourcesDirectory(), "knative.yaml"))

			Eventually(
				func() bool {
					createdUpstreams, err := clients.Gloo.V1().Upstreams().List()
					Expect(err).NotTo(HaveOccurred())

					for _, us := range createdUpstreams {
						if us.Type == knativeplugin.UpstreamTypeKnative {
							return true
						}
					}
					return false
				}, "1m", "1s").Should(BeTrue())
		})
	})
})

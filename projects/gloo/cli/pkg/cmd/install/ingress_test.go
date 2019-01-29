package install_test

import (
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/testutils"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	helpers2 "github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/go-utils/kubeutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	kubeerrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

var _ = Describe("Ingress", func() {
	BeforeEach(func() {
		err := testutils.Make(helpers2.GlooDir(), "install/gloo-ingress.yaml")
		Expect(err).NotTo(HaveOccurred())
	})
	AfterEach(func() {
		os.Remove(filepath.Join(helpers2.GlooDir(), "install", "gloo-ingress.yaml"))
	})
	It("should install the gloo ingress", func() {
		err := testutils.Glooctl("install ingress --file " + filepath.Join(helpers2.GlooInstallDir(), "gloo-ingress.yaml"))
		Expect(err).NotTo(HaveOccurred())

		// when we see that discovery has created an upstream for gateway-proxy, we're good
		var us *v1.Upstream
		Eventually(func() (*v1.Upstream, error) {
			u, err := helpers.MustUpstreamClient().Read("gloo-system", "gloo-system-ingress-proxy-80", clients.ReadOpts{})
			us = u
			return us, err
		}, time.Minute).Should(Not(BeNil()))

		err = testutils.Glooctl("uninstall")
		Expect(err).NotTo(HaveOccurred())

		// uninstall should clear out all the crds from the namespace
		Eventually(func() bool {
			cfg, err := kubeutils.GetConfig("", "")
			Expect(err).NotTo(HaveOccurred())
			kube, err := kubernetes.NewForConfig(cfg)
			Expect(err).NotTo(HaveOccurred())
			_, err = kube.CoreV1().Namespaces().Get("gloo-system", metav1.GetOptions{})
			return err != nil && kubeerrs.IsNotFound(err)
		}, time.Minute).Should(BeTrue())
	})
})

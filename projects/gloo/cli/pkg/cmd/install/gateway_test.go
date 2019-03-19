package install_test

import (
	"os"
	"path/filepath"
	"time"

	"k8s.io/client-go/kubernetes"

	helpers2 "github.com/solo-io/gloo/test/helpers"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/testutils"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"

	"github.com/solo-io/go-utils/kubeutils"
	kubeerrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Gateway", func() {
	BeforeEach(func() {
		err := testutils.Make(helpers2.GlooDir(), "install/gloo-gateway.yaml")
		Expect(err).NotTo(HaveOccurred())
	})
	AfterEach(func() {
		os.Remove(filepath.Join(helpers2.GlooDir(), "install", "gloo-gateway.yaml"))
	})

	It("should install the gloo gateway", func() {
		err := testutils.Glooctl("install gateway --file " + filepath.Join(helpers2.GlooInstallDir(), "gloo-gateway.yaml"))
		Expect(err).NotTo(HaveOccurred())

		// when we see that discovery has created an upstream for gateway-proxy, we're good
		var us *v1.Upstream
		Eventually(func() (*v1.Upstream, error) {
			u, err := helpers.MustUpstreamClient().Read("gloo-system", "gloo-system-gateway-proxy-8080", clients.ReadOpts{})
			us = u
			return us, err
		}, time.Minute).Should(Not(BeNil()))

		err = testutils.Glooctl("uninstall")
		Expect(err).NotTo(HaveOccurred())

		// uninstall should delete the namespace
		Eventually(func() bool {
			cfg, err := kubeutils.GetConfig("", "")
			Expect(err).NotTo(HaveOccurred())
			kube, err := kubernetes.NewForConfig(cfg)
			Expect(err).NotTo(HaveOccurred())
			_, err = kube.CoreV1().Namespaces().Get("gloo-system", metav1.GetOptions{})
			return err != nil && kubeerrs.IsNotFound(err)
		}, time.Minute).Should(BeTrue())
	})

	It("should print out the correct manifest on dry run ", func() {
		manifest, err := testutils.GlooctlOut("install gateway --file " + filepath.Join(helpers2.GlooInstallDir(), "gloo-gateway.yaml") + " --dry-run")
		Expect(err).NotTo(HaveOccurred())

		parsedManifest
	})
})

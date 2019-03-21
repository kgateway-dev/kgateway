package install_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/testutils"
)

var _ = Describe("Install", func() {

	/**
	NOTE: If these tests start failing, it could mean we've added a new kind of resource that gets created at install time.
	These are strictly validated in the CLI installer so they can be cleaned up correctly during uninstall. To fix that issue,
	add the new kind to the installKinds slice here: projects/gloo/cli/pkg/cmd/install/util.go
	*/

	It("shouldn't get errors for gateway dry run", func() {
		_, err := testutils.GlooctlOut("install gateway --file https://storage.googleapis.com/solo-public-helm/charts/gloo-0.11.1.tgz --dry-run")
		Expect(err).NotTo(HaveOccurred())
	})

	It("shouldn't get errors for knative dry run", func() {
		_, err := testutils.GlooctlOut("install knative --file https://storage.googleapis.com/solo-public-helm/charts/gloo-0.11.1.tgz --dry-run")
		Expect(err).NotTo(HaveOccurred())
	})

	It("shouldn't get errors for ingress dry run", func() {
		_, err := testutils.GlooctlOut("install ingress --file https://storage.googleapis.com/solo-public-helm/charts/gloo-0.11.1.tgz --dry-run")
		Expect(err).NotTo(HaveOccurred())
	})

	It("should error when not overriding helm chart in dev mode", func() {
		_, err := testutils.GlooctlOut("install ingress")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("installing gloo in ingress mode: you must provide a Gloo Helm chart URI via the 'file' option when running an unreleased version of glooctl"))
	})

	It("should error when not providing file with valid extension", func() {
		_, err := testutils.GlooctlOut("install gateway --file foo")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("installing gloo in gateway mode: installing Gloo from helm chart: unsupported file extension for Helm chart URI: [foo]. Extension must either be .tgz or .tar.gz"))
	})

	It("should error when not providing valid file", func() {
		_, err := testutils.GlooctlOut("install gateway --file foo.tgz")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("installing gloo in gateway mode: installing Gloo from helm chart: retrieving gloo helm chart archive"))
	})
})

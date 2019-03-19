package install_test

import (
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/testutils"
)

var _ = Describe("Install", func() {
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
		output, err := testutils.GlooctlOut("install ingress")
		Expect(err).NotTo(HaveOccurred())
		Expect(strings.HasPrefix(output, "Error: installing gloo in ingress mode: you must provide a Gloo Helm chart URI via the 'file' option when running an unreleased version of glooctl")).To(BeTrue())
	})

	It("should error when not providing file with valid extension", func() {
		output, err := testutils.GlooctlOut("install gateway --file foo")
		Expect(err).NotTo(HaveOccurred())
		Expect(strings.HasPrefix(output, "Error: installing gloo in gateway mode: installing Gloo from helm chart: unsupported file extension for Helm chart URI: [foo]. Extension must either be .tgz or .tar.gz")).To(BeTrue())
	})

	It("should error when not providing valid file", func() {
		output, err := testutils.GlooctlOut("install gateway --file foo.tgz")
		Expect(err).NotTo(HaveOccurred())
		Expect(strings.HasPrefix(output, "Error: installing gloo in gateway mode: installing Gloo from helm chart: retrieving gloo helm chart archive")).To(BeTrue())
	})
})

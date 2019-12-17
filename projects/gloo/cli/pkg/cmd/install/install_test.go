package install_test

import (
	"fmt"
	"path/filepath"

	"github.com/solo-io/go-utils/testutils/exec"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/testutils"
)

var _ = Describe("Install", func() {

	const licenseKey = "--license-key=fake-license-key"
	const overrideVersion = "0.20.7"

	It("shouldn't get errors for gateway dry run", func() {
		_, err := testutils.GlooctlOut(fmt.Sprintf("install gateway --file %s --dry-run", file))
		Expect(err).NotTo(HaveOccurred())
	})

	It("shouldn't get errors for gateway upgrade dry run", func() {
		_, err := testutils.GlooctlOut(fmt.Sprintf("install gateway --file %s --dry-run --upgrade", file))
		Expect(err).NotTo(HaveOccurred())
	})

	It("shouldn't get errors for gateway dry run with multiple values", func() {
		outputYaml, err := testutils.GlooctlOut(fmt.Sprintf("install gateway --file %s --dry-run --values %s,%s ", file, values1, values2))
		Expect(err).NotTo(HaveOccurred())
		// Test that the values are being merged as we expect
		Expect(outputYaml).To(ContainSubstring("test-namespace-2\n"))
	})

	It("shouldn't get errors when overriding release version", func() {
		_, err := testutils.GlooctlOut(fmt.Sprintf("install gateway --release-version %s --dry-run", overrideVersion))
		Expect(err).NotTo(HaveOccurred())
	})

	It("shouldn't allow both --file and --release-version flags", func() {
		_, err := testutils.GlooctlOut(fmt.Sprintf("install gateway --file %s --dry-run --release-version %s ", file, overrideVersion))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("you may only provide one override value, detected two: %s and %s", file, overrideVersion)))
	})

	It("shouldn't get errors for enterprise dry run", func() {
		_, err := testutils.GlooctlOut(fmt.Sprintf("install gateway enterprise --file %s --dry-run %s", file, licenseKey))
		Expect(err).NotTo(HaveOccurred())
	})

	It("shouldn't get errors for enterprise dry run without file", func() {
		_, err := testutils.GlooctlOut(fmt.Sprintf("install gateway enterprise --dry-run %s", licenseKey))
		Expect(err).NotTo(HaveOccurred())
	})

	It("shouldn't get errors when overriding enterprise version", func() {
		_, err := testutils.GlooctlOut(fmt.Sprintf("install gateway enterprise --release-version %s --dry-run %s", overrideVersion, licenseKey))
		Expect(err).NotTo(HaveOccurred())
	})

	It("shouldn't get errors for enterprise upgrade dry run", func() {
		_, err := testutils.GlooctlOut(fmt.Sprintf("install gateway enterprise --file %s --dry-run --upgrade", file))
		Expect(err).NotTo(HaveOccurred())
	})

	It("shouldn't get errors for knative dry run", func() {
		_, err := testutils.GlooctlOut(fmt.Sprintf("install knative --file %s --dry-run", file))
		Expect(err).NotTo(HaveOccurred())
	})

	It("shouldn't get errors for ingress dry run", func() {
		_, err := testutils.GlooctlOut(fmt.Sprintf("install ingress --file %s --dry-run", file))
		Expect(err).NotTo(HaveOccurred())
	})

	It("should error when not overriding helm chart in dev mode", func() {
		_, err := testutils.GlooctlOut("install ingress --dry-run")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("installing gloo in ingress mode: you must provide a Gloo Helm chart URI via the 'file' option when running an unreleased version of glooctl"))
	})

	It("should error when not providing file with valid extension", func() {
		_, err := testutils.GlooctlOut("install gateway --file foo --dry-run")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("installing gloo in gateway mode: unsupported file extension for Helm chart URI: [foo]. Extension must either be .tgz or .tar.gz"))
	})

	It("should error when not providing valid file", func() {
		_, err := testutils.GlooctlOut("install gateway --file foo.tgz --dry-run")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("no such file or directory"))
	})

	It("should not error when providing the admin console flag", func() {
		// This test fetches the corresponding GlooE helm chart, thus it needs the version that gets linked
		// into the glooctl binary at build time
		out, err := exec.RunCommandOutput(RootDir, true, filepath.Join("_output", "glooctl"), "install", "gateway", "--dry-run", "--with-admin-console")
		Expect(err).NotTo(HaveOccurred())
		Expect(out).NotTo(BeEmpty())
	})

	It("should not error when providing a new release-name flag value", func() {
		out, err := testutils.GlooctlOut(fmt.Sprintf("install gateway --file %s --release-name test --dry-run", file))
		Expect(err).NotTo(HaveOccurred())
		Expect(out).NotTo(BeEmpty())
	})

})

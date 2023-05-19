package install_test

import (
	"fmt"

	"github.com/rotisserie/eris"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/version"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/install"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/testutils"
)

var _ = Describe("Install", func() {

	const licenseKey = "--license-key=fake-license-key"
	const overrideVersion = "1.7.0-beta11"

	BeforeEach(func() {
		version.Version = version.UndefinedVersion // we're testing an "unreleased" glooctl
	})

	It("should error for gateway dry run on unreleased glooctl", func() {
		_, err := testutils.GlooctlOut("install gateway --dry-run")
		Expect(err).To(MatchError(install.UnreleasedWithoutOverrideErr))
	})

	It("shouldn't error for gateway dry run on released glooctl", func() {
		version.Version = "1.3.2" // pretend we set this using linker on a release build of glooctl
		_, err := testutils.GlooctlOut("install gateway --dry-run")
		Expect(err).ToNot(HaveOccurred())
	})

	It("should error for gateway dry run on released glooctl with bad linked version", func() {
		version.Version = "1.3.2-11-g271bd663c" // pretend we set this using linker on a release build of glooctl
		_, err := testutils.GlooctlOut("install gateway --dry-run")
		Expect(err).To(MatchError(install.UnreleasedWithoutOverrideErr))
	})

	It("shouldn't get errors for gateway dry run with file override", func() {
		_, err := testutils.GlooctlOut(fmt.Sprintf("install gateway --file %s --dry-run", file))
		Expect(err).NotTo(HaveOccurred())
	})

	It("shouldn't get errors for gateway dry run with multiple values", func() {
		outputYaml, err := testutils.GlooctlOut(fmt.Sprintf("install gateway --file %s --dry-run --values %s,%s ", file, values1, values2))
		Expect(err).NotTo(HaveOccurred())
		// Test that the values are being merged as we expect
		Expect(outputYaml).To(ContainSubstring("test-namespace-2\n"))
	})

	It("shouldn't get errors when overriding release version", func() {
		_, err := testutils.GlooctlOut(fmt.Sprintf("install gateway --version %s --dry-run", overrideVersion))
		Expect(err).NotTo(HaveOccurred())
	})

	It("shouldn't allow both --file and --version flags", func() {
		_, err := testutils.GlooctlOut(fmt.Sprintf("install gateway --file %s --dry-run --version %s ", file, overrideVersion))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(install.ChartAndReleaseFlagErr(file, overrideVersion).Error()))
	})

	It("shouldn't get errors for enterprise dry run", func() {
		_, err := testutils.GlooctlOut(fmt.Sprintf("install gateway enterprise --file %s --dry-run %s", file, licenseKey))
		Expect(err).NotTo(HaveOccurred())
	})

	It("should get errors for installing enterprise without license key", func() {
		_, err := testutils.GlooctlOut(fmt.Sprintf("install gateway enterprise --file %s --dry-run", file))
		Expect(err).To(MatchError(eris.New("No license key provided, please re-run the install with the following flag `--license-key=<YOUR-LICENSE-KEY>")))
	})

	It("shouldn't get errors for enterprise dry run without file", func() {
		_, err := testutils.GlooctlOut(fmt.Sprintf("install gateway enterprise --dry-run %s", licenseKey))
		Expect(err).NotTo(HaveOccurred())
	})

	It("shouldn't install federation when with-gloo-fed is false", func() {
		_, err := testutils.GlooctlOut(fmt.Sprintf("install gateway enterprise --file %s --dry-run %s  --with-gloo-fed=false", file, licenseKey))
		Expect(err).NotTo(HaveOccurred())
	})

	It("shouldn't install federation when with-gloo-fed is false without file", func() {
		_, err := testutils.GlooctlOut(fmt.Sprintf("install gateway enterprise --dry-run %s --with-gloo-fed=false", licenseKey))
		Expect(err).NotTo(HaveOccurred())
	})

	It("shouldn't get errors when overriding enterprise version", func() {
		_, err := testutils.GlooctlOut(fmt.Sprintf("install gateway enterprise --version %s --dry-run %s", overrideVersion, licenseKey))
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

	It("should contain 'ingress-proxy' for ingress dry run with enterprise helm chart override", func() {
		outputYaml, err := testutils.GlooctlOut(fmt.Sprintf("install ingress --file https://storage.googleapis.com/gloo-ee-helm/charts/gloo-ee-1.0.0-rc8.tgz --dry-run"))
		Expect(err).NotTo(HaveOccurred())
		Expect(outputYaml).NotTo(BeEmpty())
		Expect(outputYaml).To(ContainSubstring("name: ingress-proxy\n"))
	})

	It("should contain 'knative-external-proxy' for knative dry run with enterprise helm chart override", func() {
		outputYaml, err := testutils.GlooctlOut(fmt.Sprintf("install knative --file https://storage.googleapis.com/gloo-ee-helm/charts/gloo-ee-1.0.0-rc8.tgz --dry-run"))
		Expect(err).NotTo(HaveOccurred())
		Expect(outputYaml).NotTo(BeEmpty())
		Expect(outputYaml).To(ContainSubstring("name: knative-external-proxy\n"))
	})

	It("should contain base64 encoding of license key for gateway enterprise dry run with license-key flag", func() {
		outputYaml, err := testutils.GlooctlOut(fmt.Sprintf("install gateway enterprise --dry-run %s", licenseKey))
		Expect(err).NotTo(HaveOccurred())
		Expect(outputYaml).NotTo(BeEmpty())
		Expect(outputYaml).To(ContainSubstring("license-key: \"ZmFrZS1saWNlbnNlLWtleQ==\"\n"))
	})

	It("should not contain license key for gateway enterprise dry run with open-source chart override", func() {
		outputYaml, err := testutils.GlooctlOut(fmt.Sprintf("install gateway enterprise --file %s --dry-run %s", file, licenseKey))
		Expect(err).NotTo(HaveOccurred())
		Expect(outputYaml).NotTo(BeEmpty())
		Expect(outputYaml).NotTo(ContainSubstring("license-key"))
	})

	It("should not contain license key for gateway enterprise dry run with open-source chart override", func() {
		outputYaml, err := testutils.GlooctlOut(fmt.Sprintf("install gateway enterprise --file %s --with-gloo-fed=false --dry-run %s", file, licenseKey))
		Expect(err).NotTo(HaveOccurred())
		Expect(outputYaml).NotTo(BeEmpty())
		Expect(outputYaml).NotTo(ContainSubstring("license-key"))
	})

	It("shouldn't get errors for enterprise dry run with multiple values", func() {
		outputYaml, err := testutils.GlooctlOut(fmt.Sprintf("install gateway enterprise --file %s --dry-run --values %s,%s %s", file, values1, values2, licenseKey))
		Expect(err).NotTo(HaveOccurred())
		// Test that the values are being merged as we expect
		Expect(outputYaml).To(ContainSubstring("test-namespace-2\n"))
	})

	It("should error when not overriding helm chart in dev mode", func() {
		_, err := testutils.GlooctlOut("install ingress --dry-run")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("installing gloo edge in ingress mode: you must provide a Gloo Helm chart URI via the 'file' option when running an unreleased version of glooctl"))
	})

	It("should error when not providing file with valid extension", func() {
		_, err := testutils.GlooctlOut("install gateway --file foo --dry-run")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("installing gloo edge in gateway mode: unsupported file extension for Helm chart URI: [foo]. Extension must either be .tgz or .tar.gz"))
	})

	It("should error when not providing valid file", func() {
		_, err := testutils.GlooctlOut("install gateway --file foo.tgz --dry-run")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("no such file or directory"))
	})

	It("should not error when providing a new release-name flag value", func() {
		out, err := testutils.GlooctlOut(fmt.Sprintf("install gateway --file %s --release-name test --dry-run", file))
		Expect(err).NotTo(HaveOccurred())
		Expect(out).NotTo(BeEmpty())
	})

})

package helm_test

import (
	"path/filepath"

	"github.com/solo-io/skv2/codegen/util"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/version"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/test/kube2e"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)

var _ = Describe("Kube2e: helm", func() {

	var (
		chartUri           string
		rlcCrdName         = "ratelimitconfigs.ratelimit.solo.io"
		rlcCrdTemplateName = filepath.Join(util.GetModuleRoot(), "install", "helm", "gloo", "crds", "ratelimit_config.yaml")
	)

	It("uses helm to upgrade to this gloo version without errors", func() {

		By("should start with gloo version 1.3.0")
		Expect(GetGlooServerVersion(testHelper.InstallNamespace)).To(Equal("1.3.0"))

		By("apply new `RateLimitConfig` CRD")
		runAndCleanCommand("kubectl", "apply", "-f", rlcCrdTemplateName)
		Eventually(func() string {
			outputBytes := runAndCleanCommand("kubectl", "get", "crd", rlcCrdName)
			return string(outputBytes)
		}, "5s", "1s").Should(ContainSubstring(rlcCrdName))

		// upgrade to the gloo version being tested
		chartUri = filepath.Join("../../..", testHelper.TestAssetDir, testHelper.HelmChartName+"-"+testHelper.ChartVersion()+".tgz")
		runAndCleanCommand("helm", "upgrade", "gloo", chartUri,
			"-n", testHelper.InstallNamespace)

		By("should have upgraded to the gloo version being tested")
		Expect(GetGlooServerVersion(testHelper.InstallNamespace)).To(Equal(testHelper.ChartVersion()))

		kube2e.GlooctlCheckEventuallyHealthy(1, testHelper, "180s")
	})

	It("uses helm to update the settings without errors", func() {

		By("should start with the default settings.invalidConfigPolicy.invalidRouteResponseCode=404")
		client := helpers.MustSettingsClient()
		settings, err := client.Read(testHelper.InstallNamespace, defaults.SettingsName, clients.ReadOpts{})
		Expect(err).To(BeNil())
		Expect(settings.GetGloo().GetInvalidConfigPolicy().GetInvalidRouteResponseCode()).To(Equal(uint32(404)))

		// update the settings with `helm upgrade` (without updating the gloo version)
		if chartUri == "" { // hasn't yet upgraded to the chart being tested- use regular gloo/gloo chart
			runAndCleanCommand("helm", "upgrade", "gloo", "gloo/gloo",
				"-n", testHelper.InstallNamespace,
				"--set", "settings.invalidConfigPolicy.invalidRouteResponseCode=400",
				"--version", GetGlooServerVersion(testHelper.InstallNamespace))
		} else { // has already upgraded to the chart being tested- use it
			runAndCleanCommand("helm", "upgrade", "gloo", chartUri,
				"-n", testHelper.InstallNamespace,
				"--set", "settings.invalidConfigPolicy.invalidRouteResponseCode=400")
		}

		By("should have updated to settings.invalidConfigPolicy.invalidRouteResponseCode=400")
		settings, err = client.Read(testHelper.InstallNamespace, defaults.SettingsName, clients.ReadOpts{})
		Expect(err).To(BeNil())
		Expect(settings.GetGloo().GetInvalidConfigPolicy().GetInvalidRouteResponseCode()).To(Equal(uint32(400)))

		kube2e.GlooctlCheckEventuallyHealthy(1, testHelper, "90s")
	})

})

func GetGlooServerVersion(namespace string) (v string) {
	glooVersion, err := version.GetClientServerVersions(version.NewKube(namespace))
	Expect(err).To(BeNil())
	Expect(len(glooVersion.GetServer())).To(Equal(1))
	for _, container := range glooVersion.GetServer()[0].GetKubernetes().GetContainers() {
		if v == "" {
			v = container.Tag
		} else {
			Expect(container.Tag).To(Equal(v))
		}
	}
	return v
}

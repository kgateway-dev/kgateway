package helm_test

import (
	"context"
	"os"
	"path/filepath"

	"github.com/solo-io/go-utils/testutils/exec"
	"github.com/solo-io/solo-kit/pkg/code-generator/schemagen"
	v1beta12 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"

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
		crdDir             = filepath.Join(util.GetModuleRoot(), "install", "helm", "gloo", "crds")
		chartUri           string
		rlcCrdName         = "ratelimitconfigs.ratelimit.solo.io"
		rlcCrdTemplateName = filepath.Join(crdDir, "ratelimit_config.yaml")
		vhoCrdName         = "virtualhostoptions.gateway.solo.io"
		vhoCrdTemplateName = filepath.Join(crdDir, "gateway.solo.io_v1_VirtualHostOption.yaml")
		rtoCrdName         = "routeoptions.gateway.solo.io"
		rtoCrdTemplateName = filepath.Join(crdDir, "gateway.solo.io_v1_RouteOption.yaml")

		ctx    context.Context
		cancel context.CancelFunc
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
	})

	AfterEach(func() { cancel() })

	It("uses helm to upgrade to this gloo version without errors", func() {

		By("should start with gloo version 1.3.0")
		Expect(GetGlooServerVersion(ctx, testHelper.InstallNamespace)).To(Equal("1.3.0"))

		By("apply new `RateLimitConfig` CRD")
		runAndCleanCommand("kubectl", "apply", "-f", rlcCrdTemplateName)
		Eventually(func() string {
			outputBytes := runAndCleanCommand("kubectl", "get", "crd", rlcCrdName)
			return string(outputBytes)
		}, "5s", "1s").Should(ContainSubstring(rlcCrdName))

		By("apply new `VirtualHostOption` CRD")
		runAndCleanCommand("kubectl", "apply", "-f", vhoCrdTemplateName)
		Eventually(func() string {
			outputBytes := runAndCleanCommand("kubectl", "get", "crd", vhoCrdName)
			return string(outputBytes)
		}, "5s", "1s").Should(ContainSubstring(vhoCrdName))

		By("apply new `RouteOption` CRD")
		runAndCleanCommand("kubectl", "apply", "-f", rtoCrdTemplateName)
		Eventually(func() string {
			outputBytes := runAndCleanCommand("kubectl", "get", "crd", rtoCrdName)
			return string(outputBytes)
		}, "5s", "1s").Should(ContainSubstring(rtoCrdName))

		// upgrade to the gloo version being tested
		chartUri = filepath.Join("../../..", testHelper.TestAssetDir, testHelper.HelmChartName+"-"+testHelper.ChartVersion()+".tgz")
		runAndCleanCommand("helm", "upgrade", "gloo", chartUri,
			"-n", testHelper.InstallNamespace)

		By("should have upgraded to the gloo version being tested")
		Expect(GetGlooServerVersion(ctx, testHelper.InstallNamespace)).To(Equal(testHelper.ChartVersion()))

		kube2e.GlooctlCheckEventuallyHealthy(1, testHelper, "180s")
	})

	It("uses helm to update the settings without errors", func() {

		By("should start with the settings.invalidConfigPolicy.invalidRouteResponseCode=404")
		client := helpers.MustSettingsClient(ctx)
		settings, err := client.Read(testHelper.InstallNamespace, defaults.SettingsName, clients.ReadOpts{})
		Expect(err).To(BeNil())
		Expect(settings.GetGloo().GetInvalidConfigPolicy().GetInvalidRouteResponseCode()).To(Equal(uint32(404)))

		// following logic handles chartUri for focused test
		// update the settings with `helm upgrade` (without updating the gloo version)
		if chartUri == "" { // hasn't yet upgraded to the chart being tested- use regular gloo/gloo chart
			runAndCleanCommand("helm", "upgrade", "gloo", "gloo/gloo",
				"-n", testHelper.InstallNamespace,
				"--set", "settings.replaceInvalidRoutes=true",
				"--set", "settings.invalidConfigPolicy.invalidRouteResponseCode=400",
				"--version", GetGlooServerVersion(ctx, testHelper.InstallNamespace))
		} else { // has already upgraded to the chart being tested- use it
			runAndCleanCommand("helm", "upgrade", "gloo", chartUri,
				"-n", testHelper.InstallNamespace,
				"--set", "settings.replaceInvalidRoutes=true",
				"--set", "settings.invalidConfigPolicy.invalidRouteResponseCode=400")
		}

		By("should have updated to settings.invalidConfigPolicy.invalidRouteResponseCode=400")
		settings, err = client.Read(testHelper.InstallNamespace, defaults.SettingsName, clients.ReadOpts{})
		Expect(err).To(BeNil())
		Expect(settings.GetGloo().GetInvalidConfigPolicy().GetInvalidRouteResponseCode()).To(Equal(uint32(400)))

		kube2e.GlooctlCheckEventuallyHealthy(1, testHelper, "90s")
	})

	Context("applies all CRD manifests without an error", func() {

		var crdsByFileName = map[string]v1beta12.CustomResourceDefinition{}

		BeforeEach(func() {
			err := filepath.Walk(crdDir, func(crdFile string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if info.IsDir() {
					return nil
				}

				// Parse the file, and extract the CRD
				crd, err := schemagen.GetCRDFromFile(crdFile)
				if err != nil {
					return err
				}
				crdsByFileName[crdFile] = crd

				// continue traversing
				return nil
			})
			Expect(err).NotTo(HaveOccurred())
		})

		It("works using kubectl apply", func() {
			for crdFile, crd := range crdsByFileName {
				// Apply the CRD
				err := exec.RunCommand(testHelper.RootDir, false, "kubectl", "apply", "-f", crdFile)
				Expect(err).NotTo(HaveOccurred(), "should be able to kubectl apply -f %s", crdFile)

				// Ensure the CRD is eventually accepted
				Eventually(func() (string, error) {
					return exec.RunCommandOutput(testHelper.RootDir, false, "kubectl", "get", "crd", crd.GetName())
				}, "10s", "1s").Should(ContainSubstring(crd.GetName()))
			}
		})
	})

})

func GetGlooServerVersion(ctx context.Context, namespace string) (v string) {
	glooVersion, err := version.GetClientServerVersions(ctx, version.NewKube(namespace))
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

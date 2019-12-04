package install_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/cliutil/helm"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/install"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"helm.sh/helm/v3/pkg/chartutil"
)

var _ = Describe("Install", func() {

	//var (
	//	installer install.GlooStagedInstaller
	//	opts      options.Options
	//	validator MockInstallClient
	//)
	//
	//BeforeEach(func() {
	//	opts.Install.Namespace = "gloo-system"
	//	opts.Install.HelmChartOverride = file
	//})
	//
	//expectKinds := func(resources []install2.ResourceType, kinds []string) {
	//	for _, resource := range resources {
	//		ExpectWithOffset(1, kinds).To(ContainElement(resource.Kind))
	//	}
	//}
	//
	//expectNames := func(resources []install2.ResourceType, names []string) {
	//	for _, resource := range resources {
	//		ExpectWithOffset(1, names).To(ContainElement(resource.Metadata.Name))
	//	}
	//}
	//
	//expectLabels := func(resources []install2.ResourceType, labels map[string]string) {
	//	for _, resource := range resources {
	//		actualLabels := resource.Metadata.Labels
	//		for k, v := range labels {
	//			val, ok := actualLabels[k]
	//			ExpectWithOffset(1, ok).To(BeTrue())
	//			ExpectWithOffset(1, v).To(BeEquivalentTo(val))
	//		}
	//	}
	//}
	//
	//withSettings := func(kinds []string) []string {
	//	// default knative values create Settings
	//	kindsWithSettings := make([]string, len(kinds))
	//	for _, kind := range kinds {
	//		kindsWithSettings = append(kindsWithSettings, kind)
	//	}
	//	kindsWithSettings = append(kindsWithSettings, "Settings")
	//
	//	return kindsWithSettings
	//}

	FIt("install", func() {

		val, err := chartutil.ReadValues([]byte(`
global:
  glooRbac:
    nameSuffix: asdf
`))
		Expect(err).NotTo(HaveOccurred())

		err = install.Install(&options.Install{
			DryRun:                  true,
			Namespace:               defaults.GlooSystem,
			HelmChartOverride:       "/Users/marco/code/projects/helm3/gloo-1.1.0.tgz",
			HelmChartValueFileNames: []string{"/Users/marco/code/projects/helm3/values.yaml"},
		}, val, false, false)
		Expect(err).NotTo(HaveOccurred())
	})

	It("uninstall", func() {

		uninstallAction, err := helm.NewUninstall("gloo-system")
		Expect(err).NotTo(HaveOccurred())

		rel, err := uninstallAction.Run(constants.GlooReleaseName)
		Expect(err).NotTo(HaveOccurred())
		Expect(rel).NotTo(BeNil())
	})

	//Context("Gateway with default values", func() {
	//	BeforeEach(func() {
	//		spec, err := install.GetInstallSpec(&opts, constants.GatewayValuesFileName)
	//		Expect(err).NotTo(HaveOccurred())
	//		validator = MockInstallClient{
	//			expectedCrds: install.GlooCrdNames,
	//		}
	//		installer, err = install.NewGlooStagedInstaller(&opts, *spec, &validator)
	//		Expect(err).NotTo(HaveOccurred())
	//	})
	//
	//	It("installs expected crds for gloo", func() {
	//		err := installer.DoCrdInstall()
	//		Expect(err).NotTo(HaveOccurred())
	//		Expect(validator.applied).To(BeTrue())
	//		Expect(validator.waited).To(BeTrue())
	//		expectKinds(validator.resources, []string{"CustomResourceDefinition"})
	//		expectNames(validator.resources, install.GlooCrdNames)
	//	})
	//
	//	It("does nothing on preinstall", func() {
	//		err := installer.DoPreInstall()
	//		Expect(err).NotTo(HaveOccurred())
	//		Expect(validator.applied).To(BeTrue())
	//		Expect(validator.waited).To(BeFalse())
	//		expectKinds(validator.resources, install.GlooPreInstallKinds)
	//		expectLabels(validator.resources, install.ExpectedLabels)
	//	})
	//
	//	It("installs expected kinds for gloo", func() {
	//		err := installer.DoInstall()
	//		Expect(err).NotTo(HaveOccurred())
	//		Expect(validator.applied).To(BeTrue())
	//		Expect(validator.waited).To(BeFalse())
	//		expectKinds(validator.resources, install.GlooInstallKinds)
	//		expectLabels(validator.resources, install.ExpectedLabels)
	//	})
	//
	//})
	//
	//Context("Gateway with default values and upgrade option", func() {
	//	BeforeEach(func() {
	//		opts.Install.Upgrade = true
	//		spec, err := install.GetInstallSpec(&opts, constants.GatewayValuesFileName)
	//		Expect(err).NotTo(HaveOccurred())
	//		validator = MockInstallClient{
	//			expectedCrds: install.GlooCrdNames,
	//		}
	//		installer, err = install.NewGlooStagedInstaller(&opts, *spec, &validator)
	//		Expect(err).NotTo(HaveOccurred())
	//	})
	//
	//	It("installs expected crds for gloo", func() {
	//		err := installer.DoCrdInstall()
	//		Expect(err).NotTo(HaveOccurred())
	//		Expect(validator.applied).To(BeTrue())
	//		Expect(validator.waited).To(BeTrue())
	//		expectKinds(validator.resources, []string{"CustomResourceDefinition"})
	//		expectNames(validator.resources, install.GlooCrdNames)
	//	})
	//
	//	It("does nothing on preinstall", func() {
	//		err := installer.DoPreInstall()
	//		Expect(err).NotTo(HaveOccurred())
	//		Expect(validator.applied).To(BeTrue())
	//		Expect(validator.waited).To(BeFalse())
	//		expectKinds(validator.resources, install.GlooPreInstallKinds)
	//		expectLabels(validator.resources, install.ExpectedLabels)
	//	})
	//
	//	It("installs expected kinds for gloo", func() {
	//		err := installer.DoInstall()
	//		Expect(err).NotTo(HaveOccurred())
	//		Expect(validator.applied).To(BeTrue())
	//		Expect(validator.waited).To(BeFalse())
	//		expectKinds(validator.resources, install.GlooGatewayUpgradeKinds)
	//		expectLabels(validator.resources, install.ExpectedLabels)
	//	})
	//
	//})
	//
	//Context("Ingress with default values", func() {
	//	BeforeEach(func() {
	//		spec, err := install.GetInstallSpec(&opts, constants.IngressValuesFileName)
	//		Expect(err).NotTo(HaveOccurred())
	//		validator = MockInstallClient{
	//			expectedCrds: install.GlooCrdNames,
	//		}
	//		installer, err = install.NewGlooStagedInstaller(&opts, *spec, &validator)
	//		Expect(err).NotTo(HaveOccurred())
	//	})
	//
	//	It("installs expected crds for gloo", func() {
	//		err := installer.DoCrdInstall()
	//		Expect(err).NotTo(HaveOccurred())
	//		Expect(validator.applied).To(BeTrue())
	//		Expect(validator.waited).To(BeTrue())
	//		expectKinds(validator.resources, []string{"CustomResourceDefinition"})
	//		expectNames(validator.resources, install.GlooCrdNames)
	//	})
	//
	//	It("does nothing on preinstall", func() {
	//		err := installer.DoPreInstall()
	//		Expect(err).NotTo(HaveOccurred())
	//		Expect(validator.applied).To(BeTrue())
	//		Expect(validator.waited).To(BeFalse())
	//		expectKinds(validator.resources, install.GlooPreInstallKinds)
	//		expectLabels(validator.resources, install.ExpectedLabels)
	//	})
	//
	//	It("installs expected kinds for gloo", func() {
	//		err := installer.DoInstall()
	//		Expect(err).NotTo(HaveOccurred())
	//		Expect(validator.applied).To(BeTrue())
	//		Expect(validator.waited).To(BeFalse())
	//		expectKinds(validator.resources, install.GlooInstallKinds)
	//		expectLabels(validator.resources, install.ExpectedLabels)
	//	})
	//
	//})
	//
	//Context("Knative with default values and no previous knative", func() {
	//
	//	BeforeEach(func() {
	//		spec, err := install.GetInstallSpec(&opts, constants.KnativeValuesFileName)
	//		Expect(err).NotTo(HaveOccurred())
	//		validator = MockInstallClient{
	//			expectedCrds: install.GlooCrdNames,
	//		}
	//		installer, err = install.NewGlooStagedInstaller(&opts, *spec, &validator)
	//		Expect(err).NotTo(HaveOccurred())
	//	})
	//
	//	It("installs all crds", func() {
	//		err := installer.DoCrdInstall()
	//		Expect(err).NotTo(HaveOccurred())
	//		Expect(validator.applied).To(BeTrue())
	//		Expect(validator.waited).To(BeTrue())
	//		expectKinds(validator.resources, []string{"CustomResourceDefinition"})
	//		expectNames(validator.resources, install.GlooCrdNames)
	//	})
	//
	//	It("does nothing on preinstall", func() {
	//		err := installer.DoPreInstall()
	//		Expect(err).NotTo(HaveOccurred())
	//		Expect(validator.applied).To(BeTrue())
	//		Expect(validator.waited).To(BeFalse())
	//		expectKinds(validator.resources, append([]string{"Settings"}, install.GlooPreInstallKinds...))
	//		expectLabels(validator.resources, install.ExpectedLabels)
	//	})
	//
	//	It("installs expected kinds for gloo", func() {
	//		err := installer.DoInstall()
	//		Expect(err).NotTo(HaveOccurred())
	//		Expect(validator.applied).To(BeTrue())
	//		Expect(validator.waited).To(BeFalse())
	//
	//		expectKinds(validator.resources, withSettings(install.GlooInstallKinds))
	//		expectLabels(validator.resources, install.ExpectedLabels)
	//	})
	//
	//})
	//
	//Context("Knative with default values and previous knative (ours)", func() {
	//
	//	BeforeEach(func() {
	//		spec, err := install.GetInstallSpec(&opts, constants.KnativeValuesFileName)
	//		Expect(err).NotTo(HaveOccurred())
	//		validator = MockInstallClient{
	//			expectedCrds:     install.GlooCrdNames,
	//			knativeInstalled: true,
	//			knativeOurs:      true,
	//		}
	//		installer, err = install.NewGlooStagedInstaller(&opts, *spec, &validator)
	//		Expect(err).NotTo(HaveOccurred())
	//	})
	//
	//	It("installs gloo crds only", func() {
	//		err := installer.DoCrdInstall()
	//		Expect(err).NotTo(HaveOccurred())
	//		Expect(validator.applied).To(BeTrue())
	//		Expect(validator.waited).To(BeTrue())
	//		expectKinds(validator.resources, []string{"CustomResourceDefinition"})
	//		expectNames(validator.resources, install.GlooCrdNames)
	//	})
	//
	//	It("does nothing on preinstall", func() {
	//		err := installer.DoPreInstall()
	//		Expect(err).NotTo(HaveOccurred())
	//		Expect(validator.applied).To(BeTrue())
	//		Expect(validator.waited).To(BeFalse())
	//		expectKinds(validator.resources, append([]string{"Settings"}, install.GlooPreInstallKinds...))
	//		expectLabels(validator.resources, install.ExpectedLabels)
	//	})
	//
	//	It("installs expected kinds for gloo", func() {
	//		err := installer.DoInstall()
	//		Expect(err).NotTo(HaveOccurred())
	//		Expect(validator.applied).To(BeTrue())
	//		Expect(validator.waited).To(BeFalse())
	//		expectKinds(validator.resources, withSettings(install.GlooInstallKinds))
	//		expectLabels(validator.resources, install.ExpectedLabels)
	//	})
	//
	//})
	//
	//Context("Knative with default values and previous knative (not ours)", func() {
	//
	//	BeforeEach(func() {
	//		spec, err := install.GetInstallSpec(&opts, constants.KnativeValuesFileName)
	//		Expect(err).NotTo(HaveOccurred())
	//		validator = MockInstallClient{
	//			expectedCrds:     install.GlooCrdNames,
	//			knativeInstalled: true,
	//		}
	//		installer, err = install.NewGlooStagedInstaller(&opts, *spec, &validator)
	//		Expect(err).NotTo(HaveOccurred())
	//	})
	//
	//	It("installs gloo crds only", func() {
	//		err := installer.DoCrdInstall()
	//		Expect(err).NotTo(HaveOccurred())
	//		Expect(validator.applied).To(BeTrue())
	//		Expect(validator.waited).To(BeTrue())
	//		expectKinds(validator.resources, []string{"CustomResourceDefinition"})
	//		expectNames(validator.resources, install.GlooCrdNames)
	//	})
	//
	//	It("does nothing on preinstall", func() {
	//		err := installer.DoPreInstall()
	//		Expect(err).NotTo(HaveOccurred())
	//		Expect(validator.applied).To(BeTrue())
	//		Expect(validator.waited).To(BeFalse())
	//		expectKinds(validator.resources, append([]string{"Settings"}, install.GlooPreInstallKinds...))
	//		expectLabels(validator.resources, install.ExpectedLabels)
	//	})
	//
	//	It("installs expected kinds for gloo", func() {
	//		err := installer.DoInstall()
	//		Expect(err).NotTo(HaveOccurred())
	//		Expect(validator.applied).To(BeTrue())
	//		Expect(validator.waited).To(BeFalse())
	//		expectKinds(validator.resources, withSettings(install.GlooInstallKinds))
	//		expectLabels(validator.resources, install.ExpectedLabels)
	//	})
	//
	//})
	//
	//Context("Enterprise Gateway NamespacedGlooKubeInstallClient", func() {
	//	var (
	//		kubectlCmd        string
	//		kubeInstallClient install.NamespacedGlooKubeInstallClient
	//	)
	//	BeforeEach(func() {
	//
	//		MockKubectl := func(stdin io.Reader, args ...string) error {
	//			kubectl := exec.Command("kubectl", args...)
	//			kubectlCmd = fmt.Sprintf("running kubectl command: %v\n", kubectl.Args)
	//			return nil
	//		}
	//
	//		opts.Install.Namespace = "gloo-system-test"
	//		kubeInstallClient = install.NamespacedGlooKubeInstallClient{
	//			Namespace: opts.Install.Namespace,
	//			Delegate:  &MockInstallClient{},
	//			Executor:  MockKubectl,
	//		}
	//	})
	//
	//	It("ensure namespace argument is passed into kubectl apply", func() {
	//		err := kubeInstallClient.KubectlApply([]byte{})
	//		Expect(err).NotTo(HaveOccurred())
	//		Expect(kubectlCmd).To(Equal("running kubectl command: [kubectl apply -n gloo-system-test -f -]\n"))
	//	})
	//
	//})
})

package install_test

import (
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	install2 "github.com/solo-io/gloo/pkg/cliutil/install"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/install"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
)

type MockInstallClient struct {
	expectedCrds     []string
	applied          bool
	waited           bool
	resources        []install2.ResourceType
	knativeInstalled bool
	knativeOurs      bool
}

func (i *MockInstallClient) KubectlApply(manifest []byte) error {
	Expect(i.applied).To(BeFalse())
	i.applied = true
	resources, err := install2.GetResources(string(manifest))
	Expect(err).NotTo(HaveOccurred())
	i.resources = resources
	return nil
}

func (i *MockInstallClient) WaitForCrdsToBeRegistered(crds []string, timeout, interval time.Duration) error {
	Expect(i.waited).To(BeFalse())
	i.waited = true
	Expect(crds).To(ConsistOf(i.expectedCrds))
	return nil
}

func (i *MockInstallClient) CheckKnativeInstallation() (bool, bool, error) {
	return i.knativeInstalled, i.knativeOurs, nil
}

var _ = Describe("Install", func() {

	var (
		file      string
		installer install.GlooStagedInstaller
		opts      options.Options
		validator MockInstallClient
	)

	BeforeEach(func() {
		file = filepath.Join(RootDir, "_test/gloo-test-unit-testing.tgz")
		opts.Install.Namespace = "gloo-system"
		opts.Install.HelmChartOverride = file
	})

	expectKinds := func(resources []install2.ResourceType, kinds []string) {
		for _, resource := range resources {
			Expect(kinds).To(ContainElement(resource.Kind))
		}
	}

	expectNames := func(resources []install2.ResourceType, names []string) {
		for _, resource := range resources {
			Expect(names).To(ContainElement(resource.Metadata.Name))
		}
	}

	expectLabels := func(resources []install2.ResourceType, labels map[string]string) {
		for _, resource := range resources {
			actualLabels := resource.Metadata.Labels
			for k, v := range labels {
				val, ok := actualLabels[k]
				Expect(ok).To(BeTrue())
				Expect(v).To(BeEquivalentTo(val))
			}
		}
	}

	expectNamespace := func(resources []install2.ResourceType, namespace string) {
		for _, resource := range resources {
			if resource.Metadata.Namespace != "" {
				Expect(resource.Metadata.Namespace).To(BeEquivalentTo(namespace))
			}
		}
	}

	Context("Gateway with default values", func() {
		BeforeEach(func() {
			spec, err := install.GetInstallSpec(&opts, constants.GatewayValuesFileName)
			Expect(err).NotTo(HaveOccurred())
			validator = MockInstallClient{
				expectedCrds: install.GlooCrdNames,
			}
			installer, err = install.NewGlooStagedInstaller(&opts, *spec, &validator)
			Expect(err).NotTo(HaveOccurred())
		})

		It("installs expected crds for gloo", func() {
			err := installer.DoCrdInstall()
			Expect(err).NotTo(HaveOccurred())
			Expect(validator.applied).To(BeTrue())
			Expect(validator.waited).To(BeTrue())
			expectKinds(validator.resources, []string{"CustomResourceDefinition"})
			expectNames(validator.resources, install.GlooCrdNames)
		})

		It("does nothing on preinstall", func() {
			err := installer.DoPreInstall()
			Expect(err).NotTo(HaveOccurred())
			Expect(validator.applied).To(BeFalse())
			Expect(validator.waited).To(BeFalse())
			Expect(validator.resources).To(BeEmpty())
		})

		It("installs expected kinds for gloo", func() {
			err := installer.DoInstall()
			Expect(err).NotTo(HaveOccurred())
			Expect(validator.applied).To(BeTrue())
			Expect(validator.waited).To(BeFalse())
			expectKinds(validator.resources, install.GlooInstallKinds)
			expectLabels(validator.resources, install.ExpectedLabels)
		})

		It("skips knative install", func() {
			err := installer.DoKnativeInstall()
			Expect(err).NotTo(HaveOccurred())
			Expect(validator.applied).To(BeFalse())
			Expect(validator.waited).To(BeFalse())
		})
	})

	Context("Ingress with default values", func() {
		BeforeEach(func() {
			spec, err := install.GetInstallSpec(&opts, constants.IngressValuesFileName)
			Expect(err).NotTo(HaveOccurred())
			validator = MockInstallClient{
				expectedCrds: install.GlooCrdNames,
			}
			installer, err = install.NewGlooStagedInstaller(&opts, *spec, &validator)
			Expect(err).NotTo(HaveOccurred())
		})

		It("installs expected crds for gloo", func() {
			err := installer.DoCrdInstall()
			Expect(err).NotTo(HaveOccurred())
			Expect(validator.applied).To(BeTrue())
			Expect(validator.waited).To(BeTrue())
			expectKinds(validator.resources, []string{"CustomResourceDefinition"})
			expectNames(validator.resources, install.GlooCrdNames)
		})

		It("does nothing on preinstall", func() {
			err := installer.DoPreInstall()
			Expect(err).NotTo(HaveOccurred())
			Expect(validator.applied).To(BeFalse())
			Expect(validator.waited).To(BeFalse())
			Expect(validator.resources).To(BeEmpty())
		})

		It("installs expected kinds for gloo", func() {
			err := installer.DoInstall()
			Expect(err).NotTo(HaveOccurred())
			Expect(validator.applied).To(BeTrue())
			Expect(validator.waited).To(BeFalse())
			expectKinds(validator.resources, install.GlooInstallKinds)
			expectLabels(validator.resources, install.ExpectedLabels)
		})

		It("skips knative install", func() {
			err := installer.DoKnativeInstall()
			Expect(err).NotTo(HaveOccurred())
			Expect(validator.applied).To(BeFalse())
			Expect(validator.waited).To(BeFalse())
		})
	})

	Context("Knative with default values and no previous knative", func() {

		allCrds := append(install.GlooCrdNames, install.KnativeCrdNames...)

		BeforeEach(func() {
			spec, err := install.GetInstallSpec(&opts, constants.KnativeValuesFileName)
			Expect(err).NotTo(HaveOccurred())
			validator = MockInstallClient{
				expectedCrds: allCrds,
			}
			installer, err = install.NewGlooStagedInstaller(&opts, *spec, &validator)
			Expect(err).NotTo(HaveOccurred())
		})

		It("installs all crds", func() {
			err := installer.DoCrdInstall()
			Expect(err).NotTo(HaveOccurred())
			Expect(validator.applied).To(BeTrue())
			Expect(validator.waited).To(BeTrue())
			expectKinds(validator.resources, []string{"CustomResourceDefinition"})
			expectNames(validator.resources, allCrds)
		})

		It("does nothing on preinstall", func() {
			err := installer.DoPreInstall()
			Expect(err).NotTo(HaveOccurred())
			Expect(validator.applied).To(BeFalse())
			Expect(validator.waited).To(BeFalse())
			Expect(validator.resources).To(BeEmpty())
		})

		It("installs expected kinds for gloo", func() {
			err := installer.DoInstall()
			Expect(err).NotTo(HaveOccurred())
			Expect(validator.applied).To(BeTrue())
			Expect(validator.waited).To(BeFalse())
			expectKinds(validator.resources, install.GlooInstallKinds)
			expectLabels(validator.resources, install.ExpectedLabels)
		})

		It("does knative install when not already installed", func() {
			err := installer.DoKnativeInstall()
			Expect(err).NotTo(HaveOccurred())
			Expect(validator.applied).To(BeTrue())
			Expect(validator.waited).To(BeFalse())
			expectNamespace(validator.resources, "knative-serving")
		})
	})

	Context("Knative with default values and previous knative (ours)", func() {

		BeforeEach(func() {
			spec, err := install.GetInstallSpec(&opts, constants.KnativeValuesFileName)
			Expect(err).NotTo(HaveOccurred())
			validator = MockInstallClient{
				expectedCrds:     install.GlooCrdNames,
				knativeInstalled: true,
				knativeOurs:      true,
			}
			installer, err = install.NewGlooStagedInstaller(&opts, *spec, &validator)
			Expect(err).NotTo(HaveOccurred())
		})

		It("installs gloo crds only", func() {
			err := installer.DoCrdInstall()
			Expect(err).NotTo(HaveOccurred())
			Expect(validator.applied).To(BeTrue())
			Expect(validator.waited).To(BeTrue())
			expectKinds(validator.resources, []string{"CustomResourceDefinition"})
			expectNames(validator.resources, install.GlooCrdNames)
		})

		It("does nothing on preinstall", func() {
			err := installer.DoPreInstall()
			Expect(err).NotTo(HaveOccurred())
			Expect(validator.applied).To(BeFalse())
			Expect(validator.waited).To(BeFalse())
			Expect(validator.resources).To(BeEmpty())
		})

		It("installs expected kinds for gloo", func() {
			err := installer.DoInstall()
			Expect(err).NotTo(HaveOccurred())
			Expect(validator.applied).To(BeTrue())
			Expect(validator.waited).To(BeFalse())
			expectKinds(validator.resources, install.GlooInstallKinds)
			expectLabels(validator.resources, install.ExpectedLabels)
		})

		It("does apply knative", func() {
			err := installer.DoKnativeInstall()
			Expect(err).NotTo(HaveOccurred())
			Expect(validator.applied).To(BeTrue())
			Expect(validator.waited).To(BeFalse())
			expectNamespace(validator.resources, "knative-serving")
		})
	})

	Context("Knative with default values and previous knative (not ours)", func() {

		BeforeEach(func() {
			spec, err := install.GetInstallSpec(&opts, constants.KnativeValuesFileName)
			Expect(err).NotTo(HaveOccurred())
			validator = MockInstallClient{
				expectedCrds:     install.GlooCrdNames,
				knativeInstalled: true,
			}
			installer, err = install.NewGlooStagedInstaller(&opts, *spec, &validator)
			Expect(err).NotTo(HaveOccurred())
		})

		It("installs gloo crds only", func() {
			err := installer.DoCrdInstall()
			Expect(err).NotTo(HaveOccurred())
			Expect(validator.applied).To(BeTrue())
			Expect(validator.waited).To(BeTrue())
			expectKinds(validator.resources, []string{"CustomResourceDefinition"})
			expectNames(validator.resources, install.GlooCrdNames)
		})

		It("does nothing on preinstall", func() {
			err := installer.DoPreInstall()
			Expect(err).NotTo(HaveOccurred())
			Expect(validator.applied).To(BeFalse())
			Expect(validator.waited).To(BeFalse())
			Expect(validator.resources).To(BeEmpty())
		})

		It("installs expected kinds for gloo", func() {
			err := installer.DoInstall()
			Expect(err).NotTo(HaveOccurred())
			Expect(validator.applied).To(BeTrue())
			Expect(validator.waited).To(BeFalse())
			expectKinds(validator.resources, install.GlooInstallKinds)
			expectLabels(validator.resources, install.ExpectedLabels)
		})

		It("does nothing on knative install", func() {
			err := installer.DoKnativeInstall()
			Expect(err).NotTo(HaveOccurred())
			Expect(validator.applied).To(BeFalse())
			Expect(validator.waited).To(BeFalse())
			Expect(validator.resources).To(BeEmpty())
		})
	})
})

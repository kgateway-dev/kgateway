package glooupgrade_test

import (
	"context"
	"github.com/solo-io/gloo/test/kube2e/upgrades"
	"github.com/solo-io/k8s-utils/kubeutils"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"os"
	"path/filepath"

	"github.com/solo-io/skv2/codegen/util"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/go-utils/versionutils"
	"github.com/solo-io/k8s-utils/testutils/helper"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)

const namespace = defaults.GlooSystem

var _ = Describe("Kube2e: Upgrade Tests", func() {

	var (
		crdDir     string
		chartUri   string
		ctx        context.Context
		cancel     context.CancelFunc
		testHelper *helper.SoloTestHelper

		// whether to set validation webhook's failurePolicy=Fail
		strictValidation bool

		// Versions to upgrade from
		// ex: current branch is 1.13.10 - this would be the latest patch release of 1.12
		LastPatchMostRecentMinorVersion *versionutils.Version

		// ex:current branch is 1.13.10 - this would be 1.13.9
		CurrentPatchMostRecentMinorVersion *versionutils.Version
	)

	// setup for all tests
	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())

		cwd, err := os.Getwd()
		Expect(err).NotTo(HaveOccurred())
		testHelper, err = helper.NewSoloTestHelper(func(defaults helper.TestConfig) helper.TestConfig {
			defaults.RootDir = filepath.Join(cwd, "../../../../")
			defaults.HelmChartName = "gloo"
			defaults.InstallNamespace = namespace
			defaults.Verbose = true
			return defaults
		})
		Expect(err).NotTo(HaveOccurred())

		crdDir = filepath.Join(util.GetModuleRoot(), "install", "helm", "gloo", "crds")
		chartUri = filepath.Join(testHelper.RootDir, testHelper.TestAssetDir, testHelper.HelmChartName+"-"+testHelper.ChartVersion()+".tgz")
		strictValidation = false

		LastPatchMostRecentMinorVersion, CurrentPatchMostRecentMinorVersion, err = upgrades.GetUpgradeVersions(ctx, "gloo")
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("Upgrading from a previous gloo version to current version", func() {
		Context("When upgrading from LastPatchMostRecentMinorVersion to PR version of gloo", func() {
			BeforeEach(func() {
				upgrades.InstallGloo(testHelper, LastPatchMostRecentMinorVersion.String(), strictValidation)
			})
			AfterEach(func() {
				upgrades.UninstallGloo(testHelper, ctx, cancel)
			})
			//It("Used for local testing to check base case upgrades", func() {
			//	baseUpgradeTest(ctx, crdDir, LastPatchMostRecentMinorVersion.String(), testHelper, chartUri, strictValidation)
			//})
			It("uses helm to update validationServerGrpcMaxSizeBytes without errors", func() {
				upgrades.UpdateSettingsWithoutErrors(ctx, testHelper, crdDir, LastPatchMostRecentMinorVersion.String(), chartUri, strictValidation)
			})
			It("uses helm to add a second gateway-proxy in a separate namespace without errors", func() {
				upgrades.AddSecondGatewayProxySeparateNamespaceTest(testHelper, crdDir, chartUri, strictValidation)
			})
		})
		Context("When upgrading from CurrentPatchMostRecentMinorVersion to PR version of gloo", func() {
			BeforeEach(func() {
				upgrades.InstallGloo(testHelper, CurrentPatchMostRecentMinorVersion.String(), strictValidation)
			})
			AfterEach(func() {
				upgrades.UninstallGloo(testHelper, ctx, cancel)
			})
			It("uses helm to update validationServerGrpcMaxSizeBytes without errors", func() {
				upgrades.UpdateSettingsWithoutErrors(ctx, testHelper, CurrentPatchMostRecentMinorVersion.String(), crdDir, chartUri, strictValidation)
			})
			It("uses helm to add a second gateway-proxy in a separate namespace without errors", func() {
				upgrades.AddSecondGatewayProxySeparateNamespaceTest(testHelper, crdDir, chartUri, strictValidation)
			})
		})
	})

	Context("Validation webhook upgrade tests", func() {
		var cfg *rest.Config
		var err error
		var kubeClientset kubernetes.Interface

		BeforeEach(func() {
			cfg, err = kubeutils.GetConfig("", "")
			Expect(err).NotTo(HaveOccurred())
			kubeClientset, err = kubernetes.NewForConfig(cfg)
			Expect(err).NotTo(HaveOccurred())
			strictValidation = true
		})

		Context("When upgrading from LastPatchMostRecentMinorVersion to PR version of gloo", func() {
			BeforeEach(func() {
				upgrades.InstallGloo(testHelper, LastPatchMostRecentMinorVersion.String(), strictValidation)
			})
			AfterEach(func() {
				upgrades.UninstallGloo(testHelper, ctx, cancel)
			})
			It("sets validation webhook caBundle on install and upgrade", func() {
				upgrades.UpdateValidationWebhookTests(ctx, crdDir, kubeClientset, testHelper, chartUri, false)
			})
		})

		Context("When upgrading from CurrentPatchMostRecentMinorVersion to PR version of gloo", func() {
			BeforeEach(func() {
				upgrades.InstallGloo(testHelper, CurrentPatchMostRecentMinorVersion.String(), strictValidation)
			})
			AfterEach(func() {
				upgrades.UninstallGloo(testHelper, ctx, cancel)
			})
			It("sets validation webhook caBundle on install and upgrade", func() {
				upgrades.UpdateValidationWebhookTests(ctx, crdDir, kubeClientset, testHelper, chartUri, false)
			})
		})
	})
})

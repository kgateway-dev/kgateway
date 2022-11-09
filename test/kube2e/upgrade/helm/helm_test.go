package helm_test

import (
	"bytes"
	"context"
	"fmt"
	"github.com/ghodss/yaml"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	gatewayv1kube "github.com/solo-io/gloo/projects/gateway/pkg/api/v1/kube/client/clientset/versioned/typed/gateway.solo.io/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/test/kube2e"
	"github.com/solo-io/gloo/test/kube2e/upgrade"
	exec_utils "github.com/solo-io/go-utils/testutils/exec"
	"github.com/solo-io/k8s-utils/kubeutils"
	"github.com/solo-io/k8s-utils/testutils/helper"
	"github.com/solo-io/skv2/codegen/util"
	"github.com/solo-io/solo-kit/pkg/code-generator/schemagen"
	admission_v1 "k8s.io/api/admissionregistration/v1"
	v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	admission_v1_types "k8s.io/client-go/kubernetes/typed/admissionregistration/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/rest"
	"os"
	"path/filepath"
	"text/template"
)

// now that we run CI on a kube 1.22 cluster, we must ensure that we install versions of gloo with v1 CRDs
// Per https://github.com/solo-io/gloo/issues/4543: CRDs were migrated from v1beta1 -> v1 in Gloo 1.9.0
const earliestVersionWithV1CRDs = "1.9.0"

// for testing upgrades from a gloo version before the gloo/gateway merge and
// before https://github.com/solo-io/gloo/pull/6349 was fixed
// TODO delete tests once this version is no longer supported https://github.com/solo-io/gloo/issues/6661
const versionBeforeGlooGatewayMerge = "1.11.0"

const namespace = defaults.GlooSystem

var _ = Describe("Kube2e: helm", func() {

	var (
		crdDir   string
		chartUri string

		ctx    context.Context
		cancel context.CancelFunc

		testHelper *helper.SoloTestHelper

		// if set, the test will install from a released version (rather than local version) of the helm chart
		fromRelease string
		// whether to set validation webhook's failurePolicy=Fail
		strictValidation bool
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())

		cwd, err := os.Getwd()
		Expect(err).NotTo(HaveOccurred())
		testHelper, err = helper.NewSoloTestHelper(func(defaults helper.TestConfig) helper.TestConfig {
			defaults.RootDir = filepath.Join(cwd, "../../..")
			defaults.HelmChartName = "gloo"
			defaults.InstallNamespace = namespace
			defaults.Verbose = true
			return defaults
		})
		Expect(err).NotTo(HaveOccurred())

		crdDir = filepath.Join(util.GetModuleRoot(), "install", "helm", "gloo", "crds")
		chartUri = filepath.Join(testHelper.RootDir, testHelper.TestAssetDir, testHelper.HelmChartName+"-"+testHelper.ChartVersion()+".tgz")

		fromRelease = ""
		strictValidation = false
	})

	JustBeforeEach(func() {
		installGloo(testHelper, chartUri, fromRelease, strictValidation)
	})

	AfterEach(func() {
		upgrade.UninstallGloo(testHelper, ctx, cancel)
	})

	Context("upgrades", func() {
		BeforeEach(func() {
			fromRelease = earliestVersionWithV1CRDs
		})

		It("uses helm to upgrade to this gloo version and settings without errors", func() {
			upgrade.UpdateSettingsWithoutErrors(ctx, testHelper, crdDir, earliestVersionWithV1CRDs, chartUri, strictValidation)
		})

		It("uses helm to add a second gateway-proxy in a separate namespace without errors", func() {
			upgrade.AddSecondGatewayProxySeparateNamespaceTest(testHelper, crdDir, chartUri, strictValidation)
		})
	})

	Context("validation webhook", func() {
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

		It("sets validation webhook caBundle on install and upgrade", func() {
			upgrade.UpdateValidationWebhookTests(ctx, crdDir, kubeClientset, testHelper, chartUri, false)
		})

		// Below are tests with different combinations of upgrades with failurePolicy=Ignore/Fail.
		Context("failurePolicy upgrades", func() {

			var webhookConfigClient admission_v1_types.ValidatingWebhookConfigurationInterface
			var gatewayV1Client gatewayv1kube.GatewayV1Interface

			BeforeEach(func() {
				webhookConfigClient = kubeClientset.AdmissionregistrationV1().ValidatingWebhookConfigurations()
				gatewayV1Client, err = gatewayv1kube.NewForConfig(cfg)
				Expect(err).NotTo(HaveOccurred())
			})

			testFailurePolicyUpgrade := func(oldFailurePolicy admission_v1.FailurePolicyType, newFailurePolicy admission_v1.FailurePolicyType) {
				By(fmt.Sprintf("should start with gateway.validation.failurePolicy=%v", oldFailurePolicy))
				webhookConfig, err := webhookConfigClient.Get(ctx, "gloo-gateway-validation-webhook-"+testHelper.InstallNamespace, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
				Expect(*webhookConfig.Webhooks[0].FailurePolicy).To(Equal(oldFailurePolicy))

				// to ensure the default Gateways were not deleted during upgrade, compare their creation timestamps before and after the upgrade
				gw, err := gatewayV1Client.Gateways(namespace).Get(ctx, "gateway-proxy", metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
				gwTimestampBefore := gw.GetCreationTimestamp().String()
				gwSsl, err := gatewayV1Client.Gateways(namespace).Get(ctx, "gateway-proxy-ssl", metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
				gwSslTimestampBefore := gwSsl.GetCreationTimestamp().String()

				// upgrade to the new failurePolicy type
				var newStrictValue = false
				if newFailurePolicy == admission_v1.Fail {
					newStrictValue = true
				}
				upgrade.GlooToBranchVersion(testHelper, chartUri, crdDir, newStrictValue, []string{})

				By(fmt.Sprintf("should have updated to gateway.validation.failurePolicy=%v", newFailurePolicy))
				webhookConfig, err = webhookConfigClient.Get(ctx, "gloo-gateway-validation-webhook-"+testHelper.InstallNamespace, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
				Expect(*webhookConfig.Webhooks[0].FailurePolicy).To(Equal(newFailurePolicy))

				By("Gateway creation timestamps should not have changed")
				gw, err = gatewayV1Client.Gateways(namespace).Get(ctx, "gateway-proxy", metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
				gwTimestampAfter := gw.GetCreationTimestamp().String()
				Expect(gwTimestampBefore).To(Equal(gwTimestampAfter))
				gwSsl, err = gatewayV1Client.Gateways(namespace).Get(ctx, "gateway-proxy-ssl", metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
				gwSslTimestampAfter := gwSsl.GetCreationTimestamp().String()
				Expect(gwSslTimestampBefore).To(Equal(gwSslTimestampAfter))
			}

			Context("starting from before the gloo/gateway merge, with failurePolicy=Ignore", func() {
				BeforeEach(func() {
					fromRelease = versionBeforeGlooGatewayMerge
					strictValidation = false
				})
				It("can upgrade to current release, with failurePolicy=Ignore", func() {
					testFailurePolicyUpgrade(admission_v1.Ignore, admission_v1.Ignore)
				})
				It("can upgrade to current release, with failurePolicy=Fail", func() {
					testFailurePolicyUpgrade(admission_v1.Ignore, admission_v1.Fail)
				})
			})
			Context("starting from helm hook release, with failurePolicy=Fail", func() {
				BeforeEach(func() {
					// The original fix for installing with failurePolicy=Fail (https://github.com/solo-io/gloo/issues/6213)
					// went into gloo v1.11.10. It turned the Gloo custom resources into helm hooks to guarantee ordering,
					// however it caused additional issues so we moved away from using helm hooks. This test is to ensure
					// we can successfully upgrade from the helm hook release to the current release.
					// TODO delete tests once this version is no longer supported https://github.com/solo-io/gloo/issues/6661
					fromRelease = "1.11.10"
					strictValidation = true
				})
				It("can upgrade to current release, with failurePolicy=Fail", func() {
					testFailurePolicyUpgrade(admission_v1.Fail, admission_v1.Fail)
				})
			})
		})

	})

	Context("applies all CRD manifests without an error", func() {

		var crdsByFileName = map[string]v1.CustomResourceDefinition{}

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
				err := exec_utils.RunCommand(testHelper.RootDir, false, "kubectl", "apply", "-f", crdFile)
				Expect(err).NotTo(HaveOccurred(), "should be able to kubectl apply -f %s", crdFile)

				// Ensure the CRD is eventually accepted
				Eventually(func() (string, error) {
					return exec_utils.RunCommandOutput(testHelper.RootDir, false, "kubectl", "get", "crd", crd.GetName())
				}, "10s", "1s").Should(ContainSubstring(crd.GetName()))
			}
		})
	})

	Context("applies settings manifests used in helm unit tests (install/test/fixtures/settings)", func() {
		// The local helm tests involve templating settings with various values set
		// and then validating that the templated data matches fixture data.
		// The tests assume that the fixture data we have defined is valid yaml that
		// will be accepted by a cluster. However, this has not always been the case
		// and it's important that we validate the settings end to end
		//
		// This solution may not be the best way to validate settings, but it
		// attempts to avoid re-running all the helm template tests against a live cluster
		var settingsFixturesFolder string

		BeforeEach(func() {
			settingsFixturesFolder = filepath.Join(util.GetModuleRoot(), "install", "test", "fixtures", "settings")

			// Apply the Settings CRD to ensure it is the most up to date version
			// this ensures that any new fields that have been added are included in the CRD validation schemas
			settingsCrdFilePath := filepath.Join(crdDir, "gloo.solo.io_v1_Settings.yaml")
			upgrade.RunAndCleanCommand("kubectl", "apply", "-f", settingsCrdFilePath)
		})

		It("works using kubectl apply", func() {
			err := filepath.Walk(settingsFixturesFolder, func(settingsFixtureFile string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if info.IsDir() {
					return nil
				}

				templatedSettings := makeUnstructuredFromTemplateFile(settingsFixtureFile, namespace)
				settingsBytes, err := templatedSettings.MarshalJSON()

				// Apply the fixture
				err = exec_utils.RunCommandInput(string(settingsBytes), testHelper.RootDir, false, "kubectl", "apply", "-f", "-")
				Expect(err).NotTo(HaveOccurred(), "should be able to kubectl apply -f %s", settingsFixtureFile)

				// continue traversing
				return nil
			})
			Expect(err).NotTo(HaveOccurred())
		})

	})
})

func makeUnstructured(yam string) *unstructured.Unstructured {
	jsn, err := yaml.YAMLToJSON([]byte(yam))
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	runtimeObj, err := runtime.Decode(unstructured.UnstructuredJSONScheme, jsn)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	return runtimeObj.(*unstructured.Unstructured)
}

func makeUnstructuredFromTemplateFile(fixtureName string, values interface{}) *unstructured.Unstructured {
	tmpl, err := template.ParseFiles(fixtureName)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	var b bytes.Buffer
	err = tmpl.Execute(&b, values)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	return makeUnstructured(b.String())
}

// Helm tests use the fromRelease parameter to determine the current
// release version so cannot share code between other upgrade tests
func installGloo(testHelper *helper.SoloTestHelper, chartUri string, fromRelease string, strictValidation bool) {
	valueOverrideFile, cleanupFunc := kube2e.GetHelmValuesOverrideFile()
	defer cleanupFunc()

	// construct helm args
	var args = []string{"install", testHelper.HelmChartName}
	if fromRelease != "" {
		upgrade.RunAndCleanCommand("helm", "repo", "add", testHelper.HelmChartName,
			"https://storage.googleapis.com/solo-public-helm", "--force-update")
		args = append(args, "gloo/gloo",
			"--version", fmt.Sprintf("v%s", fromRelease))
	} else {
		args = append(args, chartUri)
	}
	args = append(args, "-n", testHelper.InstallNamespace,
		"--create-namespace",
		"--values", valueOverrideFile)
	if strictValidation {
		args = append(args, upgrade.StrictValidationArgs...)
	}

	fmt.Printf("running helm with args: %v\n", args)
	upgrade.RunAndCleanCommand("helm", args...)

	// Check that everything is OK
	upgrade.CheckGlooOssHealthy(testHelper)
}

package helm_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/ghodss/yaml"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gatewayv1kube "github.com/solo-io/gloo/projects/gateway/pkg/api/v1/kube/client/clientset/versioned/typed/gateway.solo.io/v1"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/gateway"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/version"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/grpc_json"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/test/kube2e"
	exec_utils "github.com/solo-io/go-utils/testutils/exec"
	"github.com/solo-io/k8s-utils/kubeutils"
	"github.com/solo-io/k8s-utils/testutils/helper"
	"github.com/solo-io/skv2/codegen/util"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/code-generator/schemagen"
	admission_v1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	admission_v1_types "k8s.io/client-go/kubernetes/typed/admissionregistration/v1"
	core_v1_types "k8s.io/client-go/kubernetes/typed/core/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/rest"
)

// now that we run CI on a kube 1.22+ cluster, we must ensure that we install versions of gloo with v1 CRDs
// Per https://github.com/solo-io/gloo/issues/4543: CRDs were migrated from v1beta1 -> v1 in Gloo 1.9.0
const earliestVersionWithV1CRDs = "1.9.0"

// for testing upgrades from a gloo version before the gloo/gateway merge and
// before https://github.com/solo-io/gloo/pull/6349 was fixed
// TODO delete tests once this version is no longer supported https://github.com/solo-io/gloo/issues/6661
const versionBeforeGlooGatewayMerge = "1.11.0"

const namespace = defaults.GlooSystem

var glooDeploymentsToCheck []string

var _ = Describe("Kube2e: helm", func() {

	var (
		crdDir   string
		chartUri string

		ctx    context.Context
		cancel context.CancelFunc

		testHelper *helper.SoloTestHelper

		// if set, test a released version (rather than local version) of the helm chart
		targetVersion string
		// Gloo version to install in JustBeforeEach (sometimes this is the version to upgrade from, sometimes version to install and verify)
		fromRelease string
		// whether to set validation webhook's failurePolicy=Fail
		strictValidation bool

		// additional args to pass into the initial helm install
		additionalInstallArgs []string
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())

		cwd, err := os.Getwd()
		Expect(err).NotTo(HaveOccurred())
		fromRelease = ""
		targetVersion = kube2e.GetTestReleasedVersion(ctx, "gloo")

		testHelper, err = helper.NewSoloTestHelper(func(defaults helper.TestConfig) helper.TestConfig {
			defaults.RootDir = filepath.Join(cwd, "../../..")
			defaults.HelmChartName = "gloo"
			defaults.InstallNamespace = namespace
			defaults.Verbose = true
			defaults.ReleasedVersion = targetVersion
			return defaults
		})
		Expect(err).NotTo(HaveOccurred())

		crdDir = filepath.Join(util.GetModuleRoot(), "install", "helm", "gloo", "crds")
		chartUri = filepath.Join(testHelper.RootDir, testHelper.TestAssetDir, testHelper.HelmChartName+"-"+testHelper.ChartVersion()+".tgz")
		strictValidation = false

		glooDeploymentsToCheck = []string{"gloo", "discovery", "gateway-proxy"}
		additionalInstallArgs = []string{}
	})

	JustBeforeEach(func() {
		if fromRelease == "" && targetVersion != "" {
			fromRelease = targetVersion
		}
		installGloo(testHelper, chartUri, fromRelease, strictValidation, additionalInstallArgs)
	})

	AfterEach(func() {
		uninstallGloo(testHelper, ctx, cancel)
	})

	Context("upgrades", func() {
		BeforeEach(func() {
			fromRelease = earliestVersionWithV1CRDs
		})

		It("uses helm to update the settings without errors", func() {
			By("should start with gloo version 1.9.0")
			Expect(getGlooServerVersion(ctx, testHelper.InstallNamespace)).To(Equal(earliestVersionWithV1CRDs))

			By("should start with the settings.invalidConfigPolicy.invalidRouteResponseCode=404")
			client := helpers.MustSettingsClient(ctx)
			settings, err := client.Read(testHelper.InstallNamespace, defaults.SettingsName, clients.ReadOpts{})
			Expect(err).To(BeNil())
			Expect(settings.GetGloo().GetInvalidConfigPolicy().GetInvalidRouteResponseCode()).To(Equal(uint32(404)))
			Expect(settings.GetGateway().GetValidation().GetValidationServerGrpcMaxSizeBytes().GetValue()).To(Equal(int32(4000000)))

			upgradeGloo(testHelper, chartUri, crdDir, fromRelease, targetVersion, strictValidation, []string{
				"--set", "settings.replaceInvalidRoutes=true",
				"--set", "settings.invalidConfigPolicy.invalidRouteResponseCode=400",
				"--set", "gateway.validation.validationServerGrpcMaxSizeBytes=5000000",
			})

			By("should have upgraded to the gloo version being tested")
			Expect(getGlooServerVersion(ctx, testHelper.InstallNamespace)).To(Equal(testHelper.ChartVersion()))

			By("should have updated to settings.invalidConfigPolicy.invalidRouteResponseCode=400")
			settings, err = client.Read(testHelper.InstallNamespace, defaults.SettingsName, clients.ReadOpts{})
			Expect(err).To(BeNil())
			Expect(settings.GetGloo().GetInvalidConfigPolicy().GetInvalidRouteResponseCode()).To(Equal(uint32(400)))
			Expect(settings.GetGateway().GetValidation().GetValidationServerGrpcMaxSizeBytes().GetValue()).To(Equal(int32(5000000)))
		})

		It("uses helm to add a second gateway-proxy in a separate namespace without errors", func() {
			const externalNamespace = "other-ns"
			requiredSettings := map[string]string{
				"gatewayProxies.proxyExternal.disabled":              "false",
				"gatewayProxies.proxyExternal.namespace":             externalNamespace,
				"gatewayProxies.proxyExternal.service.type":          "NodePort",
				"gatewayProxies.proxyExternal.service.httpPort":      "31500",
				"gatewayProxies.proxyExternal.service.httpsPort":     "32500",
				"gatewayProxies.proxyExternal.service.httpNodePort":  "31500",
				"gatewayProxies.proxyExternal.service.httpsNodePort": "32500",
			}

			var settings []string
			for key, val := range requiredSettings {
				settings = append(settings, "--set")
				settings = append(settings, strings.Join([]string{key, val}, "="))
			}

			runAndCleanCommand("kubectl", "create", "ns", externalNamespace)
			defer runAndCleanCommand("kubectl", "delete", "ns", externalNamespace)

			upgradeGloo(testHelper, chartUri, crdDir, fromRelease, targetVersion, strictValidation, settings)

			// Ensures deployment is created for both default namespace and external one
			// Note- name of external deployments is kebab-case of gatewayProxies NAME helm value
			Eventually(func() (string, error) {
				return exec_utils.RunCommandOutput(testHelper.RootDir, false,
					"kubectl", "get", "deployment", "-A")
			}, "10s", "1s").Should(
				And(ContainSubstring("gateway-proxy"),
					ContainSubstring("proxy-external")))

			// Ensures service account is created for the external namespace
			Eventually(func() (string, error) {
				return exec_utils.RunCommandOutput(testHelper.RootDir, false,
					"kubectl", "get", "serviceaccount", "-n", externalNamespace)
			}, "10s", "1s").Should(ContainSubstring("gateway-proxy"))
		})

		It("triggers a new rollout when the configmap has changed", func() {
			getDeploymentChecksumAnnotation := func() []byte {
				// kubectl -n gloo-system get deployment gateway-proxy -o jsonpath='{.spec.template.metadata.annotations.checksum/gateway-proxy-envoy-config}'
				return runAndCleanCommand("kubectl", "-n", "gloo-system", "get", "deployment", "gateway-proxy", "-o", "jsonpath='{.spec.template.metadata.annotations.checksum/gateway-proxy-envoy-config}'")
			}

			expectDeploymentChecksumAnnotationToEqual := func(old []byte) {
				EventuallyWithOffset(1, func() []byte {
					return getDeploymentChecksumAnnotation()
				}, "30s", "1s").Should(
					Equal(old))
			}

			expectDeploymentChecksumAnnotationChangedFrom := func(old []byte) {
				EventuallyWithOffset(1, func() []byte {
					return getDeploymentChecksumAnnotation()
				}, "30s", "1s").Should(
					Not(Equal(old)))
			}

			expectConfigDumpToContain := func(str string) {
				EventuallyWithOffset(1, func() string {
					return GetEnvoyCfgDump(testHelper)
				}, "30s", "1s").Should(
					ContainSubstring(str))
			}

			// The default value is 250000
			expectConfigDumpToContain(`"global_downstream_max_connections": 250000`)

			// Since we are running a version that doesn't have this annotation, we need to upgrade to one that does.
			// This should trigger a new deployment anyway
			previousAnnotationValue := getDeploymentChecksumAnnotation()
			upgradeGloo(testHelper, chartUri, crdDir, fromRelease, targetVersion, strictValidation, nil)
			expectDeploymentChecksumAnnotationChangedFrom(previousAnnotationValue)
			expectConfigDumpToContain(`"global_downstream_max_connections": 250000`)

			// Repeat the same upgrade. The annotation shouldn't have changed
			previousAnnotationValue = getDeploymentChecksumAnnotation()
			upgradeGloo(testHelper, chartUri, crdDir, fromRelease, targetVersion, strictValidation, nil)
			expectDeploymentChecksumAnnotationToEqual(previousAnnotationValue)

			// We upgrade Gloo with a new value of `globalDownstreamMaxConnections` on envoy
			// This should cause the checkup annotation on the deployment to change and therefore
			// the deployment should be updated with the new value
			previousAnnotationValue = getDeploymentChecksumAnnotation()
			requiredSettings := map[string]string{
				"gatewayProxies.gatewayProxy.globalDownstreamMaxConnections": "12345",
			}
			var settings []string
			for key, val := range requiredSettings {
				settings = append(settings, "--set")
				settings = append(settings, strings.Join([]string{key, val}, "="))
			}
			upgradeGloo(testHelper, chartUri, crdDir, fromRelease, targetVersion, strictValidation, settings)
			expectDeploymentChecksumAnnotationChangedFrom(previousAnnotationValue)
			expectConfigDumpToContain(`"global_downstream_max_connections": 12345`)
		})
	})

	Context("Production recommendations", func() {
		var valuesForProductionRecommendations []string
		var expectGatewayProxyIsReady func()

		BeforeEach(func() {
			valuesForProductionRecommendations = getHelmValuesForProductionRecommendations()

			// Since the production recommendation is to disable discovery, we remove it from the list of deployments to check to consider gloo is healthy
			glooDeploymentsToCheck = []string{"gloo", "gateway-proxy"}

			additionalInstallArgs = []string{
				// Setting `settings.disableKubernetesDestinations` && `global.glooRbac.namespaced` leads to panic in gloo
				// Ref: https://github.com/solo-io/gloo/issues/8801
				"--set", "global.glooRbac.namespaced=false",
			}
			additionalInstallArgs = append(additionalInstallArgs, valuesForProductionRecommendations...)

			expectGatewayProxyIsReady = func() {
				Eventually(func() (string, error) {
					a, b := exec_utils.RunCommandOutput(testHelper.RootDir, false,
						"kubectl", "-n", namespace, "get", "deployment", "gateway-proxy", "-o", "yaml")
					return a, b
				}, "30s", "1s").Should(
					// kubectl -n gloo-system get deployment gateway-proxy -o yaml
					// ...
					// readinessProbe:
					//   httpGet:
					//     path: /envoy-hc
					// ...
					// readyReplicas: 1
					And(ContainSubstring("readinessProbe:"),
						ContainSubstring("/envoy-hc"),
						ContainSubstring("readyReplicas: 1")))
			}
		})

		It("succeeds", func() {
			// Since one of the production recommendations is to have a custom readiness probe, check if it is present on the proxy.
			// The rest of them have their own unit / e2e tests.
			expectGatewayProxyIsReady()
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
			webhookConfigClient := kubeClientset.AdmissionregistrationV1().ValidatingWebhookConfigurations()
			secretClient := kubeClientset.CoreV1().Secrets(testHelper.InstallNamespace)

			By("the webhook caBundle should be the same as the secret's root ca value")
			webhookConfig, err := webhookConfigClient.Get(ctx, "gloo-gateway-validation-webhook-"+testHelper.InstallNamespace, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			secret, err := secretClient.Get(ctx, "gateway-validation-certs", metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(webhookConfig.Webhooks[0].ClientConfig.CABundle).To(Equal(secret.Data[corev1.ServiceAccountRootCAKey]))

			// do an upgrade
			upgradeGloo(testHelper, chartUri, crdDir, fromRelease, targetVersion, strictValidation, nil)

			By("the webhook caBundle and secret's root ca value should still match after upgrade")
			webhookConfig, err = webhookConfigClient.Get(ctx, "gloo-gateway-validation-webhook-"+testHelper.InstallNamespace, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			secret, err = secretClient.Get(ctx, "gateway-validation-certs", metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(webhookConfig.Webhooks[0].ClientConfig.CABundle).To(Equal(secret.Data[corev1.ServiceAccountRootCAKey]))
		})

		It("sets timeout on validation webhook", func() {
			webhookConfigClient := kubeClientset.AdmissionregistrationV1().ValidatingWebhookConfigurations()

			validationWebhook, err := webhookConfigClient.Get(ctx, fmt.Sprintf("gloo-gateway-validation-webhook-%v", namespace), metav1.GetOptions{})
			Expect(err).ToNot(HaveOccurred())
			Expect(*validationWebhook.Webhooks[0].TimeoutSeconds).To(Equal(int32(10)))

			upgradeGloo(testHelper, chartUri, crdDir, fromRelease, targetVersion, strictValidation, []string{"--set", "gateway.validation.webhook.timeoutSeconds=5"})

			validationWebhook, err = webhookConfigClient.Get(ctx, fmt.Sprintf("gloo-gateway-validation-webhook-%v", namespace), metav1.GetOptions{})
			Expect(err).ToNot(HaveOccurred())
			Expect(*validationWebhook.Webhooks[0].TimeoutSeconds).To(Equal(int32(5)))
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
				upgradeGloo(testHelper, chartUri, crdDir, fromRelease, targetVersion, newStrictValue, []string{})

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

	Context("installing with large proto descriptor", func() {
		var gatewayClient gatewayv1kube.GatewayV1Interface
		var configMapClient core_v1_types.ConfigMapInterface
		var protoDescriptor string

		BeforeEach(func() {
			cfg, err := kubeutils.GetConfig("", "")
			Expect(err).NotTo(HaveOccurred())

			// initialize gateway client
			gatewayClient, err = gatewayv1kube.NewForConfig(cfg)
			Expect(err).NotTo(HaveOccurred())

			// initialize configmap client
			kubeClientset, err := kubernetes.NewForConfig(cfg)
			Expect(err).NotTo(HaveOccurred())
			configMapClient = kubeClientset.CoreV1().ConfigMaps(testHelper.InstallNamespace)

			protoDescriptor = getExampleProtoDescriptor()
		})

		Context("using protoDescrfiptorBin field", func() {
			BeforeEach(func() {
				// args to install gloo with protoDescriptorBin on http and https gateway
				additionalInstallArgs = []string{
					"--set", "gatewayProxies.gatewayProxy.gatewaySettings.customHttpGateway.options.grpcJsonTranscoder.protoDescriptorBin=" + protoDescriptor,
					"--set", "gatewayProxies.gatewayProxy.gatewaySettings.customHttpsGateway.options.grpcJsonTranscoder.protoDescriptorBin=" + protoDescriptor,
				}
			})
			It("can install with large protoDescriptorBin", func() {
				// check that each Gateway's protoDescriptorBin field was populated
				gw, err := gatewayClient.Gateways(namespace).Get(ctx, "gateway-proxy", metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
				// the field on the Gateway gets automatically decoded to the binary bytes, so we need to re-encode it to do the comparison
				gwProtoDescBytes := gw.Spec.GatewayType.(*gatewayv1.Gateway_HttpGateway).HttpGateway.Options.GrpcJsonTranscoder.DescriptorSet.(*grpc_json.GrpcJsonTranscoder_ProtoDescriptorBin).ProtoDescriptorBin
				gwProtoDesc := base64.StdEncoding.EncodeToString(gwProtoDescBytes)
				Expect(gwProtoDesc).To(Equal(protoDescriptor))

				gwSsl, err := gatewayClient.Gateways(namespace).Get(ctx, "gateway-proxy-ssl", metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
				gwSslProtoDescBytes := gwSsl.Spec.GatewayType.(*gatewayv1.Gateway_HttpGateway).HttpGateway.Options.GrpcJsonTranscoder.DescriptorSet.(*grpc_json.GrpcJsonTranscoder_ProtoDescriptorBin).ProtoDescriptorBin
				gwSslProtoDesc := base64.StdEncoding.EncodeToString(gwSslProtoDescBytes)
				Expect(gwSslProtoDesc).To(Equal(protoDescriptor))
			})
		})

		Context("using protoDescriptorConfigMap field", func() {
			BeforeEach(func() {
				// args to install gloo with protoDescriptorConfigMap on http and https gateway
				additionalInstallArgs = []string{
					"--set", "gatewayProxies.gatewayProxy.gatewaySettings.customHttpGateway.options.grpcJsonTranscoder.protoDescriptorConfigMap.configMapRef.name=my-config-map",
					"--set", "gatewayProxies.gatewayProxy.gatewaySettings.customHttpGateway.options.grpcJsonTranscoder.protoDescriptorConfigMap.configMapRef.namespace=gloo-system",
					"--set", "gatewayProxies.gatewayProxy.gatewaySettings.customHttpGateway.options.grpcJsonTranscoder.protoDescriptorConfigMap.key=my-key",
					"--set", "gatewayProxies.gatewayProxy.gatewaySettings.customHttpsGateway.options.grpcJsonTranscoder.protoDescriptorConfigMap.configMapRef.name=my-config-map",
					"--set", "gatewayProxies.gatewayProxy.gatewaySettings.customHttpsGateway.options.grpcJsonTranscoder.protoDescriptorConfigMap.configMapRef.namespace=gloo-system",
					"--set", "gatewayProxies.gatewayProxy.gatewaySettings.customHttpsGateway.options.grpcJsonTranscoder.protoDescriptorConfigMap.key=my-key",
					"--set", "global.configMaps[0].name=my-config-map",
					"--set", "global.configMaps[0].namespace=gloo-system",
					"--set", "global.configMaps[0].data.my-key=" + protoDescriptor,
				}
			})
			It("can install with protoDescriptorConfigMap", func() {
				// check that each Gateway's protoDescriptorConfigMap field was populated
				gw, err := gatewayClient.Gateways(namespace).Get(ctx, "gateway-proxy", metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
				gwProtoDescConfigMap := gw.Spec.GatewayType.(*gatewayv1.Gateway_HttpGateway).HttpGateway.Options.GrpcJsonTranscoder.DescriptorSet.(*grpc_json.GrpcJsonTranscoder_ProtoDescriptorConfigMap).ProtoDescriptorConfigMap
				Expect(gwProtoDescConfigMap.GetConfigMapRef().GetName()).To(Equal("my-config-map"))
				Expect(gwProtoDescConfigMap.GetConfigMapRef().GetNamespace()).To(Equal("gloo-system"))
				Expect(gwProtoDescConfigMap.GetKey()).To(Equal("my-key"))

				gwSsl, err := gatewayClient.Gateways(namespace).Get(ctx, "gateway-proxy-ssl", metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
				gwSslrotoDescConfigMap := gwSsl.Spec.GatewayType.(*gatewayv1.Gateway_HttpGateway).HttpGateway.Options.GrpcJsonTranscoder.DescriptorSet.(*grpc_json.GrpcJsonTranscoder_ProtoDescriptorConfigMap).ProtoDescriptorConfigMap
				Expect(gwSslrotoDescConfigMap.GetConfigMapRef().GetName()).To(Equal("my-config-map"))
				Expect(gwSslrotoDescConfigMap.GetConfigMapRef().GetNamespace()).To(Equal("gloo-system"))
				Expect(gwSslrotoDescConfigMap.GetKey()).To(Equal("my-key"))

				// check that the ConfigMap was created to store the proto descriptor
				cm, err := configMapClient.Get(ctx, "my-config-map", metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
				Expect(cm.Data["my-key"]).To(Equal(protoDescriptor))
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
			runAndCleanCommand("kubectl", "apply", "-f", settingsCrdFilePath)
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

func getGlooServerVersion(ctx context.Context, namespace string) (v string) {
	glooVersion, err := version.GetClientServerVersions(ctx, version.NewKube(namespace, ""))
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

func installGloo(testHelper *helper.SoloTestHelper, chartUri string, fromRelease string, strictValidation bool, additionalInstallArgs []string) {
	helmValuesFile := getHelmValuesFile("helm.yaml")

	// construct helm args
	var args = []string{"install", testHelper.HelmChartName}
	if fromRelease != "" {
		runAndCleanCommand("helm", "repo", "add", testHelper.HelmChartName,
			"https://storage.googleapis.com/solo-public-helm", "--force-update")
		args = append(args, "gloo/gloo",
			"--version", fmt.Sprintf("%s", fromRelease))
	} else {
		args = append(args, chartUri)
	}
	args = append(args, "-n", testHelper.InstallNamespace,
		// As most CD tools wait for resources to be ready before marking the release as successful,
		// we're emulating that here by passing these two flags.
		// This way we ensure that we indirectly add support for CD tools
		"--wait",
		"--wait-for-jobs",
		// We run our e2e tests on a kind cluster, but kind hasn’t implemented LoadBalancer support.
		// This leads to the service being in a pending state.
		// Since the --wait flag is set, this can cause the upgrade to fail
		// as helm waits until the service is ready and eventually times out.
		// So instead we use the service type as ClusterIP to work around this limitation.
		"--set", "gatewayProxies.gatewayProxy.service.type=ClusterIP",
		"--create-namespace",
		"--values", helmValuesFile)
	if strictValidation {
		args = append(args, strictValidationArgs...)
	}

	args = append(args, additionalInstallArgs...)
	fmt.Printf("running helm with args: %v, target: %v\n", args, fromRelease)
	runAndCleanCommand("helm", args...)

	// Check that everything is OK
	checkGlooHealthy(testHelper)
}

// CRDs are applied to a cluster when performing a `helm install` operation
// However, `helm upgrade` intentionally does not apply CRDs (https://helm.sh/docs/topics/charts/#limitations-on-crds)
// Before performing the upgrade, we must manually apply any CRDs that were introduced since v1.9.0
func upgradeCrds(testHelper *helper.SoloTestHelper, fromRelease string, crdDir string) {
	fmt.Printf("Upgrading crds release %s, crdDir %s\n", fromRelease, crdDir)
	// if we're just upgrading within the same release, no need to reapply crds
	if fromRelease == "" {
		return
	}

	// apply crds from the release we're upgrading to
	runAndCleanCommand("kubectl", "apply", "-f", crdDir)
	// allow some time for the new crds to take effect
	time.Sleep(time.Second * 5)
}

func upgradeGlooWithCustomValuesFile(testHelper *helper.SoloTestHelper, chartUri string, crdDir string, fromRelease string, targetRelease string, strictValidation bool, additionalArgs []string, valueOverrideFile string) {
	upgradeCrds(testHelper, fromRelease, crdDir)

	var args = []string{"upgrade", testHelper.HelmChartName,
		// As most CD tools wait for resources to be ready before marking the release as successful,
		// we're emulating that here by passing these two flags.
		// This way we ensure that we indirectly add support for CD tools
		"--wait",
		"--wait-for-jobs",
		// We run our e2e tests on a kind cluster, but kind hasn’t implemented LoadBalancer support.
		// This leads to the service being in a pending state.
		// Since the --wait flag is set, this can cause the upgrade to fail
		// as helm waits until the service is ready and eventually times out.
		// So instead we use the service type as ClusterIP to work around this limitation.
		"--set", "gatewayProxies.gatewayProxy.service.type=ClusterIP",
		"-n", testHelper.InstallNamespace,
	}
	if valueOverrideFile != "" {
		args = append(args, "--values", valueOverrideFile)
	}
	if targetRelease != "" {
		args = append(args, "gloo/gloo",
			"--version", fmt.Sprintf("%s", targetRelease))
	} else {
		args = append(args, chartUri)
	}
	args = append(args, "-n", testHelper.InstallNamespace, "--values", valueOverrideFile)
	if strictValidation {
		args = append(args, strictValidationArgs...)
	}
	args = append(args, additionalArgs...)
	fmt.Printf("running helm with args: %v target %v\n", args, targetRelease)
	runAndCleanCommand("helm", args...)

	// Check that everything is OK
	checkGlooHealthy(testHelper)

}

func upgradeGloo(testHelper *helper.SoloTestHelper, chartUri string, crdDir string, fromRelease string, targetRelease string, strictValidation bool, additionalArgs []string) {
	valueOverrideFile := getHelmUpgradeValuesOverrideFile()
	upgradeGlooWithCustomValuesFile(testHelper, chartUri, crdDir, fromRelease, targetRelease, strictValidation, additionalArgs, valueOverrideFile)
}

func uninstallGloo(testHelper *helper.SoloTestHelper, ctx context.Context, cancel context.CancelFunc) {
	Expect(testHelper).ToNot(BeNil())
	err := testHelper.UninstallGlooAll()
	Expect(err).NotTo(HaveOccurred())
	_, err = kube2e.MustKubeClient().CoreV1().Namespaces().Get(ctx, testHelper.InstallNamespace, metav1.GetOptions{})
	Expect(apierrors.IsNotFound(err)).To(BeTrue())
	cancel()
}

func getHelmValuesFile(filename string) string {
	cwd, err := os.Getwd()
	Expect(err).NotTo(HaveOccurred(), "working dir could not be retrieved")
	helmUpgradeValuesFile := filepath.Join(cwd, "artifacts", filename)
	return helmUpgradeValuesFile

}

func getHelmUpgradeValuesOverrideFile() (filename string) {
	return getHelmValuesFile("upgrade-override.yaml")
}

func getHelmValuesForProductionRecommendations() []string {
	return []string{
		"--values", getHelmValuesFile("access-logging.yaml"),
		"--values", getHelmValuesFile("custom-readiness-probe.yaml"),
		"--values", getHelmValuesFile("horizontal-scaling.yaml"),
		"--values", getHelmValuesFile("performance.yaml"),
		"--values", getHelmValuesFile("safeguards.yaml"),
	}
}

// return a base64-encoded proto descriptor to use for testing
func getExampleProtoDescriptor() string {
	pathToDescriptors := "../../v1helpers/test_grpc_service/descriptors/proto.pb"
	bytes, err := os.ReadFile(pathToDescriptors)
	Expect(err).NotTo(HaveOccurred())
	return base64.StdEncoding.EncodeToString(bytes)
}

var strictValidationArgs = []string{
	"--set", "gateway.validation.failurePolicy=Fail",
	"--set", "gateway.validation.allowWarnings=false",
	"--set", "gateway.validation.alwaysAcceptResources=false",
}

func runAndCleanCommand(name string, arg ...string) []byte {
	cmd := exec.Command(name, arg...)
	b, err := cmd.Output()
	// for debugging in Cloud Build
	if err != nil {
		if v, ok := err.(*exec.ExitError); ok {
			fmt.Println("ExitError: ", string(v.Stderr))
		}
	}
	Expect(err).To(BeNil())
	cmd.Process.Kill()
	cmd.Process.Release()
	return b
}

func checkGlooHealthy(testHelper *helper.SoloTestHelper) {
	for _, deploymentName := range glooDeploymentsToCheck {
		runAndCleanCommand("kubectl", "rollout", "status", "deployment", "-n", testHelper.InstallNamespace, deploymentName)
	}
	kube2e.GlooctlCheckEventuallyHealthy(2, testHelper, "90s")
}

func GetEnvoyCfgDump(testHelper *helper.SoloTestHelper) string {
	contextWithCancel, cancel := context.WithCancel(context.Background())
	defer cancel()
	opts := &options.Options{
		Metadata: core.Metadata{
			Namespace: testHelper.InstallNamespace,
		},
		Top: options.Top{
			Ctx: contextWithCancel,
		},
		Proxy: options.Proxy{
			Name: "gateway-proxy",
		},
	}

	cfg, err := gateway.GetEnvoyCfgDump(opts)
	Expect(err).NotTo(HaveOccurred())
	return cfg
}

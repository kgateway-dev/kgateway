package upgrade

import (
	"context"
	"fmt"
	"strings"

	exec_utils "github.com/solo-io/go-utils/testutils/exec"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/k8s-utils/testutils/helper"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)

// ===================================
// Repeated Test Code
// ===================================
// Based case test for local runs to help narrow down failures
func baseUpgradeTest(ctx context.Context, testHelper *helper.SoloTestHelper, crdDir string, startingVersion string, chartUri string, strictValidation bool) {
	By(fmt.Sprintf("should start with gloo version %s", startingVersion))
	Expect(fmt.Sprintf("v%s", GetGlooServerVersion(ctx, testHelper.InstallNamespace))).To(Equal(startingVersion))

	// upgrade to the gloo version being tested
	GlooToBranchVersion(testHelper, chartUri, crdDir, strictValidation, nil)

	By("should have upgraded to the gloo version being tested")
	Expect(GetGlooServerVersion(ctx, testHelper.InstallNamespace)).To(Equal(testHelper.ChartVersion()))
}

func UpdateSettingsWithoutErrors(ctx context.Context, testHelper *helper.SoloTestHelper, startingVersion string, crdDir string, chartUri string, strictValidation bool) {
	By(fmt.Sprintf("should start with gloo version %s", startingVersion))
	Expect(fmt.Sprintf("v%s", GetGlooServerVersion(ctx, testHelper.InstallNamespace))).To(Equal(startingVersion))

	By("should start with the settings.invalidConfigPolicy.invalidRouteResponseCode=404")
	client := helpers.MustSettingsClient(ctx)
	settings, err := client.Read(testHelper.InstallNamespace, defaults.SettingsName, clients.ReadOpts{})
	Expect(err).To(BeNil())
	Expect(settings.GetGloo().GetInvalidConfigPolicy().GetInvalidRouteResponseCode()).To(Equal(uint32(404)))

	GlooToBranchVersion(testHelper, chartUri, crdDir, strictValidation, []string{
		"--set", "settings.replaceInvalidRoutes=true",
		"--set", "settings.invalidConfigPolicy.invalidRouteResponseCode=400",
		"--set", "gateway.validation.validationServerGrpcMaxSizeBytes=5000000",
	})

	By("should have updated to settings.invalidConfigPolicy.invalidRouteResponseCode=400")
	settings, err = client.Read(testHelper.InstallNamespace, defaults.SettingsName, clients.ReadOpts{})
	Expect(err).To(BeNil())
	Expect(settings.GetGloo().GetInvalidConfigPolicy().GetInvalidRouteResponseCode()).To(Equal(uint32(400)))
	Expect(settings.GetGateway().GetValidation().GetValidationServerGrpcMaxSizeBytes().GetValue()).To(Equal(int32(5000000)))
}

func AddSecondGatewayProxySeparateNamespaceTest(testHelper *helper.SoloTestHelper, crdDir string, chartUri string, strictValidation bool) {
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

	RunAndCleanCommand("kubectl", "create", "ns", externalNamespace)
	defer RunAndCleanCommand("kubectl", "delete", "ns", externalNamespace)

	GlooToBranchVersion(testHelper, chartUri, crdDir, strictValidation, settings)

	// Ensures deployment is created for both default namespace and external one
	// Note - name of external deployments is kebab-case of gatewayProxies NAME helm value
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
}

func UpdateValidationWebhookTests(ctx context.Context, crdDir string, kubeClientset kubernetes.Interface, testHelper *helper.SoloTestHelper, chartUri string, strictValidation bool) {
	webhookConfigClient := kubeClientset.AdmissionregistrationV1().ValidatingWebhookConfigurations()
	secretClient := kubeClientset.CoreV1().Secrets(testHelper.InstallNamespace)

	By("the webhook caBundle should be the same as the secret's root ca value")
	webhookConfig, err := webhookConfigClient.Get(ctx, "gloo-gateway-validation-webhook-"+testHelper.InstallNamespace, metav1.GetOptions{})
	Expect(err).NotTo(HaveOccurred())
	secret, err := secretClient.Get(ctx, "gateway-validation-certs", metav1.GetOptions{})
	Expect(err).NotTo(HaveOccurred())
	Expect(webhookConfig.Webhooks[0].ClientConfig.CABundle).To(Equal(secret.Data[corev1.ServiceAccountRootCAKey]))

	GlooToBranchVersion(testHelper, chartUri, crdDir, strictValidation, nil)

	By("the webhook caBundle and secret's root ca value should still match after upgrade")
	webhookConfig, err = webhookConfigClient.Get(ctx, "gloo-gateway-validation-webhook-"+testHelper.InstallNamespace, metav1.GetOptions{})
	Expect(err).NotTo(HaveOccurred())
	secret, err = secretClient.Get(ctx, "gateway-validation-certs", metav1.GetOptions{})
	Expect(err).NotTo(HaveOccurred())
	Expect(webhookConfig.Webhooks[0].ClientConfig.CABundle).To(Equal(secret.Data[corev1.ServiceAccountRootCAKey]))
}

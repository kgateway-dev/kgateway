package tests_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/envutils"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	. "github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/tests"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/testutils/install"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

// TestAIExtensions tests the AI extension functionality
func TestAIExtensions(t *testing.T) {
	ctx := context.Background()
	installNs, nsEnvPredefined := envutils.LookupOrDefault(testutils.InstallNamespace, "ai-test")
	testInstallation := e2e.CreateTestInstallation(
		t,
		&install.Context{
			InstallNamespace:          installNs,
			ProfileValuesManifestFile: e2e.AIValuesManifestPath,
			ValuesManifestFile:        e2e.EmptyValuesManifestPath,
		},
	)

	// Set the env to the install namespace if it is not already set
	if !nsEnvPredefined {
		os.Setenv(testutils.InstallNamespace, installNs)
	}

	// We register the cleanup function _before_ we actually perform the installation.
	// This allows us to uninstall kgateway, in case the original installation only completed partially
	t.Cleanup(func() {
		if !nsEnvPredefined {
			os.Unsetenv(testutils.InstallNamespace)
		}
		if t.Failed() {
			testInstallation.PreFailHandler(ctx)
		}

		testInstallation.UninstallKgateway(ctx)
		cleanupMockProvider(ctx, testInstallation, installNs)
	})

	// Install kgateway
	testInstallation.InstallKgatewayFromLocalChart(ctx)
	err := bootstrapEnv(ctx, testInstallation, installNs)
	if err != nil {
		t.Error(err)
	}

	// Install provider mock app
	installProviderMockApp(ctx, testInstallation, installNs)

	AIGatewaySuiteRunner().Run(ctx, t, testInstallation)
}

// Create a secret for the AI extension
func bootstrapEnv(
	ctx context.Context,
	testInstallation *e2e.TestInstallation,
	installNamespace string,
) error {
	// note: e2e tests are currently using the mock provider
	openaiKey := "fake-openai-key"
	azureOpenAiKey := "fake-azure-openai-key"
	geminiKey := "fake-gemini-key"
	vertexAITokenStr := "fake-vertex-ai-token"

	secretsMap := map[string]map[string]string{
		"openai-secret":    {"Authorization": "Bearer " + openaiKey},
		"azure-secret":     {"Authorization": azureOpenAiKey},
		"gemini-secret":    {"Authorization": geminiKey},
		"vertex-ai-secret": {"Authorization": vertexAITokenStr},
	}

	for name, data := range secretsMap {
		err := createOrUpdateSecret(ctx, testInstallation, installNamespace, name, data)
		if err != nil {
			return err
		}
	}

	return nil
}

func createOrUpdateSecret(
	ctx context.Context,
	testInstallation *e2e.TestInstallation,
	namespace string,
	name string,
	data map[string]string,
) error {
	resource := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		StringData: data,
	}
	err := testInstallation.ClusterContext.Client.Create(ctx, resource)
	if err != nil {
		err = testInstallation.ClusterContext.Client.Update(ctx, resource)
		if err != nil {
			return fmt.Errorf("failed to create or update %s: %s", name, err.Error())
		}
	}

	return nil
}

func getMockProviderYAML(namespace, image string) string {
	return fmt.Sprintf(`
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-ai-provider
  namespace: %s
  labels:
    app: test-ai-provider
spec:
  replicas: 1
  selector:
    matchLabels:
      app: test-ai-provider
  template:
    metadata:
      labels:
        app: test-ai-provider
    spec:
      containers:
        - name: test-ai-provider
          image: %s
          imagePullPolicy: IfNotPresent
          ports:
            - containerPort: 5001
          resources:
            requests:
              cpu: 100m
              memory: 128Mi
            limits:
              cpu: 500m
              memory: 256Mi
---
apiVersion: v1
kind: Service
metadata:
  name: test-ai-provider
  namespace: %s
spec:
  selector:
    app: test-ai-provider
  ports:
    - port: 5001
      targetPort: 5001
  type: ClusterIP`, namespace, image, namespace)
}

func cleanupMockProvider(ctx context.Context, testInstallation *e2e.TestInstallation, namespace string) {
	// Use empty image as it's not needed for cleanup
	yaml := getMockProviderYAML(namespace, "")
	err := testInstallation.ClusterContext.Cli.Delete(ctx, []byte(yaml))
	if err != nil {
		fmt.Printf("Warning: Failed to cleanup mock provider: %v\n", err)
	}
}

func installProviderMockApp(ctx context.Context, testInstallation *e2e.TestInstallation, namespace string) {
	// Get version from environment variable or use default image
	version := os.Getenv("VERSION")
	image := "ghcr.io/kgateway-dev/test-ai-provider:1.0.0-dev"
	if version != "" {
		image = fmt.Sprintf("ghcr.io/kgateway-dev/test-ai-provider:%s", version)
	}

	yaml := getMockProviderYAML(namespace, image)
	err := testInstallation.ClusterContext.Cli.Apply(ctx, []byte(yaml))
	if err != nil {
		panic(fmt.Sprintf("Failed to install mock provider: %v", err))
	}
}

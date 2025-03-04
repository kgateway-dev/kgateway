package tests_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
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
	})

	// Install kgateway
	testInstallation.InstallKgatewayFromLocalChart(ctx)
	err := bootstrapEnv(ctx, testInstallation, installNs)
	if err != nil {
		t.Error(err)
	}

	AIGatewaySuiteRunner().Run(ctx, t, testInstallation)
}

// Create a secret for the AI extension
func bootstrapEnv(
	ctx context.Context,
	testInstallation *e2e.TestInstallation,
	installNamespace string,
) error {
	openaiKey, ok := os.LookupEnv("OPENAI_API_KEY")
	if !ok {
		return fmt.Errorf("OPENAI_API_KEY environment variable not set")
	}
	mistralKey, ok := os.LookupEnv("MISTRAL_API_KEY")
	if !ok {
		return fmt.Errorf("MISTRAL_API_KEY environment variable not set")
	}
	anthropicKey, ok := os.LookupEnv("ANTHROPIC_API_KEY")
	if !ok {
		return fmt.Errorf("ANTHROPIC_API_KEY environment variable not set")
	}
	azureOpenAiKey, ok := os.LookupEnv("AZURE_OPENAI_API_KEY")
	if !ok {
		return fmt.Errorf("AZURE_OPENAI_API_KEY environment variable not set")
	}
	geminiKey, ok := os.LookupEnv("GEMINI_API_KEY")
	if !ok {
		return fmt.Errorf("GEMINI_API_KEY environment variable not set")
	}

	vertexAITokenEnv, _ := os.LookupEnv("VERTEX_AI_AUTH_TOKEN")
	vertexAITokenStr := vertexAITokenEnv
	if vertexAITokenEnv == "" {
		var err error
		vertexAITokenStr, err = getVertexAIToken()
		if err != nil {
			return fmt.Errorf("failed to get Vertex AI token %s", err.Error())
		}
	}

	secretsMap := map[string]map[string]string{
		"openai-secret":    {"Authorization": "Bearer " + openaiKey},
		"mistralai-secret": {"Authorization": "Bearer " + mistralKey},
		"anthropic-secret": {"Authorization": anthropicKey},
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

func getVertexAIToken() (string, error) {
	cmd := exec.Command("gcloud", "auth", "print-access-token",
		"ci-cloud-run@gloo-ee.iam.gserviceaccount.com", "--project", "gloo-ee")
	vertexAIToken, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get access token: %s", err.Error())
	}
	return string(bytes.TrimSpace(vertexAIToken)), nil
}

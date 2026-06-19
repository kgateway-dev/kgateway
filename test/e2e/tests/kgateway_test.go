//go:build e2e

package tests_test

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/envutils"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/common"
	. "github.com/kgateway-dev/kgateway/v2/test/e2e/tests"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/testutils/install"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

func TestKgateway(t *testing.T) {
	ctx := context.Background()
	installNs, nsEnvPredefined := envutils.LookupOrDefault(testutils.InstallNamespace, "kgateway-test")
	installContext := &install.Context{
		InstallNamespace:          installNs,
		ProfileValuesManifestFile: e2e.CommonRecommendationManifest,
		ValuesManifestFile:        e2e.EmptyValuesManifestPath,
		ExtraHelmArgs: []string{
			"--set", "controller.enableAwsEc2Discovery=true",
			"--set", "controller.extraEnv.KGW_GLOBAL_POLICY_NAMESPACE=" + installNs,
			"--set", "policyMerge.trafficPolicy.extProc=DeepMerge",
		},
	}
	testInstallation := e2e.CreateTestInstallation(
		t,
		installContext,
	)
	if localstackEndpoint, ok := lookupLocalstackEndpoint(t, ctx, testInstallation); ok {
		installContext.ExtraHelmArgs = append(
			installContext.ExtraHelmArgs,
			"--set-string", "controller.extraEnv.AWS_ENDPOINT_URL_EC2="+localstackEndpoint,
		)
	}

	// Set the env to the install namespace if it is not already set
	if !nsEnvPredefined {
		os.Setenv(testutils.InstallNamespace, installNs)
	}

	// We register the cleanup function _before_ we actually perform the installation.
	// This allows us to uninstall kgateway, in case the original installation only completed partially
	testutils.Cleanup(t, func() {
		if !nsEnvPredefined {
			os.Unsetenv(testutils.InstallNamespace)
		}

		testInstallation.UninstallKgateway(ctx, t)
	})

	// Install kgateway
	testInstallation.InstallKgatewayFromLocalChart(ctx, t)

	// The base gateway has two variants: one with "allowedListeners" to support ListenerSets to and one without.
	// The gateway to apply is selected based on whether the installed Gateway API version supports ListenerSets.
	// If the gateway with "allowedListeners"  is applied with a Gateway API version that doesn't support it, the resource will be rejected.

	channel, version, err := base.DetectGwApiInfo(ctx, testInstallation.ClusterContext.Client)
	if err != nil {
		t.Fatalf("failed to detect Gateway API version: %v", err)
	}
	gatewayManifest := "kgateway-base-gateway.yaml"
	if base.SupportsListenerSets(channel, version) {
		gatewayManifest = "kgateway-base-gateway-listenersets.yaml"
	}

	// Apply the base gateway once, then the shared nginx backend that suites reference
	// instead of each deploying their own.
	common.SetupBaseConfig(ctx, t, testInstallation,
		filepath.Join("manifests", "kgateway-base.yaml"),
		filepath.Join("manifests", gatewayManifest),
	)
	common.SetupSharedNginxBackend(ctx, t, testInstallation)
	common.SetupBaseGateway(ctx, t, testInstallation, types.NamespacedName{
		Namespace: "kgateway-base",
		Name:      "gateway",
	})

	KubeGatewaySuiteRunner().Run(ctx, t, testInstallation)
}

func lookupLocalstackEndpoint(t *testing.T, ctx context.Context, testInstallation *e2e.TestInstallation) (string, bool) {
	t.Helper()
	localstackService := &corev1.Service{}
	err := testInstallation.ClusterContext.Client.Get(ctx, client.ObjectKey{
		Namespace: "localstack",
		Name:      "localstack",
	}, localstackService)
	if apierrors.IsNotFound(err) {
		return "", false
	}
	if err != nil {
		t.Fatalf("failed to get localstack service: %v", err)
	}
	if len(localstackService.Spec.Ports) == 0 || localstackService.Spec.Ports[0].NodePort == 0 {
		t.Fatal("localstack service is missing a node port")
	}

	var nodes corev1.NodeList
	if err := testInstallation.ClusterContext.Client.List(ctx, &nodes); err != nil {
		t.Fatalf("failed to list cluster nodes: %v", err)
	}
	if len(nodes.Items) == 0 {
		t.Fatal("cluster must have at least one node")
	}

	var nodeIP string
	for _, node := range nodes.Items {
		for _, addr := range node.Status.Addresses {
			if addr.Type == corev1.NodeInternalIP {
				nodeIP = addr.Address
				break
			}
		}
		if nodeIP != "" {
			break
		}
	}
	if nodeIP == "" {
		t.Fatal("failed to determine localstack node internal IP")
	}

	localstackURL := fmt.Sprintf("http://%s:%d", nodeIP, localstackService.Spec.Ports[0].NodePort)
	parsed, err := url.Parse(localstackURL)
	if err != nil {
		t.Fatalf("failed to parse localstack URL: %v", err)
	}
	return parsed.String(), true
}

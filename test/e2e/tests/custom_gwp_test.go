//go:build e2e

package tests

import (
	"os"
	"testing"

	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/envutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/testutils/install"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

var kgatewayGWP = `
apiVersion: gateway.kgateway.dev/v1alpha1
kind: GatewayParameters
metadata:
  name: custom-gwp
  namespace: kgateway-test
spec:
  kube:
    podTemplate:
      extraLabels:
        custom: custom-label
`

var kgatewayGateway = `
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: gw
spec:
  gatewayClassName: kgateway
  listeners:
    - protocol: HTTP
      port: 8080
      name: http
`

// TestCustomGWP tests that the helm chart's gatewayClassParametersRefs configures
// the default GatewayClass parametersRef correctly.
// The test installs CRDs, creates the custom GatewayParameters resource, installs kgateway,
// verifies that the GatewayClass parametersRef is configured correctly, creates a Gateway,
// and verifies that the gateway pod has the custom label defined in the GatewayParameters.
func TestCustomGWP(t *testing.T) {
	ctx := t.Context()
	installNs, nsEnvPredefined := envutils.LookupOrDefault(testutils.InstallNamespace, "kgateway-test")
	testInstallation := e2e.CreateTestInstallation(
		t,
		&install.Context{
			InstallNamespace:          installNs,
			ProfileValuesManifestFile: e2e.CommonRecommendationManifest,
			ValuesManifestFile:        e2e.ManifestPath("custom-gwp.yaml"),
		},
	)

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
		if t.Failed() {
			testInstallation.PreFailHandler(ctx)
		}

		testInstallation.UninstallKgateway(ctx)
	})

	// install CRDs
	testInstallation.InstallKgatewayCRDsFromLocalChart(ctx)

	// create GatewayParameters for kgateway
	err := testInstallation.Actions.Kubectl().Apply(ctx, []byte(kgatewayGWP))
	if err != nil {
		t.Fatalf("failed to create GatewayParameters: %v", err)
	}

	// install kgateway
	testInstallation.InstallKgatewayFromLocalChart(ctx)

	// Wait for GatewayClass to be created
	testInstallation.Assertions.EventuallyObjectsExist(ctx, &gwv1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{Name: "kgateway"},
	})

	// create Gateway
	err = testInstallation.Actions.Kubectl().Apply(ctx, []byte(kgatewayGateway))
	if err != nil {
		t.Fatalf("failed to create Gateway: %v", err)
	}

	// Verify GatewayClass has correct parametersRef
	gc := &gwv1.GatewayClass{}
	err = testInstallation.ClusterContext.Client.Get(ctx, client.ObjectKey{Name: "kgateway"}, gc)
	if err != nil {
		t.Fatalf("failed to get GatewayClass: %v", err)
	}

	if gc.Spec.ParametersRef == nil {
		t.Fatal("GatewayClass spec.parametersRef is nil")
	}

	if gc.Spec.ParametersRef.Name != "custom-gwp" {
		t.Fatalf("expected GatewayClass parametersRef.name to be 'custom-gwp', got '%s'", gc.Spec.ParametersRef.Name)
	}

	expectedNamespace := gwv1.Namespace("kgateway-test")
	if gc.Spec.ParametersRef.Namespace == nil || *gc.Spec.ParametersRef.Namespace != expectedNamespace {
		t.Fatalf("expected GatewayClass parametersRef.namespace to be '%s', got '%v'", expectedNamespace, gc.Spec.ParametersRef.Namespace)
	}

	// Wait for Gateway to be accepted and deployment created
	gatewayNamespace := "default"
	proxyObjectMeta := metav1.ObjectMeta{
		Name:      "gw",
		Namespace: gatewayNamespace,
	}
	testInstallation.Assertions.EventuallyReadyReplicas(ctx, proxyObjectMeta, gomega.Equal(1))

	// Verify the gateway pod has the custom label
	pods, err := kubeutils.GetReadyPodsForDeployment(ctx, testInstallation.ClusterContext.Clientset, proxyObjectMeta)
	if err != nil {
		t.Fatalf("failed to get ready pods for deployment: %v", err)
	}
	if len(pods) == 0 {
		t.Fatal("no ready pods found for deployment")
	}

	pod := &corev1.Pod{}
	err = testInstallation.ClusterContext.Client.Get(ctx, client.ObjectKey{
		Namespace: gatewayNamespace,
		Name:      pods[0],
	}, pod)
	if err != nil {
		t.Fatalf("failed to get pod: %v", err)
	}

	if pod.Labels == nil {
		t.Fatal("pod labels are nil")
	}

	customLabelValue, ok := pod.Labels["custom"]
	if !ok {
		t.Fatal("pod does not have 'custom' label")
	}

	if customLabelValue != "custom-label" {
		t.Fatalf("expected pod label 'custom' to be 'custom-label', got '%s'", customLabelValue)
	}
}

//go:build e2e

package tests

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/envutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/helmutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/testutils/helper"
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

var kgatewayGWP2 = `
apiVersion: gateway.kgateway.dev/v1alpha1
kind: GatewayParameters
metadata:
  name: custom-gwp-2
  namespace: kgateway-test
spec:
  kube:
    podTemplate:
      extraLabels:
        another: label
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

const gatewayNamespace = "default"

var proxyObjectMeta = metav1.ObjectMeta{
	Name:      "gw",
	Namespace: gatewayNamespace,
}

// TestCustomGWP tests that the helm chart's gatewayClassParametersRefs configures
// the default GatewayClass parametersRef correctly.
// The test installs CRDs, creates the custom GatewayParameters resource, installs kgateway,
// verifies that the GatewayClass parametersRef is configured correctly, creates a Gateway,
// and verifies that the gateway pod has the custom label defined in the GatewayParameters.
// It then upgrades the helm chart to reference a different GatewayParameters resource,
// verifies that the GatewayClass parametersRef is updated correctly,
// and verifies that the gateway pod is still running (even though the ParametersRef has changed to non-existent resource).
// It then creates the new GatewayParameters resource,
// and verifies that the gateway pod has the new label defined in the new GatewayParameters resource.
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

	// We register the cleanup function before we actually perform the installation.
	// This allows us to uninstall kgateway, in case the original installation only completed partially
	testutils.Cleanup(t, func() {
		ctx := context.Background() // when you have a custom Cleanup, you can't use t.Context within it as the context is canceled before t's cleanup is called
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
	testInstallation.Assertions.EventuallyReadyReplicas(ctx, proxyObjectMeta, gomega.Equal(1))

	// Verify the gateway pod has the custom label
	verifyPodLabel(t, ctx, testInstallation, "custom", "custom-label", "")

	// Upgrade Helm to reference different ParametersRef (kgatewayGWP2)
	chartUri, err := helper.GetLocalChartPath(helmutils.ChartName, "")
	if err != nil {
		t.Fatalf("failed to get chart path: %v", err)
	}
	err = testInstallation.Actions.Helm().WithReceiver(os.Stdout).Upgrade(
		ctx,
		helmutils.InstallOpts{
			Namespace:       installNs,
			CreateNamespace: true,
			ValuesFiles:     []string{e2e.CommonRecommendationManifest, e2e.ManifestPath("custom-gwp-2.yaml")},
			ReleaseName:     helmutils.ChartName,
			ChartUri:        chartUri,
		})
	if err != nil {
		t.Fatalf("failed to upgrade Helm: %v", err)
	}
	testInstallation.Assertions.EventuallyKgatewayInstallSucceeded(ctx)

	// Verify GatewayClass is updated with new ref
	r := require.New(t)
	r.EventuallyWithT(func(c *assert.CollectT) {
		gcUpdated := &gwv1.GatewayClass{}
		err := testInstallation.ClusterContext.Client.Get(ctx, client.ObjectKey{Name: "kgateway"}, gcUpdated)
		assert.NoError(c, err, "failed to get GatewayClass after upgrade")
		assert.NotNil(c, gcUpdated.Spec.ParametersRef, "GatewayClass spec.parametersRef is nil after upgrade")
		assert.Equal(c, "custom-gwp-2", gcUpdated.Spec.ParametersRef.Name, "expected GatewayClass parametersRef.name to be 'custom-gwp-2' after upgrade")
		assert.NotNil(c, gcUpdated.Spec.ParametersRef.Namespace, "GatewayClass spec.parametersRef.namespace is nil after upgrade")
		assert.Equal(c, expectedNamespace, *gcUpdated.Spec.ParametersRef.Namespace, "expected GatewayClass parametersRef.namespace to be '%s' after upgrade", expectedNamespace)
	}, 20*time.Second, 200*time.Millisecond)

	// Ensure gateway pods are still running (even though the ParametersRef has changed to non-existent resource)
	testInstallation.Assertions.EventuallyReadyReplicas(ctx, proxyObjectMeta, gomega.Equal(1))

	// Create the kgatewayGWP2 GatewayParameters resource
	err = testInstallation.Actions.Kubectl().Apply(ctx, []byte(kgatewayGWP2))
	if err != nil {
		t.Fatalf("failed to create GatewayParameters kgatewayGWP2: %v", err)
	}

	// Wait for Gateway to reconcile with new parameters
	// The Gateway should pick up the new GatewayParameters and update the pods
	testInstallation.Assertions.EventuallyReadyReplicas(ctx, proxyObjectMeta, gomega.Equal(1))

	// Assert that eventually the deployment gateway pod is updated with the new label
	r.EventuallyWithT(func(c *assert.CollectT) {
		pods, err := kubeutils.GetReadyPodsForDeployment(ctx, testInstallation.ClusterContext.Clientset, proxyObjectMeta)
		assert.NoError(c, err, "failed to get ready pods for deployment after upgrade")
		assert.NotEmpty(c, pods, "no ready pods found for deployment after upgrade")

		pod := &corev1.Pod{}
		err = testInstallation.ClusterContext.Client.Get(ctx, client.ObjectKey{
			Namespace: gatewayNamespace,
			Name:      pods[0],
		}, pod)
		assert.NoError(c, err, "failed to get pod after upgrade")
		assert.NotNil(c, pod.Labels, "pod labels are nil after upgrade")
		assert.Contains(c, pod.Labels, "another", "pod should have the 'another' label after upgrade")
		assert.Equal(c, "label", pod.Labels["another"], "pod should have the new label 'another: label' after upgrade")
	}, 15*time.Second, 200*time.Millisecond)
}

// verifyPodLabel checks that a pod for the given deployment has the specified label with the expected value.
func verifyPodLabel(
	t *testing.T,
	ctx context.Context,
	testInstallation *e2e.TestInstallation,
	labelKey string,
	expectedValue string,
	errorContext string,
) {
	pods, err := kubeutils.GetReadyPodsForDeployment(ctx, testInstallation.ClusterContext.Clientset, proxyObjectMeta)
	if err != nil {
		t.Fatalf("failed to get ready pods for deployment %s: %v", errorContext, err)
	}
	if len(pods) == 0 {
		t.Fatalf("no ready pods found for deployment %s", errorContext)
	}

	pod := &corev1.Pod{}
	err = testInstallation.ClusterContext.Client.Get(ctx, client.ObjectKey{
		Namespace: gatewayNamespace,
		Name:      pods[0],
	}, pod)
	if err != nil {
		t.Fatalf("failed to get pod %s: %v", errorContext, err)
	}

	if pod.Labels == nil {
		t.Fatalf("pod labels are nil %s", errorContext)
	}

	labelValue, ok := pod.Labels[labelKey]
	if !ok {
		t.Fatalf("pod does not have '%s' label %s", labelKey, errorContext)
	}

	if labelValue != expectedValue {
		t.Fatalf("expected pod label '%s' to be '%s' %s, got '%s'", labelKey, expectedValue, errorContext, labelValue)
	}
}

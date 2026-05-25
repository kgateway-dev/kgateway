//go:build e2e

// Package customgwp holds the body of the TestCustomGWP e2e test.
package customgwp

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
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

// Run executes the TestCustomGWP scenario against the Installation produced by
// the supplied factory.
//
// The scenario:
//   - install CRDs and create a custom GatewayParameters
//   - install the control plane and verify the default GatewayClass.parametersRef
//   - create a Gateway and verify the pod inherits the custom label
//   - helm-upgrade the control plane to a different GatewayParameters ref
//   - verify the GatewayClass updates and the deployment stays Ready even
//     while that ref is dangling
//   - create the second GatewayParameters and verify the pod picks up the new
//     label
func Run(t *testing.T, factory e2e.InstallationFactory) {
	ctx := t.Context()
	installNs, nsEnvPredefined := envutils.LookupOrDefault(testutils.InstallNamespace, "kgateway-test")
	rolloutStabilityValuesFile := e2e.ManifestPath("custom-gwp-rollout-stability.yaml")
	testInstallation := factory(
		t,
		&install.Context{
			InstallNamespace:          installNs,
			ProfileValuesManifestFile: e2e.CommonRecommendationManifest,
			ValuesManifestFile:        e2e.ManifestPath("custom-gwp.yaml"),
			ExtraHelmArgs:             []string{"--values", rolloutStabilityValuesFile},
		},
	)

	if !nsEnvPredefined {
		os.Setenv(testutils.InstallNamespace, installNs)
	}

	// Register cleanup before installing so a partial install is still torn down.
	testutils.Cleanup(t, func() {
		ctx := context.Background() // t.Context() is canceled before t's cleanup runs
		if !nsEnvPredefined {
			os.Unsetenv(testutils.InstallNamespace)
		}
		if t.Failed() {
			testInstallation.PreFailHandler(ctx, t)
		}
		testInstallation.UninstallKgateway(ctx, t)
	})

	testInstallation.InstallKgatewayCRDsFromLocalChart(ctx, t)

	// Drop any kgateway-managed GatewayClass left over from a prior run. The
	// controller creates these via SSA, so `helm uninstall` doesn't reap them;
	// a stale entry with the wrong parametersRef races the reconciler below.
	deleteStaleKgatewayGatewayClasses(ctx, t, testInstallation)

	if err := testInstallation.ActionsProvider().Kubectl().Apply(ctx, []byte(kgatewayGWP)); err != nil {
		t.Fatalf("failed to create GatewayParameters: %v", err)
	}

	testInstallation.InstallKgatewayFromLocalChart(ctx, t)

	testInstallation.Assertions(t).EventuallyObjectsExist(ctx, &gwv1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{Name: wellknown.DefaultGatewayClassName},
	})

	if err := testInstallation.ActionsProvider().Kubectl().Apply(ctx, []byte(kgatewayGateway)); err != nil {
		t.Fatalf("failed to create Gateway: %v", err)
	}

	// Poll for the controller to populate parametersRef. EventuallyObjectsExist
	// above only proves the GatewayClass exists, not that the controller has
	// reconciled it.
	r := require.New(t)
	expectedNamespace := gwv1.Namespace("kgateway-test")
	r.EventuallyWithT(func(c *assert.CollectT) {
		gc := &gwv1.GatewayClass{}
		err := testInstallation.Cluster().Client.Get(ctx, client.ObjectKey{Name: wellknown.DefaultGatewayClassName}, gc)
		require.NoError(c, err, "failed to get kgateway GatewayClass")
		require.NotNil(c, gc.Spec.ParametersRef, "kgateway GatewayClass spec.parametersRef is nil")
		assert.Equal(c, "custom-gwp", gc.Spec.ParametersRef.Name, "expected kgateway GatewayClass parametersRef.name to be 'custom-gwp'")
		require.NotNil(c, gc.Spec.ParametersRef.Namespace, "kgateway GatewayClass spec.parametersRef.namespace is nil")
		assert.Equal(c, expectedNamespace, *gc.Spec.ParametersRef.Namespace, "expected kgateway GatewayClass parametersRef.namespace to be '%s'", expectedNamespace)
		assert.Equal(c, gwv1.Group("gateway.kgateway.dev"), gc.Spec.ParametersRef.Group, "expected kgateway GatewayClass parametersRef.group to be 'gateway.kgateway.dev'")
		assert.Equal(c, gwv1.Kind("GatewayParameters"), gc.Spec.ParametersRef.Kind, "expected kgateway GatewayClass parametersRef.kind to be 'GatewayParameters'")
	}, 60*time.Second, 200*time.Millisecond)

	testInstallation.Assertions(t).EventuallyReadyReplicas(ctx, proxyObjectMeta, gomega.Equal(1))

	verifyPodLabel(t, ctx, testInstallation, "custom", "custom-label", "")

	// Upgrade to point at custom-gwp-2. Chart name and release name come from
	// the Installation; the test only owns values files and helm extra args.
	testInstallation.UpgradeKgatewayCore(
		ctx,
		t,
		[]string{
			e2e.CommonRecommendationManifest,
			e2e.ManifestPath("custom-gwp-2.yaml"),
			rolloutStabilityValuesFile,
		},
		[]string{"--wait", "--timeout", "2m"},
	)
	testInstallation.Assertions(t).EventuallyKgatewayInstallSucceeded(ctx)

	r.EventuallyWithT(func(c *assert.CollectT) {
		gcUpdated := &gwv1.GatewayClass{}
		err := testInstallation.Cluster().Client.Get(ctx, client.ObjectKey{Name: wellknown.DefaultGatewayClassName}, gcUpdated)
		require.NoError(c, err, "failed to get kgateway GatewayClass after upgrade")
		require.NotNil(c, gcUpdated.Spec.ParametersRef, "kgateway GatewayClass spec.parametersRef is nil after upgrade")
		assert.Equal(c, "custom-gwp-2", gcUpdated.Spec.ParametersRef.Name, "expected kgateway GatewayClass parametersRef.name to be 'custom-gwp-2' after upgrade")
		require.NotNil(c, gcUpdated.Spec.ParametersRef.Namespace, "kgateway GatewayClass spec.parametersRef.namespace is nil after upgrade")
		assert.Equal(c, expectedNamespace, *gcUpdated.Spec.ParametersRef.Namespace, "expected kgateway GatewayClass parametersRef.namespace to be '%s' after upgrade", expectedNamespace)
	}, 60*time.Second, 200*time.Millisecond)

	// Even though parametersRef points at a non-existent resource for a moment,
	// pods must stay running.
	testInstallation.Assertions(t).EventuallyReadyReplicas(ctx, proxyObjectMeta, gomega.Equal(1))

	if err := testInstallation.ActionsProvider().Kubectl().Apply(ctx, []byte(kgatewayGWP2)); err != nil {
		t.Fatalf("failed to create GatewayParameters kgatewayGWP2: %v", err)
	}

	testInstallation.Assertions(t).EventuallyReadyReplicas(ctx, proxyObjectMeta, gomega.Equal(1))

	r.EventuallyWithT(func(c *assert.CollectT) {
		pods, err := kubeutils.GetReadyPodsForDeployment(ctx, testInstallation.Cluster().Clientset, proxyObjectMeta)
		require.NoError(c, err, "failed to get ready pods for deployment after upgrade")
		require.NotEmpty(c, pods, "no ready pods found for deployment after upgrade")

		pod := &corev1.Pod{}
		err = testInstallation.Cluster().Client.Get(ctx, client.ObjectKey{
			Namespace: gatewayNamespace,
			Name:      pods[0],
		}, pod)
		require.NoError(c, err, "failed to get pod after upgrade")
		assert.NotNil(c, pod.Labels, "pod labels are nil after upgrade")
		assert.Contains(c, pod.Labels, "another", "pod should have the 'another' label after upgrade")
		assert.Equal(c, "label", pod.Labels["another"], "pod should have the new label 'another: label' after upgrade")
	}, 60*time.Second, 2*time.Second)
}

// deleteStaleKgatewayGatewayClasses removes any GatewayClass managed by the
// kgateway controller. Helm doesn't track these (the controller creates them
// via SSA) so they survive `helm uninstall` and leak state across tests.
func deleteStaleKgatewayGatewayClasses(ctx context.Context, t *testing.T, testInstallation e2e.Installation) {
	list := &gwv1.GatewayClassList{}
	if err := testInstallation.Cluster().Client.List(ctx, list); err != nil {
		t.Fatalf("failed to list GatewayClasses: %v", err)
	}
	for idx := range list.Items {
		gc := &list.Items[idx]
		if string(gc.Spec.ControllerName) != wellknown.DefaultGatewayControllerName {
			continue
		}
		if err := testInstallation.Cluster().Client.Delete(ctx, gc); err != nil && !apierrors.IsNotFound(err) {
			t.Fatalf("failed to delete stale GatewayClass %q: %v", gc.Name, err)
		}
	}
}

// verifyPodLabel waits for a pod of the gateway deployment to carry the
// expected label value. EventuallyWithT covers the lag between pod creation
// and GatewayParameters reconciliation.
func verifyPodLabel(
	t *testing.T,
	ctx context.Context,
	testInstallation e2e.Installation,
	labelKey string,
	expectedValue string,
	errorContext string,
) {
	require.New(t).EventuallyWithT(func(c *assert.CollectT) {
		pods, err := kubeutils.GetReadyPodsForDeployment(ctx, testInstallation.Cluster().Clientset, proxyObjectMeta)
		require.NoError(c, err, "failed to get ready pods for deployment %s", errorContext)
		require.NotEmpty(c, pods, "no ready pods found for deployment %s", errorContext)

		pod := &corev1.Pod{}
		err = testInstallation.Cluster().Client.Get(ctx, client.ObjectKey{
			Namespace: gatewayNamespace,
			Name:      pods[0],
		}, pod)
		require.NoError(c, err, "failed to get pod %s", errorContext)

		require.NotNil(c, pod.Labels, "pod labels are nil %s", errorContext)

		labelValue, ok := pod.Labels[labelKey]
		require.True(c, ok, fmt.Sprintf("pod does not have '%s' label %s", labelKey, errorContext))

		assert.Equal(c, expectedValue, labelValue, "expected pod label '%s' to be '%s' %s", labelKey, expectedValue, errorContext)
	}, 60*time.Second, 2*time.Second)
}

//go:build e2e

package bootstrappatches

import (
	"context"
	"net/http"
	"time"

	envoybootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	"github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/conditions"
	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/common"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
	"github.com/kgateway-dev/kgateway/v2/test/envoyutils/admincli"
	"github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

const (
	// checksumAnnotation is the pod-template annotation that holds the checksum of the
	// proxy's bootstrap, so that a bootstrap change rolls the pods.
	checksumAnnotation = "kgateway.dev/checksum-config"
	// managedPathsEnv is the controller setting that decides what happens to bootstrap
	// patches that set paths the control plane manages.
	managedPathsEnv = "KGW_BOOTSTRAP_PATCH_MANAGED_PATHS"

	// statusTimeout bounds waiting for the controller to report on the Gateway, which
	// can include a controller restart.
	statusTimeout = 2 * time.Minute
	// rolloutTimeout bounds waiting for the proxy pods to roll.
	rolloutTimeout = 3 * time.Minute
	pollInterval   = 2 * time.Second
)

// testingSuite tests the bootstrap patch in GatewayParameters
// (spec.kube.envoyContainer.bootstrap.patch) against a running proxy. The patch is
// merged in pkg/kgateway/bootstrappatch and deployed by pkg/kgateway/deployer.
type testingSuite struct {
	*base.BaseTestingSuite
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		BaseTestingSuite: base.NewBaseTestingSuite(ctx, testInst, setup, testCases),
	}
}

// TestOverlayApplied checks that patches that set no managed path are reported as
// applied and are in the bootstrap the proxy runs.
func (s *testingSuite) TestOverlayApplied() {
	// The controller reports the patches as applied, with nothing managed to report.
	s.eventuallyBootstrapPatchApplied(metav1.ConditionTrue, conditions.KgatewayReasonBootstrapPatchApplied)
	s.eventuallyRolledOut()

	// The proxy runs them: the flush interval, and the runtime layer just before the
	// admin layer, so a value set through /runtime_modify still wins.
	s.eventuallyProxyBootstrap(func(g gomega.Gomega, bs *envoybootstrapv3.Bootstrap) {
		g.Expect(bs.GetStatsFlushInterval().AsDuration()).To(gomega.Equal(7*time.Second), "stats_flush_interval should come from the patch")
		layers := runtimeLayerNames(bs)
		g.Expect(layers).To(gomega.HaveLen(2), "the runtime layers should be the patch's layer and the admin layer")
		g.Expect(layers).To(gomega.Equal([]string{"e2e_runtime", "admin_layer"}))
	})
}

// TestManagedPathModes checks each KGW_BOOTSTRAP_PATCH_MANAGED_PATHS mode with patches
// that set node, a path the control plane manages. It upgrades the controller to
// change the mode, and restores the default when it ends.
func (s *testingSuite) TestManagedPathModes() {
	// Restore the default mode, WARN, whatever this test leaves the controller in. A
	// defer runs while s.T() is still this test; t.Cleanup would run after testify
	// restores the suite's T.
	defer s.setManagedPathsMode("")

	// WARN, the default: the patches apply, and the condition reports the managed path.
	s.eventuallyBootstrapPatchApplied(metav1.ConditionTrue, conditions.KgatewayReasonBootstrapManagedPathsOverridden)
	s.eventuallyRolledOut()
	s.eventuallyProxyBootstrap(func(g gomega.Gomega, bs *envoybootstrapv3.Bootstrap) {
		g.Expect(bs.GetNode().GetMetadata().GetFields()).To(gomega.HaveKey("e2e_marker"), "WARN should apply the node metadata")
	})
	warnChecksum := s.podTemplateChecksum()

	// PRUNE: the managed path is dropped from the patches, the condition says so, and
	// the proxy rolls to a bootstrap without it.
	s.setManagedPathsMode(apisettings.BootstrapPatchManagedPathsPrune)
	s.eventuallyBootstrapPatchApplied(metav1.ConditionTrue, conditions.KgatewayReasonBootstrapPathsIgnored)
	s.eventuallyChecksumChangedFrom(warnChecksum)
	s.eventuallyRolledOut()
	s.eventuallyProxyBootstrap(func(g gomega.Gomega, bs *envoybootstrapv3.Bootstrap) {
		g.Expect(bs.GetNode().GetMetadata().GetFields()).NotTo(gomega.HaveKey("e2e_marker"), "PRUNE should drop the node metadata")
		g.Expect(bs.GetNode().GetMetadata().GetFields()).To(gomega.HaveKey("role"), "PRUNE should keep the generated node metadata")
	})
	pruneChecksum := s.podTemplateChecksum()

	// REJECT: none of the patches apply. The Gateway stays accepted, and its proxy keeps
	// the bootstrap it runs and keeps serving traffic.
	s.setManagedPathsMode(apisettings.BootstrapPatchManagedPathsReject)
	s.eventuallyBootstrapPatchApplied(metav1.ConditionFalse, conditions.KgatewayReasonBootstrapPathsDenied)
	s.TestInstallation.AssertionsT(s.T()).EventuallyGatewayCondition(s.Ctx, proxyObjectMeta.Name, proxyObjectMeta.Namespace,
		gwv1.GatewayConditionAccepted, metav1.ConditionTrue, statusTimeout)
	s.Require().Equal(pruneChecksum, s.podTemplateChecksum(), "REJECT should leave the Deployment as it was")
	s.gateway().Send(s.T(), &matchers.HttpResponse{StatusCode: http.StatusOK},
		curl.WithHostHeader("example.com"), curl.WithPort(8080), curl.WithPath("/status/200"))
}

// TestFragmentFeatureFlag checks the case bootstrap patches exist for: changing an
// Envoy runtime feature flag that kgateway has no field for. By default Envoy rejects a
// request whose path has a fragment; with
// envoy.reloadable_features.http_reject_path_with_fragment set to false, it strips the
// fragment and forwards the request.
func (s *testingSuite) TestFragmentFeatureFlag() {
	// Without patches, a request with a fragment gets a 400.
	gw := s.gateway()
	fragmentRequest := []curl.Option{
		curl.WithHostHeader("example.com"),
		curl.WithPort(8080),
		curl.WithRequestTarget("/anything#e2e-fragment"),
	}
	gw.Send(s.T(), &matchers.HttpResponse{StatusCode: http.StatusBadRequest}, fragmentRequest...)
	oldChecksum := s.podTemplateChecksum()

	// Set the flag to false with a static runtime layer.
	s.patchGatewayParameters(func(gwp *kgateway.GatewayParameters) {
		gwp.Spec.Kube.EnvoyContainer.Bootstrap.Patch = &kgateway.BootstrapPatch{
			MergeStrategy: kgateway.BootstrapMergeStrategyStructuredMerge,
			Value: apiextensionsv1.JSON{Raw: []byte(`{"layered_runtime": {"layers": [{
				"name": "fragments",
				"static_layer": {"envoy.reloadable_features.http_reject_path_with_fragment": false}
			}]}}`)},
		}
	})

	// The patches apply and the pods roll.
	s.eventuallyBootstrapPatchApplied(metav1.ConditionTrue, conditions.KgatewayReasonBootstrapPatchApplied)
	s.eventuallyChecksumChangedFrom(oldChecksum)
	s.eventuallyRolledOut()

	// The request reaches httpbin, which echoes the path without the fragment.
	gw.Send(s.T(), &matchers.HttpResponse{
		StatusCode: http.StatusOK,
		Body: gomega.And(
			gomega.ContainSubstring("example.com/anything"),
			gomega.Not(gomega.ContainSubstring("e2e-fragment")),
		),
	}, fragmentRequest...)
}

// gateway returns a handle for sending requests to the Gateway.
func (s *testingSuite) gateway() *common.Gateway {
	gw := common.NewGateway(s.Ctx, s.T(), s.TestInstallation, types.NamespacedName{
		Name:      proxyObjectMeta.Name,
		Namespace: proxyObjectMeta.Namespace,
	})
	return &gw
}

// setManagedPathsMode upgrades the controller with KGW_BOOTSTRAP_PATCH_MANAGED_PATHS set
// to mode, or unset when mode is empty, and waits for the new controller to run. A
// controller that starts reconciles every Gateway.
func (s *testingSuite) setManagedPathsMode(mode apisettings.BootstrapPatchManagedPathsMode) {
	// Upgrade with the values the suite was installed with, plus the mode.
	install := s.TestInstallation.Metadata
	extraArgs := append([]string{"--wait", "--timeout", "2m"}, install.ExtraHelmArgs...)
	if mode != "" {
		extraArgs = append(extraArgs, "--set", "controller.extraEnv."+managedPathsEnv+"="+string(mode))
	}
	s.TestInstallation.UpgradeKgatewayCore(s.Ctx, s.T(),
		[]string{install.ProfileValuesManifestFile, install.ValuesManifestFile}, extraArgs)

	// Wait for the controller pods.
	s.TestInstallation.AssertionsT(s.T()).EventuallyKgatewayInstallSucceeded(s.Ctx)
}

// eventuallyBootstrapPatchApplied waits for the Gateway's
// kgateway.dev/BootstrapPatchApplied condition to have status and reason.
func (s *testingSuite) eventuallyBootstrapPatchApplied(status metav1.ConditionStatus, reason string) {
	s.TestInstallation.AssertionsT(s.T()).EventuallyGatewayStatus(s.Ctx, proxyObjectMeta.Name, proxyObjectMeta.Namespace,
		gwv1.GatewayStatus{Conditions: []metav1.Condition{{
			Type:   conditions.KgatewayConditionBootstrapPatchApplied,
			Status: status,
			Reason: reason,
		}}},
		statusTimeout, pollInterval)
}

// eventuallyRolledOut waits until every proxy pod runs the Deployment's current pod
// template and is ready, and no old pod remains, so that a request to the proxy's admin
// API reaches a pod with the current bootstrap.
func (s *testingSuite) eventuallyRolledOut() {
	s.TestInstallation.AssertionsT(s.T()).Gomega.Eventually(func(g gomega.Gomega) {
		// Read the Deployment's current checksum and replica count.
		dep := s.getDeployment(g)
		want := dep.Spec.Template.Annotations[checksumAnnotation]
		replicas := int32(1)
		if dep.Spec.Replicas != nil {
			replicas = *dep.Spec.Replicas
		}

		// Check each pod: current checksum, not terminating, and ready.
		pods := &corev1.PodList{}
		err := s.TestInstallation.ClusterContext.Client.List(s.Ctx, pods,
			client.InNamespace(proxyObjectMeta.Namespace), client.MatchingLabels(dep.Spec.Selector.MatchLabels))
		g.Expect(err).NotTo(gomega.HaveOccurred())
		g.Expect(pods.Items).To(gomega.HaveLen(int(replicas)), "only the current pods should remain")
		for _, pod := range pods.Items {
			g.Expect(pod.DeletionTimestamp).To(gomega.BeNil(), "pod %s is terminating", pod.Name)
			g.Expect(pod.Annotations).To(gomega.HaveKeyWithValue(checksumAnnotation, want), "pod %s runs an old bootstrap", pod.Name)
			g.Expect(podReady(&pod)).To(gomega.BeTrue(), "pod %s is not ready", pod.Name)
		}
	}).WithTimeout(rolloutTimeout).WithPolling(pollInterval).Should(gomega.Succeed())
}

// eventuallyChecksumChangedFrom waits for the Deployment's bootstrap checksum to differ
// from old, which shows the controller deployed a new bootstrap.
func (s *testingSuite) eventuallyChecksumChangedFrom(old string) {
	s.TestInstallation.AssertionsT(s.T()).Gomega.Eventually(func(g gomega.Gomega) {
		g.Expect(s.getDeployment(g).Spec.Template.Annotations).To(
			gomega.HaveKeyWithValue(checksumAnnotation, gomega.Not(gomega.Equal(old))))
	}).WithTimeout(statusTimeout).WithPolling(pollInterval).Should(gomega.Succeed())
}

// eventuallyProxyBootstrap waits for check to pass on the bootstrap the proxy runs, read
// through its admin API.
func (s *testingSuite) eventuallyProxyBootstrap(check func(g gomega.Gomega, bs *envoybootstrapv3.Bootstrap)) {
	s.TestInstallation.AssertionsT(s.T()).AssertEnvoyAdminApi(s.Ctx, proxyObjectMeta,
		func(ctx context.Context, adminClient *admincli.Client) {
			s.TestInstallation.AssertionsT(s.T()).Gomega.Eventually(func(g gomega.Gomega) {
				bs, err := adminClient.GetBootstrap(ctx)
				g.Expect(err).NotTo(gomega.HaveOccurred(), "can get the bootstrap from the config dump")
				check(g, bs)
			}).WithContext(ctx).WithTimeout(30 * time.Second).WithPolling(time.Second).Should(gomega.Succeed())
		})
}

// podTemplateChecksum returns the Deployment's bootstrap checksum.
func (s *testingSuite) podTemplateChecksum() string {
	checksum := s.getDeployment(s.TestInstallation.AssertionsT(s.T()).Gomega).Spec.Template.Annotations[checksumAnnotation]
	s.Require().NotEmpty(checksum, "the proxy Deployment should have a bootstrap checksum")
	return checksum
}

func (s *testingSuite) getDeployment(g gomega.Gomega) *appsv1.Deployment {
	dep := &appsv1.Deployment{}
	err := s.TestInstallation.ClusterContext.Client.Get(s.Ctx, client.ObjectKey{
		Namespace: proxyObjectMeta.Namespace,
		Name:      proxyObjectMeta.Name,
	}, dep)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	return dep
}

// patchGatewayParameters changes the Gateway's GatewayParameters with patchFn.
func (s *testingSuite) patchGatewayParameters(patchFn func(*kgateway.GatewayParameters)) {
	// Read the GatewayParameters.
	gwp := &kgateway.GatewayParameters{}
	err := s.TestInstallation.ClusterContext.Client.Get(s.Ctx, client.ObjectKey{
		Namespace: proxyObjectMeta.Namespace,
		Name:      proxyObjectMeta.Name,
	}, gwp)
	s.Require().NoError(err, "can get the GatewayParameters")

	// Patch a copy.
	updated := gwp.DeepCopy()
	patchFn(updated)
	err = s.TestInstallation.ClusterContext.Client.Patch(s.Ctx, updated, client.MergeFrom(gwp))
	s.Require().NoError(err, "can patch the GatewayParameters")
}

// runtimeLayerNames returns the names of bs's runtime layers, in order.
func runtimeLayerNames(bs *envoybootstrapv3.Bootstrap) []string {
	var names []string
	for _, l := range bs.GetLayeredRuntime().GetLayers() {
		names = append(names, l.GetName())
	}
	return names
}

func podReady(pod *corev1.Pod) bool {
	for _, c := range pod.Status.Conditions {
		if c.Type == corev1.PodReady {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}

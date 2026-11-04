//go:build e2e

package upgrade

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kgateway "github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/cmdutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/envutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/common"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

// proxyNamespace and proxyLabelSelector identify the data-plane proxy that the controller
// provisions for the Gateway defined in testdata/setup.yaml.
const (
	proxyNamespace                  = "default"
	proxyLabelSelector              = "gateway.networking.k8s.io/gateway-name=gateway"
	initialTransformationValue      = "header-modified"
	skewedTransformationValue       = "header-modified-after-control-plane-upgrade"
	upgradeTransformationPolicyName = "upgrade-header-policy"
)

var (
	setupManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "setup.yaml")
	version       string
)

func init() {
	// Default to the version used in CI
	version = envutils.GetOrDefault("VERSION", "v1.0.0-ci1", false)
}

// testingSuite validates that kgateway can be upgraded from a released version to the
// locally-built chart. The suite currently has a single Test* method (TestUpgrade); SetupTest
// (rather than SetupSuite) installs the released version so that any future Test* method added
// here gets its own fresh install and teardown instead of inheriting whatever the previous
// method left behind.
type testingSuite struct {
	*base.BaseTestingSuite
	fromVersion string
}

func NewTestingSuite(fromVersion string) e2e.NewSuiteFunc {
	return func(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
		return &testingSuite{
			BaseTestingSuite: base.NewBaseTestingSuite(ctx, testInst, base.TestCase{}, nil),
			fromVersion:      fromVersion,
		}
	}
}

// SetupTest installs the released fromVersion fresh before every Test* method and registers
// its teardown. The teardown is registered here, before the test body applies its own
// manifests, so that testify.T's LIFO cleanup order runs manifest deletion first and only
// then uninstalls kgateway -- reversing that would try to delete kgateway-CRD-backed
// resources (e.g. TrafficPolicy) after the CRDs themselves are already gone.
func (s *testingSuite) SetupTest() {
	s.TestInstallation.InstallKgatewayFromRelease(s.Ctx, s.T(), s.fromVersion)
	testutils.Cleanup(s.T(), func() {
		s.TestInstallation.UninstallKgateway(s.Ctx, s.T())
	})
	s.TestInstallation.AssertionsT(s.T()).EventuallyGatewayInstallSucceeded(s.Ctx)
}

func (s *testingSuite) applyManifests() func() {
	s.ApplyManifests(&base.TestCase{
		Manifests: []string{setupManifest, defaults.HttpbinManifest},
	})

	return func() {
		s.DeleteManifests(&base.TestCase{
			Manifests: []string{setupManifest, defaults.HttpbinManifest},
		})
	}
}

// scaleHttpbin scales httpbin to the given replica count. Called just before the #14471 churn
// check (not from applyManifests) so it never adds latency to the earlier baseline
// connectivity check -- and scoped to this suite only, since defaults.HttpbinManifest is
// shared by many other suites that assume a single replica.
func (s *testingSuite) scaleHttpbin(replicas uint) {
	s.T().Helper()
	err := s.TestInstallation.Actions.Kubectl().Scale(
		s.Ctx, defaults.HttpbinDeployment.GetNamespace(), "deployment/"+defaults.HttpbinDeployment.GetName(), replicas)
	s.Require().NoError(err, "failed to scale httpbin to %d replicas", replicas)
}

// verifyRequestWithTransformation verifies that the TrafficPolicy in setup.yaml is being applied.
func (s *testingSuite) verifyRequestWithTransformation(expectedValue string) {
	s.T().Helper()
	common.BaseGateway.Send(
		s.T(),
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Headers:    map[string]any{"X-Upgrade-Test": expectedValue},
		},
		curl.WithPath("/headers"),
		curl.WithHostHeader("example.com"),
		curl.WithPort(8080),
	)
}

func (s *testingSuite) updateTransformationHeader(value string) {
	s.T().Helper()

	policy := &kgateway.TrafficPolicy{}
	err := s.TestInstallation.ClusterContext.Client.Get(s.Ctx, types.NamespacedName{
		Namespace: proxyNamespace,
		Name:      upgradeTransformationPolicyName,
	}, policy)
	s.Require().NoError(err, "failed to get upgrade TrafficPolicy")
	s.Require().NotNil(policy.Spec.Transformation, "upgrade TrafficPolicy transformation is nil")
	s.Require().NotNil(policy.Spec.Transformation.Response, "upgrade TrafficPolicy response transformation is nil")
	s.Require().Len(policy.Spec.Transformation.Response.Set, 1, "upgrade TrafficPolicy should set exactly one response header")

	original := policy.DeepCopy()
	policy.Spec.Transformation.Response.Set[0].Value = kgateway.InjaTemplate(value)
	err = s.TestInstallation.ClusterContext.Client.Patch(s.Ctx, policy, client.MergeFrom(original))
	s.Require().NoError(err, "failed to update upgrade TrafficPolicy")
}

// httpbinPodIPs returns the current httpbin pod IPs, so a caller can confirm a "churn" of the
// backend actually replaced its pods (and thus its endpoint IPs) rather than being a no-op.
func (s *testingSuite) httpbinPodIPs() sets.Set[string] {
	s.T().Helper()
	pods, err := s.TestInstallation.ClusterContext.Clientset.CoreV1().
		Pods(defaults.HttpbinDeployment.GetNamespace()).
		List(s.Ctx, metav1.ListOptions{LabelSelector: defaults.HttpbinLabelSelector})
	s.Require().NoError(err, "failed to list httpbin pods")
	ips := sets.New[string]()
	for _, pod := range pods.Items {
		s.Require().NotEmpty(pod.Status.PodIP, "httpbin pod %s has no IP yet", pod.Name)
		ips.Insert(pod.Status.PodIP)
	}
	return ips
}

func (s *testingSuite) localChartValuesFiles() []string {
	return []string{
		s.TestInstallation.Metadata.ProfileValuesManifestFile,
		s.TestInstallation.Metadata.ValuesManifestFile,
	}
}

func (s *testingSuite) upgradeControlPlaneWithReleasedDataPlane() {
	extraArgs := append([]string{}, s.TestInstallation.Metadata.ExtraHelmArgs...)
	// UpgradeKgatewayCore prepends the local image.tag. These later Helm values
	// deliberately override the default tag while keeping the controller local.
	extraArgs = append(extraArgs,
		"--set", "image.tag="+s.fromVersion,
		"--set", "controller.image.tag="+version,
	)
	s.TestInstallation.UpgradeKgatewayCore(s.Ctx, s.T(), s.localChartValuesFiles(), extraArgs)
}

func (s *testingSuite) upgradeDataPlane() {
	s.TestInstallation.UpgradeKgatewayCore(
		s.Ctx,
		s.T(),
		s.localChartValuesFiles(),
		s.TestInstallation.Metadata.ExtraHelmArgs,
	)
}

// TestUpgrade first upgrades the CRDs and control plane while keeping the released data plane,
// then upgrades the data plane and verifies the fully converged installation.
func (s *testingSuite) TestUpgrade() {
	// Create a gateway and ensure it works as expected
	cleanup := s.applyManifests()
	testutils.Cleanup(s.T(), cleanup)

	s.T().Logf("checking connectivity with the gateway...")
	common.SetupBaseGateway(s.Ctx, s.T(), s.TestInstallation, types.NamespacedName{
		Name:      "gateway",
		Namespace: "default",
	})
	s.verifyRequestWithTransformation(initialTransformationValue)
	s.T().Logf(" ok")

	// Pause the proxy Deployment's rollout before touching the control plane. Without this,
	// the deployer (once the new controller wins leader election, ~seconds later) reconciles
	// the Gateway and replaces this pod with a freshly-rendered one -- image.tag stays pinned
	// to s.fromVersion below, but the new pod's bootstrap ConfigMap is written by the NEW
	// controller code regardless of that pinned tag, so it isn't actually the same released
	// proxy anymore by the time later assertions run. Pausing keeps the originally-connected
	// proxy in place through the version-skew window that follows (see #14471).
	err := s.TestInstallation.Actions.Kubectl().RunCommand(s.Ctx, "-n", proxyNamespace, "rollout", "pause", "deployment/gateway")
	s.Require().NoError(err, "failed to pause proxy rollout")

	// First upgrade the CRDs and control plane while keeping the released data-plane image.
	// This exercises the supported version-skew window: the new control plane must continue
	// producing xDS that the released Envoy can accept.
	s.TestInstallation.InstallKgatewayCRDsFromLocalChart(s.Ctx, s.T())
	s.upgradeControlPlaneWithReleasedDataPlane()

	// Verify kgateway control plane upgraded successfully.
	s.T().Logf("checking the kgateway deployment && pod...")
	s.TestInstallation.AssertionsT(s.T()).EventuallyKgatewayUpgradeSucceeded(s.Ctx, version)
	s.T().Logf(" ok")

	// Prove the proxy is still on the released image before forcing a new translation.
	s.T().Logf("checking released data plane image %s...", s.fromVersion)
	s.TestInstallation.AssertionsT(s.T()).EventuallyDeploymentsRolledOut(s.Ctx, proxyNamespace, proxyLabelSelector)
	s.TestInstallation.AssertionsT(s.T()).EventuallyPodsHaveImageVersion(s.Ctx, proxyNamespace, proxyLabelSelector, s.fromVersion)
	s.T().Logf(" ok")

	// Change the policy after the control-plane rollout. Observing the new header proves that
	// the released proxy accepted a freshly translated snapshot from the new control plane.
	s.updateTransformationHeader(skewedTransformationValue)
	s.T().Logf("checking released data plane against the upgraded control plane...")
	s.verifyRequestWithTransformation(skewedTransformationValue)
	s.T().Logf(" ok")

	// Guard against https://github.com/kgateway-dev/kgateway/issues/14471: the control plane
	// must not withhold endpoint (EDS) updates from a proxy that hasn't upgraded yet. Churn the
	// backend's pods (forcing new EDS endpoints) while the released proxy is still connected,
	// then check Envoy's OWN live EDS-resolved state (not app-level traffic, not controller
	// logs) actually converged on the new endpoints and dropped the old ones. If the control
	// plane withheld the whole EDS response, this config dump would keep showing the
	// pre-churn IPs and never show the post-churn ones.
	s.scaleHttpbin(2)
	s.T().Logf("restarting httpbin backend...")
	beforeIPs := s.httpbinPodIPs()
	err = s.TestInstallation.Actions.Kubectl().RestartDeploymentAndWait(
		s.Ctx, defaults.HttpbinDeployment.GetName(), "-n", defaults.HttpbinDeployment.GetNamespace())
	s.Require().NoError(err)
	s.TestInstallation.AssertionsT(s.T()).EventuallyDeploymentsRolledOut(
		s.Ctx, defaults.HttpbinDeployment.GetNamespace(), defaults.HttpbinLabelSelector)
	afterIPs := s.httpbinPodIPs()
	s.Require().NotEmpty(afterIPs, "expected httpbin pods to be running after the restart")
	s.Require().Empty(beforeIPs.Intersection(afterIPs),
		"httpbin pods should have new IPs after the restart, not reuse the old ones (churn was a no-op): before=%v after=%v", beforeIPs, afterIPs)
	s.T().Logf(" ok")

	s.T().Logf("checking the still-released proxy reaches the churned backend...")
	s.verifyRequestWithTransformation(skewedTransformationValue)
	s.T().Logf(" ok")

	// Resume the paused rollout so the deployer can actually replace this pod with the
	// upgraded data-plane image below.
	err = s.TestInstallation.Actions.Kubectl().RunCommand(s.Ctx, "-n", proxyNamespace, "rollout", "resume", "deployment/gateway")
	s.Require().NoError(err, "failed to resume proxy rollout")

	// Remove the version skew by upgrading the default data-plane image to the local build.
	s.upgradeDataPlane()

	// Ensure the proxy data plane was upgraded too: the Deployment must finish rolling out
	// (old-revision proxy pods fully scaled down) and every proxy pod must run the new image
	s.T().Logf("checking the proxy deployment...")
	s.TestInstallation.AssertionsT(s.T()).EventuallyDeploymentsRolledOut(s.Ctx, proxyNamespace, proxyLabelSelector)
	s.T().Logf(" ok")
	s.T().Logf("checking the proxy image tag...")
	s.TestInstallation.AssertionsT(s.T()).EventuallyPodsHaveImageVersion(s.Ctx, proxyNamespace, proxyLabelSelector, version)
	s.T().Logf(" ok")

	// Ensure the same gateway works after the upgrade.
	s.T().Logf("checking connectivity with the gateway after the upgrade...")
	s.verifyRequestWithTransformation(skewedTransformationValue)
	s.T().Logf(" ok")

	// Recreate the same gateway and ensure it works after the upgrade
	cleanup()
	s.applyManifests()
	s.T().Logf("checking connectivity with the gateway after recreating it...")
	s.verifyRequestWithTransformation(initialTransformationValue)
	s.T().Logf(" ok")
}

// FetchLatestRelease returns the most recent release tag that is an ancestor of HEAD.
// This mirrors `git describe --tags --abbrev=0` but works in shallow checkouts where
// tags are not fetched, by resolving HEAD via git then checking ancestry via the GitHub API.
func FetchLatestRelease(ctx context.Context) (string, error) {
	script := filepath.Join(fsutils.GetModuleRoot(), "hack", "get-release.sh")
	var stdout bytes.Buffer
	cmd := cmdutils.Command(ctx, script, "--latest").
		WithStdout(&stdout).
		WithStderr(os.Stderr)
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return strings.TrimSpace(stdout.String()), nil
}

// FetchLatestRelease returns the most recent n-1 release tag that is an ancestor of HEAD.
// This mirrors `git describe --tags --abbrev=0` but works in shallow checkouts where
// tags are not fetched, by resolving HEAD via git then checking ancestry via the GitHub API.
func FetchPreviousMinorRelease(ctx context.Context) (string, error) {
	script := filepath.Join(fsutils.GetModuleRoot(), "hack", "get-release.sh")
	var stdout bytes.Buffer
	cmd := cmdutils.Command(ctx, script, "--previous").
		WithStdout(&stdout).
		WithStderr(os.Stderr)
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return strings.TrimSpace(stdout.String()), nil
}

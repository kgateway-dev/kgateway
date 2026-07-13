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
	"k8s.io/apimachinery/pkg/types"
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

// testingSuite validates that kgateway can be upgraded from a released version to the locally-built chart.
// The parent test function (TestUpgrade) is responsible for:
//   - Installing kgateway from the remote release before this suite runs.
//   - Uninstalling kgateway after this suite completes.
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

func (s *testingSuite) SetupSuite() {
	s.BaseTestingSuite.SetupSuite()
	// kgateway was installed from a released version by the parent test function.
	// Verify it is healthy before attempting the upgrade.
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

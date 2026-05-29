//go:build e2e

package e2e

import (
	"context"
	"os"
	"testing"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/helmutils"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/testutils/actions"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/testutils/assertions"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/testutils/cluster"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/testutils/helper"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/testutils/install"
)

// Installation is the surface a shared e2e test body uses to drive an install
// of kgateway.
//
// The chart name, chart path, release name, and license-injection are
// deliberately hidden behind UpgradeKgatewayCore so test bodies stay
// chart-agnostic.
type Installation interface {
	InstallKgatewayCRDsFromLocalChart(ctx context.Context, t *testing.T)
	InstallKgatewayCoreFromLocalChart(ctx context.Context, t *testing.T)
	InstallKgatewayFromLocalChart(ctx context.Context, t *testing.T)
	UninstallKgatewayCRDs(ctx context.Context, t *testing.T)
	UninstallKgatewayCore(ctx context.Context, t *testing.T)
	UninstallKgateway(ctx context.Context, t *testing.T)
	PreFailHandler(ctx context.Context, t *testing.T)

	// UpgradeKgatewayCore runs `helm upgrade` against the kgateway control plane
	// chart. The implementation is responsible for the chart path, release name,
	// and any installation-specific extra args (e.g. license injection).
	UpgradeKgatewayCore(ctx context.Context, t *testing.T, valuesFiles []string, extraArgs []string)

	// Cluster exposes the cluster context for tests that need direct API
	// server access.
	Cluster() *cluster.Context
	// ActionsProvider exposes the actions provider for kubectl/helm/curl.
	ActionsProvider() *actions.Provider
	// Assertions returns a per-test assertions provider.
	Assertions(t *testing.T) *assertions.Provider
	// InstallContext returns the install.Context the Installation was built
	// with. Test bodies read InstallNamespace and friends from it.
	InstallContext() *install.Context
	// Underlying returns the embedded *TestInstallation. Use when
	// passing into APIs that take *TestInstallation concretely (e.g.
	// SuiteRunner.Run).
	Underlying() *TestInstallation
}

// InstallationFactory constructs an Installation from a shared install.Context.
// Tests that want to be run against multiple installations call out to a
// package-level var of this type so callers can swap the constructor.
type InstallationFactory func(t *testing.T, installCtx *install.Context) Installation

// DefaultInstallationFactory is the InstallationFactory that produces a
// *TestInstallation. Shared test bodies use this by default.
func DefaultInstallationFactory(t *testing.T, installCtx *install.Context) Installation {
	return CreateTestInstallation(t, installCtx)
}

// Cluster implements Installation.
func (i *TestInstallation) Cluster() *cluster.Context { return i.ClusterContext }

// ActionsProvider implements Installation. The method is named to avoid
// colliding with the existing exported Actions field.
func (i *TestInstallation) ActionsProvider() *actions.Provider { return i.Actions }

// Assertions implements Installation. It delegates to the existing
// AssertionsT closure so per-test scoping behavior is preserved.
func (i *TestInstallation) Assertions(t *testing.T) *assertions.Provider { return i.AssertionsT(t) }

// InstallContext implements Installation.
func (i *TestInstallation) InstallContext() *install.Context { return i.Metadata }

// Underlying implements Installation. For *TestInstallation it returns the
// receiver.
func (i *TestInstallation) Underlying() *TestInstallation { return i }

// UpgradeKgatewayCore implements Installation by running `helm upgrade`
// against the kgateway chart at its local path.
func (i *TestInstallation) UpgradeKgatewayCore(
	ctx context.Context,
	t *testing.T,
	valuesFiles []string,
	extraArgs []string,
) {
	chartUri, err := helper.GetLocalChartPath(helmutils.ChartName, "")
	if err != nil {
		t.Fatalf("failed to get chart path: %v", err)
	}
	// Mirror InstallKgatewayCoreFromLocalChart: pin image.tag to the locally-built
	// chart so upgrade points at the same image that install did.
	mergedExtraArgs := append(helper.LocalChartImageTagArgs(), extraArgs...)
	err = i.Actions.Helm().WithReceiver(os.Stdout).Upgrade(
		ctx,
		helmutils.InstallOpts{
			Namespace:       i.Metadata.InstallNamespace,
			CreateNamespace: true,
			ValuesFiles:     valuesFiles,
			ReleaseName:     helmutils.ChartName,
			ChartUri:        chartUri,
			ExtraArgs:       mergedExtraArgs,
		})
	if err != nil {
		t.Fatalf("failed to upgrade Helm: %v", err)
	}
}

// Compile-time assertion that *TestInstallation satisfies Installation.
var _ Installation = (*TestInstallation)(nil)

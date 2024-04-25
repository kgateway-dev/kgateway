package actions

import (
	"context"
	"testing"
	"time"

	"github.com/solo-io/gloo/pkg/utils/helmutils"
	"github.com/solo-io/gloo/test/kube2e"
	"github.com/solo-io/gloo/test/kube2e/helper"
	"github.com/solo-io/gloo/test/kubernetes/testutils/cluster"
	"github.com/solo-io/gloo/test/kubernetes/testutils/gloogateway"
	"github.com/stretchr/testify/require"
)

// Glooctl defines the standard operations that can be executed via glooctl
// In a perfect world, all operations would be isolated to the OSS repository
// Since there are some custom Enterprise operations, we define this as an interface,
// so that Gloo Gateway Enterprise tests can rely on a custom implementation
type Glooctl interface {
	WithClusterContext(clusterContext *cluster.Context) Glooctl
	WithGlooGatewayContext(ggCtx *gloogateway.Context) Glooctl

	TestHelperInstall(ctx context.Context) error
	TestHelperUninstall(ctx context.Context) error
	ExportReport(ctx context.Context) error
}

// providerImpl is the implementation of the Provider for Gloo Gateway Open Source
type providerImpl struct {
	require *require.Assertions

	clusterContext     *cluster.Context
	glooGatewayContext *gloogateway.Context
}

func NewGlooctl(t *testing.T) Glooctl {
	return &providerImpl{
		require:            require.New(t),
		clusterContext:     nil,
		glooGatewayContext: nil,
	}
}

// WithClusterContext sets the Provider to point to the provided cluster
func (p *providerImpl) WithClusterContext(clusterContext *cluster.Context) Glooctl {
	p.clusterContext = clusterContext
	return p
}

// WithGlooGatewayContext sets the Provider to point to the provided installation of Gloo Gateway
func (p *providerImpl) WithGlooGatewayContext(ggCtx *gloogateway.Context) Glooctl {
	p.glooGatewayContext = ggCtx
	return p
}

// requiresGlooGatewayContext is invoked by methods on the Provider that can only be invoked
// if the provider has been configured to point to a Gloo Gateway installation
// There are certain actions that can be invoked that do not require that Gloo Gateway be installed for them to be invoked
func (p *providerImpl) requiresGlooGatewayContext() {
	p.require.NotNil(p.glooGatewayContext, "Provider attempted to create an action that requires a Gloo Gateway installation, but none was configured")
}

func (p *providerImpl) TestHelperInstall(ctx context.Context) error {
	var releasedVersion string
	if useVersion := kube2e.GetTestReleasedVersion(ctx, "gloo"); useVersion != "" {
		releasedVersion = useVersion
	}

	testHelper, err := helper.NewSoloTestHelper(func(defaults helper.TestConfig) helper.TestConfig {
		defaults.RootDir = "../../../.."
		defaults.HelmChartName = helmutils.ChartName
		defaults.InstallNamespace = p.glooGatewayContext.InstallNamespace
		defaults.Verbose = true
		defaults.ReleasedVersion = releasedVersion
		return defaults
	})
	if err != nil {
		return err
	}

	return testHelper.InstallGloo(ctx, helper.GATEWAY, 5*time.Minute, helper.ExtraArgs("--values", p.glooGatewayContext.ValuesManifestFile))
}

func (p *providerImpl) TestHelperUninstall(ctx context.Context) error {
	var err error
	var releasedVersion string
	if useVersion := kube2e.GetTestReleasedVersion(ctx, "gloo"); useVersion != "" {
		releasedVersion = useVersion
	}

	testHelper, err := helper.NewSoloTestHelper(func(defaults helper.TestConfig) helper.TestConfig {
		defaults.RootDir = "../../../.."
		defaults.HelmChartName = helmutils.ChartName
		defaults.InstallNamespace = p.glooGatewayContext.InstallNamespace
		defaults.Verbose = true
		defaults.ReleasedVersion = releasedVersion
		return defaults
	})
	if err != nil {
		return err
	}

	return testHelper.UninstallGlooAll()
}

func (p *providerImpl) ExportReport(ctx context.Context) error {
	p.requiresGlooGatewayContext()

	// TODO: implement `glooctl export report`
	// This would be useful for developers debugging tests and administrators inspecting running installations
	return nil
}

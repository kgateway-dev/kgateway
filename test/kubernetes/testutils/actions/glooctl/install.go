package glooctl

import (
	"context"
	"time"

	"github.com/solo-io/gloo/pkg/utils/helmutils"

	"github.com/solo-io/gloo/test/kubernetes/testutils/actions"

	"github.com/solo-io/gloo/test/kube2e/helper"
)

// NewTestHelperInstallAction returns an actions.ClusterAction that can install Gloo Gateway.
// NOTE: This relies on a helper tool, the SoloTestHelper.
//
//	In the future, it would be nice if we just exposed a way to run a glooctl install command directly.
//	Our goal of operations is to have them mirror as closely as possible, the operations that users take
func (p *providerImpl) NewTestHelperInstallAction() actions.ClusterAction {
	return func(ctx context.Context) error {
		testHelper, err := helper.NewSoloTestHelper(func(defaults helper.TestConfig) helper.TestConfig {
			defaults.RootDir = "../../../.."
			defaults.HelmChartName = helmutils.ChartName
			defaults.InstallNamespace = p.glooGatewayContext.InstallNamespace
			defaults.Verbose = true
			return defaults
		})
		if err != nil {
			return err
		}

		return testHelper.InstallGloo(ctx, helper.GATEWAY, 5*time.Minute, helper.ExtraArgs("--values", p.glooGatewayContext.ValuesManifestFile))
	}
}

// NewTestHelperUninstallAction returns an actions.ClusterAction that can uninstall Gloo Gateway.
// NOTE: This relies on a helper tool, the SoloTestHelper.
//
//	In the future, it would be nice if we just exposed a way to run a glooctl uninstall command directly.
//	Our goal of operations is to have them mirror as closely as possible, the operations that users take
func (p *providerImpl) NewTestHelperUninstallAction() actions.ClusterAction {
	p.requiresGlooGatewayContext()

	return func(ctx context.Context) error {
		var err error
		testHelper, err := helper.NewSoloTestHelper(func(defaults helper.TestConfig) helper.TestConfig {
			defaults.RootDir = "../../../.."
			defaults.HelmChartName = helmutils.ChartName
			defaults.InstallNamespace = p.glooGatewayContext.InstallNamespace
			defaults.Verbose = true
			return defaults
		})
		if err != nil {
			return err
		}

		return testHelper.UninstallGlooAll()
	}
}

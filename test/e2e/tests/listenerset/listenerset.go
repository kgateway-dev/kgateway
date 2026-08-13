//go:build e2e

// Package listenerset holds the body of the TestListenerSet e2e test.
package listenerset

import (
	"context"
	"os"
	"testing"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/envutils"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/common"
	listenersetfeature "github.com/kgateway-dev/kgateway/v2/test/e2e/features/listenerset"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/testutils/install"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

// Run executes the TestListenerSet scenario against the Installation produced
// by the supplied factory.
func Run(t *testing.T, factory e2e.InstallationFactory) {
	ctx := t.Context()
	installNs, nsEnvPredefined := envutils.LookupOrDefault(testutils.InstallNamespace, "ls-test")
	testInstallation := factory(
		t,
		&install.Context{
			InstallNamespace:          installNs,
			ProfileValuesManifestFile: e2e.CommonRecommendationManifest,
			ValuesManifestFile:        e2e.EmptyValuesManifestPath,
		},
	)

	// Set the env to the install namespace if it is not already set
	if !nsEnvPredefined {
		os.Setenv(testutils.InstallNamespace, installNs)
	}

	// We register the cleanup function _before_ we actually perform the installation.
	// This allows us to uninstall kgateway, in case the original installation only completed partially
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

	// Install kgateway
	testInstallation.InstallKgatewayFromLocalChart(ctx, t)

	// The suite references the shared nginx backend cross-namespace (with its own
	// ReferenceGrant) instead of deploying its own.
	common.SetupSharedNginxBackend(ctx, t, testInstallation.Underlying())

	SuiteRunner().Run(ctx, t, testInstallation.Underlying())
}

// SuiteRunner returns the suite runner for the TestListenerSet scenario.
func SuiteRunner() e2e.SuiteRunner {
	suiteRunner := e2e.NewSuiteRunner(false)
	suiteRunner.Register("ListenerSet", listenersetfeature.NewTestingSuite)
	return suiteRunner
}

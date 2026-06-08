//go:build e2e

package tests_test

import (
	"context"
	"os"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/envutils"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/common"
	testdefaults "github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	. "github.com/kgateway-dev/kgateway/v2/test/e2e/tests"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/testutils/install"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

func TestListenerSet(t *testing.T) {
	ctx := context.Background()
	installNs, nsEnvPredefined := envutils.LookupOrDefault(testutils.InstallNamespace, "ls-test")
	testInstallation := e2e.CreateTestInstallation(
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

	// Apply the shared nginx pod (ns nginx-shared) that the suite references as a backend
	// instead of deploying its own. The suite applies its own ReferenceGrant for the
	// cross-namespace route -> Service reference.
	common.SetupBaseConfig(ctx, t, testInstallation, testdefaults.NginxPodManifest)
	// Wait once for the shared nginx pod so the suite doesn't race a cold image pull.
	testInstallation.AssertionsT(t).EventuallyPodsRunning(ctx, "nginx-shared", metav1.ListOptions{
		LabelSelector: testdefaults.WellKnownAppLabel + "=nginx",
	}, 2*time.Minute)

	ListenerSetSuiteRunner().Run(ctx, t, testInstallation)
}

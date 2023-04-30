package upgrade_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/test/kube2e"
	"github.com/solo-io/gloo/test/kube2e/upgrade"
	"github.com/solo-io/go-utils/versionutils"
	"github.com/solo-io/skv2/codegen/util"

	. "github.com/onsi/ginkgo/v2"
	"github.com/solo-io/gloo/test/helpers"
	skhelpers "github.com/solo-io/solo-kit/test/helpers"
)

func TestUpgrade(t *testing.T) {
	helpers.RegisterGlooDebugLogPrintHandlerAndClearLogs()
	skhelpers.RegisterCommonFailHandlers()
	skhelpers.SetupLog()
	RunSpecs(t, "Upgrade Suite")
}

var (
	suiteCtx    context.Context
	suiteCancel context.CancelFunc

	crdDir                string
	chartUri              string
	targetReleasedVersion string

	// Versions to upgrade from
	// ex: current branch is 1.13.10 - this would be the latest patch release of 1.12
	LastPatchPreviousMinorVersion *versionutils.Version

	// ex:current branch is 1.13.10 - this would be 1.13.9
	CurrentPatchMostRecentMinorVersion *versionutils.Version
	firstReleaseOfMinor                bool

	skipIfFirstMinorFunc func()
)

var _ = BeforeSuite(func() {
	suiteCtx, suiteCancel = context.WithCancel(context.Background())

	testHelper, err := kube2e.GetTestHelper(suiteCtx, namespace)
	Expect(err).NotTo(HaveOccurred())

	skhelpers.RegisterPreFailHandler(helpers.KubeDumpOnFail(GinkgoWriter, "upgrade", testHelper.InstallNamespace, "other-ns"))

	crdDir = filepath.Join(util.GetModuleRoot(), "install", "helm", "gloo", "crds")
	targetReleasedVersion = kube2e.GetTestReleasedVersion(suiteCtx, "gloo")

	chartUri = "gloo/gloo"
	if targetReleasedVersion == "" {
		chartUri = filepath.Join(testHelper.RootDir, testHelper.TestAssetDir, testHelper.HelmChartName+"-"+testHelper.ChartVersion()+".tgz")
	}

	LastPatchPreviousMinorVersion, CurrentPatchMostRecentMinorVersion, err = upgrade.GetUpgradeVersions(suiteCtx, "gloo")
	Expect(err).NotTo(HaveOccurred())

	skipIfFirstMinorFunc = func() {}
	if CurrentPatchMostRecentMinorVersion == nil {
		fmt.Println("First release of minor, skipping some upgrade tests")
		CurrentPatchMostRecentMinorVersion = versionutils.NewVersion(0, 0, 0, "", 0)
		skipIfFirstMinorFunc = func() {
			Skip("First release of minor, skipping some upgrade tests")
		}
	}
})

var _ = AfterSuite(func() {
	suiteCancel()
})

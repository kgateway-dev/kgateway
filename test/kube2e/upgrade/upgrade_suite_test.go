package upgrade_test

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/test/kube2e"
	"github.com/solo-io/gloo/test/kube2e/upgrade"
	"github.com/solo-io/gloo/test/testutils/version"
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
	if targetReleasedVersion != "" {
		chartUri = "gloo/gloo"
	} else {
		chartUri = filepath.Join(testHelper.RootDir, testHelper.TestAssetDir, testHelper.HelmChartName+"-"+testHelper.ChartVersion()+".tgz")
	}
	skipIfFirstMinorFunc = func() {}
	LastPatchPreviousMinorVersion, CurrentPatchMostRecentMinorVersion, err = upgrade.GetUpgradeVersions(suiteCtx, "gloo")
	if err != nil && errors.Is(err, version.FirstReleaseError) {
		firstReleaseOfMinor = true
		fmt.Println("First release of minor, skipping some upgrade tests")
		CurrentPatchMostRecentMinorVersion = versionutils.NewVersion(0, 0, 0, "", 0)
		skipIfFirstMinorFunc = func() {
			Skip("First release of minor, skipping some upgrade tests")
		}
	} else if err != nil {
		Expect(err).NotTo(HaveOccurred())
	}
})

var _ = AfterSuite(func() {
	suiteCancel()
})

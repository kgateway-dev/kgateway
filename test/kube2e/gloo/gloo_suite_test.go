package gloo_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap/zapcore"

	"github.com/solo-io/gloo/test/services"

	"github.com/avast/retry-go"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/kube2e"
	glootestutils "github.com/solo-io/gloo/test/testutils"
	"github.com/solo-io/go-utils/testutils"
	"github.com/solo-io/k8s-utils/kubeutils"
	"github.com/solo-io/k8s-utils/testutils/helper"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	skhelpers "github.com/solo-io/solo-kit/test/helpers"
)

func TestGloo(t *testing.T) {
	helpers.RegisterGlooDebugLogPrintHandlerAndClearLogs()
	skhelpers.RegisterCommonFailHandlers()
	skhelpers.SetupLog()
	RunSpecs(t, "Gloo Suite")
}

const (
	namespace = defaults.GlooSystem
)

var (
	testHelper        *helper.SoloTestHelper
	resourceClientset *kube2e.KubeResourceClientSet
	snapshotWriter    helpers.SnapshotWriter

	ctx    context.Context
	cancel context.CancelFunc

	envoyFactory *services.EnvoyFactory
)

var _ = BeforeSuite(func() {
	var err error

	ctx, cancel = context.WithCancel(context.Background())
	testHelper, err = kube2e.GetTestHelper(ctx, namespace)
	Expect(err).NotTo(HaveOccurred())
	skhelpers.RegisterPreFailHandler(helpers.KubeDumpOnFail(GinkgoWriter, testHelper.InstallNamespace))

	// Allow skipping of install step for running multiple times
	if !glootestutils.ShouldSkipInstall() {
		installGloo()
	}

	cfg, err := kubeutils.GetConfig("", "")
	Expect(err).NotTo(HaveOccurred())

	resourceClientset, err = kube2e.NewKubeResourceClientSet(ctx, cfg)
	Expect(err).NotTo(HaveOccurred())

	snapshotWriter = helpers.NewSnapshotWriter(resourceClientset, []retry.Option{})

	envoyFactory, err = services.NewEnvoyFactory()
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	defer cancel()

	if glootestutils.ShouldTearDown() {
		uninstallGloo()
	}
})

func installGloo() {
	cwd, err := os.Getwd()
	Expect(err).NotTo(HaveOccurred(), "working dir could not be retrieved while installing gloo")
	helmValuesFile := filepath.Join(cwd, "artifacts", "helm.yaml")

	err = testHelper.InstallGloo(ctx, helper.GATEWAY, 5*time.Minute, helper.ExtraArgs("--values", helmValuesFile))
	Expect(err).NotTo(HaveOccurred())

	kube2e.GlooctlCheckEventuallyHealthy(1, testHelper, "90s")
	kube2e.EventuallyReachesConsistentState(testHelper.InstallNamespace, zapcore.InfoLevel)
}

func uninstallGloo() {
	Expect(testHelper).ToNot(BeNil())
	err := testHelper.UninstallGlooAll()
	Expect(err).NotTo(HaveOccurred())

	err = testutils.Kubectl("delete", "namespace", testHelper.InstallNamespace)
	Expect(err).NotTo(HaveOccurred())

	Eventually(func() error {
		return testutils.Kubectl("get", "namespace", testHelper.InstallNamespace)
	}, "60s", "1s").Should(HaveOccurred())
}

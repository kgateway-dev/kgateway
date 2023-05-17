package gloo_test

import (
	"context"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/k8s-utils/testutils/clusterlock"
	apiext "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"os"
	"path/filepath"
	"testing"
	"time"

	glootestutils "github.com/solo-io/gloo/test/testutils"

	"github.com/avast/retry-go"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/k8s-utils/kubeutils"

	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/kube2e"
	"github.com/solo-io/go-utils/testutils"
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

	apiExts apiext.Interface
	locker  *clusterlock.TestClusterLocker
)

var _ = SynchronizedBeforeSuite(beforeSuiteOne, beforeSuiteAll)
var _ = SynchronizedAfterSuite(afterSuiteOne, afterSuiteAll)

func beforeSuiteOne() []byte {
	// Register the CRDs once at the beginning of the suite
	ctx, cancel = context.WithCancel(context.Background())
	cfg, err := kubeutils.GetConfig("", "")
	Expect(err).NotTo(HaveOccurred())

	apiExts, err = apiext.NewForConfig(cfg)
	Expect(err).NotTo(HaveOccurred())

	err = skhelpers.AddAndRegisterCrd(ctx, gloov1.UpstreamCrd, apiExts)
	Expect(err).NotTo(HaveOccurred())

	err = skhelpers.AddAndRegisterCrd(ctx, gatewayv1.VirtualServiceCrd, apiExts)
	Expect(err).NotTo(HaveOccurred())
	return nil
}

func beforeSuiteAll(_ []byte) {
	var err error
	locker, err = clusterlock.NewTestClusterLocker(kube2e.MustKubeClient(), clusterlock.Options{})
	Expect(err).NotTo(HaveOccurred())
	Expect(locker.AcquireLock(retry.Attempts(40))).NotTo(HaveOccurred())

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
}

func afterSuiteOne(ctx context.Context) {
	// Delete those CRDs once at the end of the suite
	_ = apiExts.ApiextensionsV1().CustomResourceDefinitions().Delete(ctx, gloov1.UpstreamCrd.FullName(), v1.DeleteOptions{})
	_ = apiExts.ApiextensionsV1().CustomResourceDefinitions().Delete(ctx, gatewayv1.VirtualServiceCrd.FullName(), v1.DeleteOptions{})

	cancel()
}

func afterSuiteAll(_ context.Context) {
	err := locker.ReleaseLock()
	Expect(err).NotTo(HaveOccurred())

	if glootestutils.ShouldTearDown() {
		uninstallGloo()
	}
}

func installGloo() {
	cwd, err := os.Getwd()
	Expect(err).NotTo(HaveOccurred(), "working dir could not be retrieved while installing gloo")
	helmValuesFile := filepath.Join(cwd, "artifacts", "helm.yaml")

	err = testHelper.InstallGloo(ctx, helper.GATEWAY, 5*time.Minute, helper.ExtraArgs("--values", helmValuesFile))
	Expect(err).NotTo(HaveOccurred())

	kube2e.GlooctlCheckEventuallyHealthy(1, testHelper, "90s")
	kube2e.EventuallyReachesConsistentState(testHelper.InstallNamespace)
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

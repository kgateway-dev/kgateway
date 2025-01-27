package gloo_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/solo-io/gloo/test/kubernetes/testutils/cluster"
	"github.com/solo-io/skv2/codegen/util"

	"github.com/solo-io/gloo/pkg/utils/kubeutils/kubectl"

	"github.com/avast/retry-go"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/solo-io/gloo/test/services/envoy"

	"github.com/solo-io/gloo/test/services"

	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/kube2e"
	"github.com/solo-io/gloo/test/kube2e/helper"
	testruntime "github.com/solo-io/gloo/test/kubernetes/testutils/runtime"
	glootestutils "github.com/solo-io/gloo/test/testutils"
	"github.com/solo-io/go-utils/testutils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	skhelpers "github.com/solo-io/solo-kit/test/helpers"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

func TestGloo(t *testing.T) {
	helpers.RegisterGlooDebugLogPrintHandlerAndClearLogs()
	skhelpers.RegisterCommonFailHandlers()
	skhelpers.SetupLog()
	RunSpecs(t, "Gloo Suite")
}

const (
	namespace   = defaults.GlooSystem
	gatewayPort = int(80)
)

var (
	testHelper        *helper.SoloTestHelper
	resourceClientset *kube2e.KubeResourceClientSet
	snapshotWriter    helpers.SnapshotWriter

	ctx    context.Context
	cancel context.CancelFunc

	envoyFactory envoy.Factory
	vaultFactory *services.VaultFactory
)

var _ = BeforeSuite(func() {
	var err error

	// This line prevents controller-runtime from complaining about log.SetLogger never being called
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))
	ctx, cancel = context.WithCancel(context.Background())
	testHelper, err = kube2e.GetTestHelper(ctx, namespace)
	Expect(err).NotTo(HaveOccurred())
	testHelper.SetKubeCli(kubectl.NewCli().WithReceiver(GinkgoWriter))

	outDir := filepath.Join(util.GetModuleRoot(), "_output", "kube2e-artifacts")
	namespaces := []string{testHelper.InstallNamespace}
	skhelpers.RegisterPreFailHandler(helpers.StandardGlooDumpOnFail(GinkgoWriter, outDir, namespaces))

	// Allow skipping of install step for running multiple times
	if !glootestutils.ShouldSkipInstall() {
		installGloo()
	}

	// We rely on the "new" kubernetes/e2e setup code, since it incorporates controller-runtime logging setup
	runtimeContext := testruntime.NewContext()
	clusterContext := cluster.MustKindContext(runtimeContext.ClusterName)

	resourceClientset, err = kube2e.NewKubeResourceClientSet(ctx, clusterContext.RestConfig)
	Expect(err).NotTo(HaveOccurred(), "can create kube resource client set")

	snapshotWriter = helpers.NewSnapshotWriter(resourceClientset).
		WithWriteNamespace(testHelper.InstallNamespace).
		// This isn't strictly necessary, but we use to ensure that WithRetryOptions behaves correctly
		WithRetryOptions([]retry.Option{retry.Attempts(3)})

	envoyFactory = envoy.NewFactory()

	vaultFactory, err = services.NewVaultFactory()
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

	kube2e.GlooctlCheckEventuallyHealthy(1, testHelper.InstallNamespace, "90s")
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

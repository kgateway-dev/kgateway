package gateway_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap/zapcore"

	kubeutils2 "github.com/solo-io/gloo/test/testutils"

	"github.com/avast/retry-go"

	gatewaydefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"

	gloodefaults "github.com/solo-io/gloo/projects/gloo/pkg/defaults"

	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/kube2e"
	"github.com/solo-io/k8s-utils/testutils/helper"
	skhelpers "github.com/solo-io/solo-kit/test/helpers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGateway(t *testing.T) {
	helpers.RegisterGlooDebugLogPrintHandlerAndClearLogs()
	skhelpers.RegisterCommonFailHandlers()
	skhelpers.SetupLog()
	RunSpecs(t, "Gateway Suite")
}

const (
	gatewayProxy = gatewaydefaults.GatewayProxyName
	gatewayPort  = int(80)
	namespace    = gloodefaults.GlooSystem
)

var (
	ctx    context.Context
	cancel context.CancelFunc

	testHelper        *helper.SoloTestHelper
	resourceClientset *kube2e.KubeResourceClientSet
	snapshotWriter    helpers.SnapshotWriter
)

var _ = BeforeSuite(StartTestHelper)
var _ = AfterSuite(TearDownTestHelper)

func StartTestHelper() {
	var err error
	ctx, cancel = context.WithCancel(context.Background())

	testHelper, err = kube2e.GetTestHelper(ctx, namespace)
	Expect(err).NotTo(HaveOccurred())
	skhelpers.RegisterPreFailHandler(helpers.KubeDumpOnFail(GinkgoWriter, testHelper.InstallNamespace))

	// Allow skipping of install step for running multiple times
	if !kubeutils2.ShouldSkipInstall() {
		installGloo()
	}

	resourceClientset, err = kube2e.NewDefaultKubeResourceClientSet(ctx)
	Expect(err).NotTo(HaveOccurred(), "can create kube resource client set")

	snapshotWriter = helpers.NewSnapshotWriter(resourceClientset, []retry.Option{})
}

func TearDownTestHelper() {
	if kubeutils2.ShouldTearDown() {
		uninstallGloo()
	}
	cancel()
}

func installGloo() {
	cwd, err := os.Getwd()
	Expect(err).NotTo(HaveOccurred(), "working dir could not be retrieved while installing gloo")
	helmValuesFile := filepath.Join(cwd, "artifacts", "helm.yaml")

	err = testHelper.InstallGloo(ctx, helper.GATEWAY, 5*time.Minute, helper.ExtraArgs("--values", helmValuesFile))
	Expect(err).NotTo(HaveOccurred())

	// Check that everything is OK
	kube2e.GlooctlCheckEventuallyHealthy(1, testHelper, "90s")

	// Ensure gloo reaches valid state and doesn't continually resync
	// we can consider doing the same for leaking go-routines after resyncs
	kube2e.EventuallyReachesConsistentState(testHelper.InstallNamespace, zapcore.DebugLevel)
}

func uninstallGloo() {
	Expect(testHelper).ToNot(BeNil())
	err := testHelper.UninstallGloo()
	Expect(err).NotTo(HaveOccurred())
	_, err = kube2e.MustKubeClient().CoreV1().Namespaces().Get(ctx, testHelper.InstallNamespace, metav1.GetOptions{})
	Expect(apierrors.IsNotFound(err)).To(BeTrue())
}

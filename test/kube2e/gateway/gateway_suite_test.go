package gateway_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	kubetestclients "github.com/solo-io/gloo/test/kubernetes/testutils/clients"

	"github.com/solo-io/gloo/pkg/utils/kubeutils/kubectl"

	kubeutils2 "github.com/solo-io/gloo/test/testutils"

	gatewaydefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"

	gloodefaults "github.com/solo-io/gloo/projects/gloo/pkg/defaults"

	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/kube2e"
	"github.com/solo-io/gloo/test/kube2e/helper"
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

	kubeCli *kubectl.Cli
)

var _ = BeforeSuite(StartTestHelper)
var _ = AfterSuite(TearDownTestHelper)

func StartTestHelper() {
	var err error
	ctx, cancel = context.WithCancel(context.Background())

	testHelper, err = kube2e.GetTestHelper(ctx, namespace)
	Expect(err).NotTo(HaveOccurred())
	skhelpers.RegisterPreFailHandler(helpers.StandardGlooDumpOnFail(GinkgoWriter, metav1.ObjectMeta{Namespace: testHelper.InstallNamespace}))

	kubeCli = kubectl.NewCli().WithReceiver(GinkgoWriter)

	// Allow skipping of install step for running multiple times
	if !kubeutils2.ShouldSkipInstall() {
		installGloo()
	}

	resourceClientset, err = kube2e.NewDefaultKubeResourceClientSet(ctx)
	Expect(err).NotTo(HaveOccurred(), "can create kube resource client set")

	snapshotWriter = helpers.NewSnapshotWriter(resourceClientset).WithWriteNamespace(testHelper.InstallNamespace)
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
	kube2e.GlooctlCheckEventuallyHealthy(1, testHelper.InstallNamespace, "90s")

	// Ensure gloo reaches valid state and doesn't continually resync
	// we can consider doing the same for leaking go-routines after resyncs
	kube2e.EventuallyReachesConsistentState(testHelper.InstallNamespace)
}

func uninstallGloo() {
	Expect(testHelper).ToNot(BeNil())
	err := testHelper.UninstallGloo()
	Expect(err).NotTo(HaveOccurred())
	_, err = kubetestclients.MustClientset().CoreV1().Namespaces().Get(ctx, testHelper.InstallNamespace, metav1.GetOptions{})
	Expect(apierrors.IsNotFound(err)).To(BeTrue())
}

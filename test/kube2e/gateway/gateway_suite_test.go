//go:build ignore

package gateway_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/solo-io/skv2/codegen/util"

	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/testutils/cluster"

	kubetestclients "github.com/kgateway-dev/kgateway/v2/test/kubernetes/testutils/clients"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils/kubectl"

	kubeutils2 "github.com/kgateway-dev/kgateway/v2/test/testutils"

	gatewaydefaults "github.com/kgateway-dev/kgateway/v2/internal/gateway/pkg/defaults"

	gloodefaults "github.com/kgateway-dev/kgateway/v2/internal/gloo/pkg/defaults"

	skhelpers "github.com/solo-io/solo-kit/test/helpers"

	"github.com/kgateway-dev/kgateway/v2/test/helpers"
	"github.com/kgateway-dev/kgateway/v2/test/kube2e"
	"github.com/kgateway-dev/kgateway/v2/test/kube2e/helper"
	testruntime "github.com/kgateway-dev/kgateway/v2/test/kubernetes/testutils/runtime"

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

	outDir := filepath.Join(util.GetModuleRoot(), "_output", "kube2e-artifacts")
	namespaces := []string{testHelper.InstallNamespace}
	skhelpers.RegisterPreFailHandler(helpers.StandardKgatewayDumpOnFail(GinkgoWriter, outDir, namespaces))

	kubeCli = kubectl.NewCli().WithReceiver(GinkgoWriter)

	// Allow skipping of install step for running multiple times
	if !kubeutils2.ShouldSkipInstall() {
		installGloo()
	}

	// We rely on the "new" kubernetes/e2e setup code, since it incorporates controller-runtime logging setup
	runtimeContext := testruntime.NewContext()
	clusterContext := cluster.MustKindContext(runtimeContext.ClusterName)

	resourceClientset, err = kube2e.NewKubeResourceClientSet(ctx, clusterContext.RestConfig)
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

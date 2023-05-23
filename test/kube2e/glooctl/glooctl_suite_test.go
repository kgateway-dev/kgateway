package glooctl_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/solo-io/go-utils/testutils/exec"

	"github.com/solo-io/gloo/test/testutils"

	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"

	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/kube2e"
	"github.com/solo-io/k8s-utils/kubeutils"
	"github.com/solo-io/k8s-utils/testutils/helper"
	skhelpers "github.com/solo-io/solo-kit/test/helpers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGlooctl(t *testing.T) {
	helpers.RegisterGlooDebugLogPrintHandlerAndClearLogs()
	skhelpers.RegisterCommonFailHandlers()
	skhelpers.SetupLog()
	RunSpecs(t, "Glooctl Suite")
}

var (
	namespace = defaults.GlooSystem

	testHelper        *helper.SoloTestHelper
	resourceClientset *kube2e.KubeResourceClientSet

	ctx    context.Context
	cancel context.CancelFunc
)

var _ = BeforeSuite(StartTestHelper)
var _ = AfterSuite(TearDownTestHelper)

func StartTestHelper() {
	ctx, cancel = context.WithCancel(context.Background())

	var err error
	testHelper, err = kube2e.GetTestHelper(ctx, namespace)
	Expect(err).NotTo(HaveOccurred())
	// Register additional fail handlers
	skhelpers.RegisterPreFailHandler(helpers.KubeDumpOnFail(GinkgoWriter, "istio-system", testHelper.InstallNamespace))

	if !testutils.ShouldSkipInstall() {
		installGloo()
	}

	// Create KubeResourceClientSet
	cfg, err := kubeutils.GetConfig("", "")
	Expect(err).NotTo(HaveOccurred())

	resourceClientset, err = kube2e.NewKubeResourceClientSet(ctx, cfg)
	Expect(err).NotTo(HaveOccurred())
}

func TearDownTestHelper() {
	if testutils.ShouldTearDown() {
		uninstallGloo()
	}
	cancel()
}

func installGloo() {
	cwd, err := os.Getwd()
	Expect(err).NotTo(HaveOccurred(), "working dir could not be retrieved while installing gloo")
	helmValuesFile := filepath.Join(cwd, "artifacts", "helm.yaml")

	// Install Gloo
	err = testHelper.InstallGloo(ctx, helper.GATEWAY, 5*time.Minute, helper.ExtraArgs("--values", helmValuesFile))
	Expect(err).NotTo(HaveOccurred())

	kube2e.GlooctlCheckEventuallyHealthy(1, testHelper, "90s")
	kube2e.EventuallyReachesConsistentState(testHelper.InstallNamespace)
}

func uninstallGloo() {
	Expect(testHelper).ToNot(BeNil())
	err := testHelper.UninstallGloo()
	Expect(err).NotTo(HaveOccurred())
	_, err = kube2e.MustKubeClient().CoreV1().Namespaces().Get(ctx, testHelper.InstallNamespace, metav1.GetOptions{})
	Expect(apierrors.IsNotFound(err)).To(BeTrue())
}

// GlooctlOut take a set of arguments for glooctl and then executes local glooctl with these arguments
func GlooctlOut(args ...string) (string, error) {
	glooctlCommand := []string{filepath.Join(testHelper.BuildAssetDir, testHelper.GlooctlExecName)}
	glooctlCommand = append(glooctlCommand, args...)
	// execute the command with verbose output
	return exec.RunCommandOutput(testHelper.RootDir, true, glooctlCommand...)
}

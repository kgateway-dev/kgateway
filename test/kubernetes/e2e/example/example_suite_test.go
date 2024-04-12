package example_test

import (
	"context"

	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/kube2e"
	"github.com/solo-io/gloo/test/kube2e/helper"
	"github.com/solo-io/gloo/test/kubernetes/testutils/assertions"
	"github.com/solo-io/gloo/test/kubernetes/testutils/operations"
	"github.com/solo-io/gloo/test/testutils"
	"github.com/solo-io/gloo/test/testutils/kubeutils"
	skhelpers "github.com/solo-io/solo-kit/test/helpers"

	"os"
	"path/filepath"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/ginkgo/v2"
)

func TestExampleSuite(t *testing.T) {
	helpers.RegisterGlooDebugLogPrintHandlerAndClearLogs()
	skhelpers.RegisterCommonFailHandlers()

	RunSpecs(t, "Example Suite")
}

var (
	clusterContext *kubeutils.ClusterContext

	operationsProvider *operations.Provider

	operator *operations.Operator

	assertionProvider *assertions.Provider

	// TEMPORARY VALUES
	// These are just to prove out the mechanism for building tests, using the old format of installing gloo
	suiteCtx    context.Context
	suiteCancel context.CancelFunc
	testHelper  *helper.SoloTestHelper

	cwd string
)

var _ = BeforeSuite(func() {
	clusterContext = kubeutils.MustKindClusterContext(os.Getenv(testutils.ClusterName))

	// Create an operator which is responsible for execution Operation agains the cluster
	operator = operations.NewGinkgoOperator()

	// Set the operations provider to point to the running cluster
	operationsProvider = operations.NewProvider().WithClusterContext(clusterContext)

	// Set the assertion provider to point to the running cluster
	assertionProvider = assertions.NewProvider().WithClusterContext(clusterContext)

	// TEMPORARY CODE
	var err error
	suiteCtx, suiteCancel = context.WithCancel(context.Background())

	testHelper, err = kube2e.GetTestHelper(suiteCtx, "example-test-ns")
	Expect(err).NotTo(HaveOccurred())

	installGlooGateway()
})

var _ = AfterSuite(func() {
	uninstallGlooGateway()

	suiteCancel()
})

// TEMPORARY
// This code is isolated, as the way that we install Gloo still needs to be implemented

func installGlooGateway() {
	var err error
	cwd, err = os.Getwd()
	Expect(err).NotTo(HaveOccurred(), "working dir could not be retrieved while installing gloo")
	helmValuesFile := filepath.Join(cwd, "manifests", "helm.yaml")

	err = testHelper.InstallGloo(suiteCtx, helper.GATEWAY, 5*time.Minute, helper.ExtraArgs("--values", helmValuesFile))
	Expect(err).NotTo(HaveOccurred())

	// Check that everything is OK
	kube2e.GlooctlCheckEventuallyHealthy(1, testHelper, "90s")

	// Ensure gloo reaches valid state and doesn't continually resync
	// we can consider doing the same for leaking go-routines after resyncs
	kube2e.EventuallyReachesConsistentState(testHelper.InstallNamespace)
}

func uninstallGlooGateway() {
	Expect(testHelper).ToNot(BeNil())
	err := testHelper.UninstallGloo()
	Expect(err).NotTo(HaveOccurred())

	_, err = clusterContext.Clientset.CoreV1().Namespaces().Get(suiteCtx, testHelper.InstallNamespace, metav1.GetOptions{})
	Expect(apierrors.IsNotFound(err)).To(BeTrue())
}

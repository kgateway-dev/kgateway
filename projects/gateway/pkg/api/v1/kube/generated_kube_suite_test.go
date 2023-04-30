package kube_test

import (
	"context"
	"os"
	"testing"

	"github.com/avast/retry-go"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/test/kube2e"
	"github.com/solo-io/k8s-utils/kubeutils"
	"github.com/solo-io/solo-kit/test/helpers"
	apiext "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/k8s-utils/testutils/clusterlock"
	"github.com/solo-io/solo-kit/pkg/utils/statusutils"
)

func TestKube(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Generated Kube Types Suite")
}

var (
	suiteCtx    context.Context
	suiteCancel context.CancelFunc
	apiExts     apiext.Interface

	locker    *clusterlock.TestClusterLocker
	namespace = "kube-test-ns"
)

var _ = SynchronizedBeforeSuite(beforeSuiteOne, beforeSuiteAll)
var _ = SynchronizedAfterSuite(afterSuiteOne, afterSuiteAll)

func beforeSuiteOne() []byte {
	// Register the CRDs once at the beginning of the suite
	suiteCtx, suiteCancel = context.WithCancel(context.Background())
	cfg, err := kubeutils.GetConfig("", "")
	Expect(err).NotTo(HaveOccurred())

	apiExts, err = apiext.NewForConfig(cfg)
	Expect(err).NotTo(HaveOccurred())

	err = helpers.AddAndRegisterCrd(suiteCtx, gloov1.UpstreamCrd, apiExts)
	Expect(err).NotTo(HaveOccurred())

	err = helpers.AddAndRegisterCrd(suiteCtx, gatewayv1.VirtualServiceCrd, apiExts)
	Expect(err).NotTo(HaveOccurred())
	return nil
}

func beforeSuiteAll(_ []byte) {
	var err error
	locker, err = clusterlock.NewTestClusterLocker(kube2e.MustKubeClient(), clusterlock.Options{})
	Expect(err).NotTo(HaveOccurred())
	Expect(locker.AcquireLock(retry.Attempts(40))).NotTo(HaveOccurred())

	// necessary for non-default namespace
	err = os.Setenv(statusutils.PodNamespaceEnvName, namespace)
	Expect(err).NotTo(HaveOccurred())
}

func afterSuiteOne(ctx context.Context) {
	// Delete those CRDs once at the end of the suite
	_ = apiExts.ApiextensionsV1().CustomResourceDefinitions().Delete(ctx, gloov1.UpstreamCrd.FullName(), v1.DeleteOptions{})
	_ = apiExts.ApiextensionsV1().CustomResourceDefinitions().Delete(ctx, gatewayv1.VirtualServiceCrd.FullName(), v1.DeleteOptions{})

	suiteCancel()
}

func afterSuiteAll(_ context.Context) {
	err := locker.ReleaseLock()
	Expect(err).NotTo(HaveOccurred())

	err = os.Unsetenv(statusutils.PodNamespaceEnvName)
	Expect(err).NotTo(HaveOccurred())
}

package kube_test

import (
	"testing"

	"github.com/avast/retry-go"
	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/reporters"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/test/kube2e"
	"github.com/solo-io/k8s-utils/testutils/clusterlock"
)

func TestKube(t *testing.T) {
	RegisterFailHandler(Fail)
	junitReporter := reporters.NewJUnitReporter("junit.xml")
	RunSpecsWithDefaultAndCustomReporters(t, "Generated Kube Types Suite", []Reporter{junitReporter})
}

var locker *clusterlock.TestClusterLocker

var _ = BeforeSuite(func() {
	var err error
	locker, err = clusterlock.NewTestClusterLocker(kube2e.MustKubeClient(), clusterlock.Options{})
	Expect(err).NotTo(HaveOccurred())
	Expect(locker.AcquireLock(retry.Attempts(40))).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	locker.ReleaseLock()
})

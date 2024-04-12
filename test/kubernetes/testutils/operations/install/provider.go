package install

import (
	"context"
	"time"

	"github.com/onsi/gomega"
	"github.com/solo-io/gloo/test/kube2e"
	"github.com/solo-io/gloo/test/kube2e/helper"
	"github.com/solo-io/gloo/test/kubernetes/testutils/assertions"
	"github.com/solo-io/gloo/test/kubernetes/testutils/operations"
	"github.com/solo-io/gloo/test/testutils/kubeutils"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// The install OperationProvider is a WORK IN PROGRESS
// It currently relies on the old paradigm for installing Gloo

type OperationProvider struct {
	clusterContext *kubeutils.ClusterContext
}

func NewProvider() *OperationProvider {
	return &OperationProvider{
		clusterContext: nil,
	}
}

// WithClusterContext sets the ScenarioProvider to point to the provided cluster
func (p *OperationProvider) WithClusterContext(clusterContext *kubeutils.ClusterContext) *OperationProvider {
	p.clusterContext = clusterContext
	return p
}

func (p *OperationProvider) NewInstallOperation(valuesManifest string) (operations.Operation, error) {
	var testHelper *helper.SoloTestHelper

	return &operation{
		name: "install-gloo-gateway",
		op: func(ctx context.Context) error {
			var err error
			testHelper, err = helper.NewSoloTestHelper(func(defaults helper.TestConfig) helper.TestConfig {
				defaults.RootDir = "../../../.."
				defaults.HelmChartName = "gloo"
				defaults.InstallNamespace = "example-test-ns"
				defaults.Verbose = true
				return defaults
			})
			if err != nil {
				return err
			}

			return testHelper.InstallGloo(ctx, helper.GATEWAY, 5*time.Minute, helper.ExtraArgs("--values", valuesManifest))
		},
		assertion: func(ctx context.Context) {
			// Check that everything is OK
			kube2e.GlooctlCheckEventuallyHealthy(1, testHelper, "90s")

			// Ensure gloo reaches valid state and doesn't continually resync
			// we can consider doing the same for leaking go-routines after resyncs
			kube2e.EventuallyReachesConsistentState(testHelper.InstallNamespace)
		},
	}, nil
}

func (p *OperationProvider) NewUninstallOperation() (operations.Operation, error) {
	var testHelper *helper.SoloTestHelper

	return &operation{
		name: "uninstall-gloo-gateway",
		op: func(ctx context.Context) error {
			var err error
			testHelper, err = helper.NewSoloTestHelper(func(defaults helper.TestConfig) helper.TestConfig {
				defaults.RootDir = "../../../.."
				defaults.HelmChartName = "gloo"
				defaults.InstallNamespace = "example-test-ns"
				defaults.Verbose = true
				return defaults
			})
			if err != nil {
				return err
			}

			return testHelper.UninstallGloo()
		},
		assertion: func(ctx context.Context) {
			_, err := p.clusterContext.Clientset.CoreV1().Namespaces().Get(ctx, testHelper.InstallNamespace, metav1.GetOptions{})
			gomega.Expect(apierrors.IsNotFound(err)).To(gomega.BeTrue())
		},
	}, nil
}

var _ operations.Operation = new(operation)

type operation struct {
	name      string
	op        func(ctx context.Context) error
	assertion func(ctx context.Context)
}

func (o operation) Name() string {
	return o.name
}

func (o operation) Execute() func(ctx context.Context) error {
	return o.op
}

func (o operation) ExecutionAssertion() assertions.DiscreteAssertion {
	return o.assertion
}

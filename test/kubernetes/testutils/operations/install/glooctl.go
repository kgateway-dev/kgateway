package install

import (
	"context"
	"time"

	"github.com/solo-io/gloo/test/kubernetes/testutils/assertions"

	"github.com/onsi/gomega"
	"github.com/solo-io/gloo/test/kube2e"
	"github.com/solo-io/gloo/test/kube2e/helper"
	"github.com/solo-io/gloo/test/kubernetes/testutils/operations"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (p *OperationProvider) NewGlooctlInstallOperation(valuesManifest string) (operations.Operation, error) {
	var testHelper *helper.SoloTestHelper

	return &operations.BasicOperation{
		OpName: "glooctl-install-gloo-gateway",
		OpExecute: func(ctx context.Context) error {
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
		OpAssertions: []assertions.DiscreteAssertion{
			func(ctx context.Context) {
				// Check that everything is OK
				kube2e.GlooctlCheckEventuallyHealthy(1, testHelper, "90s")

				// Ensure gloo reaches valid state and doesn't continually resync
				// we can consider doing the same for leaking go-routines after resyncs
				kube2e.EventuallyReachesConsistentState(testHelper.InstallNamespace)
			},
		},
	}, nil
}

func (p *OperationProvider) NewGlooctlUninstallOperation() (operations.Operation, error) {
	var testHelper *helper.SoloTestHelper

	return &operations.BasicOperation{
		OpName: "glooctl-uninstall-gloo-gateway",
		OpExecute: func(ctx context.Context) error {
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
		OpAssertions: []assertions.DiscreteAssertion{
			func(ctx context.Context) {
				_, err := p.clusterContext.Clientset.CoreV1().Namespaces().Get(ctx, testHelper.InstallNamespace, metav1.GetOptions{})
				gomega.Expect(apierrors.IsNotFound(err)).To(gomega.BeTrue())
			},
		},
	}, nil
}

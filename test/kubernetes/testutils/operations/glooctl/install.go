package glooctl

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

// NewTestHelperInstallOperation returns an Operation that can install Gloo Gateway, and assert that it succcssfully completed
// NOTE: This relies on a helper tool, the SoloTestHelper.
//
//	In the future, it would be nice if we just exposed a way to run a glooctl install command directly.
//	Our goal of operations is to have them mirror as closely as possible, the operations that users take
func (p *operationProviderImpl) NewTestHelperInstallOperation(provider *assertions.Provider) operations.Operation {
	return &operations.BasicOperation{
		OpName: "glooctl-install-gloo-gateway",
		OpExecute: func(ctx context.Context) error {

			testHelper, err := helper.NewSoloTestHelper(func(defaults helper.TestConfig) helper.TestConfig {
				defaults.RootDir = "../../../.."
				defaults.HelmChartName = "gloo"
				defaults.InstallNamespace = p.glooGatewayContext.InstallNamespace
				defaults.Verbose = true
				return defaults
			})
			if err != nil {
				return err
			}

			return testHelper.InstallGloo(ctx, helper.GATEWAY, 5*time.Minute, helper.ExtraArgs("--values", p.glooGatewayContext.ValuesManifestFile))
		},
		OpAssertions: []assertions.DiscreteAssertion{
			func(ctx context.Context) {
				// Check that everything is OK
				provider.CheckResources()(ctx)

				// Ensure gloo reaches valid state and doesn't continually resync
				// we can consider doing the same for leaking go-routines after resyncs
				kube2e.EventuallyReachesConsistentState(p.glooGatewayContext.InstallNamespace)
			},
		},
	}
}

// NewTestHelperUninstallOperation returns an Operation that can uninstall Gloo Gateway, and assert that it successfully completed
// NOTE: This relies on a helper tool, the SoloTestHelper.
//
//	In the future, it would be nice if we just exposed a way to run a glooctl install command directly.
//	Our goal of operations is to have them mirror as closely as possible, the operations that users take
func (p *operationProviderImpl) NewTestHelperUninstallOperation() operations.Operation {
	return &operations.BasicOperation{
		OpName: "glooctl-uninstall-gloo-gateway",
		OpExecute: func(ctx context.Context) error {
			var err error
			testHelper, err := helper.NewSoloTestHelper(func(defaults helper.TestConfig) helper.TestConfig {
				defaults.RootDir = "../../../.."
				defaults.HelmChartName = "gloo"
				defaults.InstallNamespace = p.glooGatewayContext.InstallNamespace
				defaults.Verbose = true
				return defaults
			})
			if err != nil {
				return err
			}

			return testHelper.UninstallGlooAll()
		},
		OpAssertions: []assertions.DiscreteAssertion{
			func(ctx context.Context) {
				_, err := p.clusterContext.Clientset.CoreV1().Namespaces().Get(ctx, p.glooGatewayContext.InstallNamespace, metav1.GetOptions{})
				gomega.Expect(apierrors.IsNotFound(err)).To(gomega.BeTrue())
			},
		},
	}
}

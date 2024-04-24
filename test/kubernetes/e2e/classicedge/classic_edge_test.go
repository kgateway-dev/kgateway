package classicedge_test

import (
	"context"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/headless_svc"
	"github.com/solo-io/gloo/test/kubernetes/testutils/gloogateway"
	"github.com/solo-io/skv2/codegen/util"
)

var _ = Describe("Classic Edge Test", Ordered, func() {

	var (
		ctx context.Context

		// testInstallation contains all the metadata/utilities necessary to execute a series of tests
		// against an installation of Gloo Edge
		testInstallation *e2e.TestInstallation
	)

	BeforeAll(func() {
		var err error
		ctx = context.Background()

		testInstallation = testCluster.RegisterTestInstallation(
			&gloogateway.Context{
				InstallNamespace:   "classic-edge-test",
				ValuesManifestFile: filepath.Join(util.MustGetThisDir(), "manifests", "classic-gateway-test-helm.yaml"),
			},
		)

		err = testInstallation.InstallGlooGateway(ctx, testInstallation.Actions.Glooctl().NewTestHelperInstallAction())
		Expect(err).NotTo(HaveOccurred())
	})

	AfterAll(func() {
		err := testInstallation.UninstallGlooGateway(ctx, testInstallation.Actions.Glooctl().NewTestHelperUninstallAction())
		Expect(err).NotTo(HaveOccurred())

		testCluster.UnregisterTestInstallation(testInstallation)
	})

	Context("Headless Service Routing", func() {

		It("routes to headless services", func() {
			testInstallation.RunTest(ctx, headless_svc.ConfigureRoutingHeadlessSvc(false))
		})
	})

})

package k8sgateway_test

import (
	"context"
	"path/filepath"

	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/deployer"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/route_options"
	"github.com/solo-io/skv2/codegen/util"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	"k8s.io/client-go/rest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/testutils/gloogateway"
)

var _ = Describe("Deployer Test", Ordered, func() {

	// An entire file is meant to capture the behaviors that we want to test for a given installation of Gloo Gateway

	var (
		ctx context.Context

		// testInstallation contains all the metadata/utilities necessary to execute a series of tests
		// against an installation of Gloo Gateway
		testInstallation *e2e.TestInstallation
	)

	BeforeAll(func() {
		var err error
		ctx = context.Background()

		testInstallation = testCluster.RegisterTestInstallation(
			"k8s-gw-deployer-test",
			&gloogateway.Context{
				InstallNamespace:   "k8s-gw-deployer-test",
				ValuesManifestFile: filepath.Join(util.MustGetThisDir(), "manifests", "k8s-gateway-test-helm.yaml"),
			},
		)

		err = testInstallation.InstallGlooGateway(ctx, testInstallation.Actions.Glooctl().NewTestHelperInstallAction())
		Expect(err).NotTo(HaveOccurred())

		// Initialize required resource clients for test after CRDs are installed
		testInstallation.RouteOptionClient, err = AddRouteOptionClient(ctx, testCluster.ClusterContext.RestConfig)
		Expect(err).NotTo(HaveOccurred(), "failed to initialize RouteOptionClient")

	})

	AfterAll(func() {
		err := testInstallation.UninstallGlooGateway(ctx, testInstallation.Actions.Glooctl().NewTestHelperUninstallAction())
		Expect(err).NotTo(HaveOccurred())

		testCluster.UnregisterTestInstallation(testInstallation)
	})

	Context("Deployer", func() {

		It("provisions resources appropriately", func() {
			testInstallation.RunTest(ctx, deployer.ProvisionDeploymentAndService)
		})

		It("configures proxies from the GatewayParameters CR", func() {
			testInstallation.RunTest(ctx, deployer.ConfigureProxiesFromGatewayParameters)
		})

	})

	Context("RouteOptions", func() {

		It("Apply fault injection using targetRef RouteOption", func() {
			testInstallation.RunTest(ctx, route_options.ConfigureRouteOptionsWithTargetRef)
		})

		It("Apply fault injection using filter extension RouteOption", func() {
			testInstallation.RunTest(ctx, route_options.ConfigureRouteOptionsWithFilterExtenstion)
		})

	})

})

func AddRouteOptionClient(ctx context.Context, restConfig *rest.Config) (gatewayv1.RouteOptionClient, error) {
	cache := kube.NewKubeCache(ctx)
	routeOptionClientFactory := &factory.KubeResourceClientFactory{
		Crd:         gatewayv1.RouteOptionCrd,
		Cfg:         restConfig,
		SharedCache: cache,
	}
	return gatewayv1.NewRouteOptionClient(ctx, routeOptionClientFactory)
}

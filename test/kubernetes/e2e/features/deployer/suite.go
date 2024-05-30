package deployer

import (
	"context"
	"time"

	"github.com/onsi/gomega"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/solo-io/gloo/pkg/utils/envoyutils/admincli"
	"github.com/solo-io/gloo/pkg/utils/kubeutils"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer/setup"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/testutils/runtime"
)

// testingSuite is the entire Suite of tests for the "deployer" feature
// The "deployer" code can be found here: /projects/gateway2/deployer
type testingSuite struct {
	suite.Suite

	ctx context.Context

	// testInstallation contains all the metadata/utilities necessary to execute a series of tests
	// against an installation of Gloo Gateway
	testInstallation *e2e.TestInstallation
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		ctx:              ctx,
		testInstallation: testInst,
	}
}

func (s *testingSuite) TestProvisionDeploymentAndService() {
	s.T().Cleanup(func() {
		err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, deployerProvisionManifestFile)
		s.NoError(err, "can delete manifest")
		s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, proxyService, proxyDeployment)
	})

	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, deployerProvisionManifestFile)
	s.Require().NoError(err, "can apply manifest")
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, proxyService, proxyDeployment)
}

func (s *testingSuite) TestConfigureProxiesFromGatewayParameters() {
	s.T().Cleanup(func() {
		err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, gwParametersManifestFile)
		s.NoError(err, "can delete manifest")
		s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, gwParams)

		err = s.testInstallation.Actions.Kubectl().DeleteFileSafe(s.ctx, deployerProvisionManifestFile)
		s.NoError(err, "can delete manifest")
		s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, proxyService, proxyDeployment)
	})

	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, deployerProvisionManifestFile)
	s.Require().NoError(err, "can apply manifest")
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, proxyService, proxyDeployment)

	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, gwParametersManifestFile)
	s.Require().NoError(err, "can apply manifest")
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, gwParams)
	s.testInstallation.Assertions.EventuallyRunningReplicas(s.ctx, proxyDeployment.ObjectMeta, gomega.Equal(1))

	// We assert that we can port-forward requests to the proxy deployment, and then execute requests against the server
	if s.testInstallation.RuntimeContext.RunSource == runtime.LocalDevelopment {
		// There are failures when opening port-forwards to the Envoy Admin API in CI
		// Those are currently being investigated
		s.testInstallation.Assertions.AssertEnvoyAdminApi(
			s.ctx,
			proxyDeployment.ObjectMeta,
			serverInfoLogLevelAssertion(s.testInstallation, "debug", "connection:trace,upstream:debug"),
			xdsClusterAssertion(s.testInstallation),
		)
	}
}

func (s *testingSuite) TestSelfManagedGateway() {
	s.T().Cleanup(func() {
		err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, selfManagedGatewayManifestFile)
		s.NoError(err, "can delete manifest")
	})

	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, selfManagedGatewayManifestFile)
	s.Require().NoError(err, "can apply manifest")

	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		gw := &gwv1.Gateway{}
		err := s.testInstallation.ClusterContext.Client.Get(s.ctx,
			types.NamespacedName{Name: glooProxyObjectMeta.Name, Namespace: glooProxyObjectMeta.Namespace},
			gw)
		assert.NoError(c, err, "gateway not found")

		accepted := false
		for _, conditions := range gw.Status.Conditions {
			if conditions.Type == string(gwv1.GatewayConditionAccepted) && conditions.Status == metav1.ConditionTrue {
				accepted = true
				break
			}
		}
		assert.True(c, accepted, "gateway status not accepted")
	}, 10*time.Second, 1*time.Second)

	s.testInstallation.Assertions.ConsistentlyObjectsNotExist(s.ctx, proxyService, proxyDeployment)
}

func (s *testingSuite) getterForMeta(meta *metav1.ObjectMeta) helpers.InputResourceGetter {
	return func() (resources.InputResource, error) {
		return s.testInstallation.ResourceClients.RouteOptionClient().Read(meta.GetNamespace(), meta.GetName(), clients.ReadOpts{})
	}
}

func serverInfoLogLevelAssertion(testInstallation *e2e.TestInstallation, expectedLogLevel, expectedComponentLogLevel string) func(ctx context.Context, adminClient *admincli.Client) {
	return func(ctx context.Context, adminClient *admincli.Client) {
		testInstallation.Assertions.Gomega.Eventually(func(g gomega.Gomega) {
			serverInfo, err := adminClient.GetServerInfo(ctx)
			g.Expect(err).NotTo(gomega.HaveOccurred(), "can get server info")
			g.Expect(serverInfo.GetCommandLineOptions().GetLogLevel()).To(
				gomega.Equal(expectedLogLevel), "defined on the GatewayParameters CR")
			g.Expect(serverInfo.GetCommandLineOptions().GetComponentLogLevel()).To(
				gomega.Equal(expectedComponentLogLevel), "defined on the GatewayParameters CR")
		}).
			WithContext(ctx).
			WithTimeout(time.Second * 10).
			WithPolling(time.Millisecond * 200).
			Should(gomega.Succeed())
	}
}

func xdsClusterAssertion(testInstallation *e2e.TestInstallation) func(ctx context.Context, adminClient *admincli.Client) {
	return func(ctx context.Context, adminClient *admincli.Client) {
		testInstallation.Assertions.Gomega.Eventually(func(g gomega.Gomega) {
			clusters, err := adminClient.GetStaticClusters(ctx)
			g.Expect(err).NotTo(gomega.HaveOccurred(), "can get static clusters from config dump")

			xdsCluster, ok := clusters["xds_cluster"]
			g.Expect(ok).To(gomega.BeTrue(), "xds_cluster in list")

			g.Expect(xdsCluster.GetLoadAssignment().GetEndpoints()).To(gomega.HaveLen(1))
			g.Expect(xdsCluster.GetLoadAssignment().GetEndpoints()[0].GetLbEndpoints()).To(gomega.HaveLen(1))
			xdsSocketAddress := xdsCluster.GetLoadAssignment().GetEndpoints()[0].GetLbEndpoints()[0].GetEndpoint().GetAddress().GetSocketAddress()
			g.Expect(xdsSocketAddress).NotTo(gomega.BeNil())

			g.Expect(xdsSocketAddress.GetAddress()).To(gomega.Equal(kubeutils.ServiceFQDN(metav1.ObjectMeta{
				Name:      kubeutils.GlooServiceName,
				Namespace: testInstallation.Metadata.InstallNamespace,
			})), "xds socket address points to gloo service, in installation namespace")

			xdsPort, err := setup.GetNamespacedControlPlaneXdsPort(ctx, testInstallation.Metadata.InstallNamespace, testInstallation.ResourceClients.ServiceClient())
			g.Expect(err).NotTo(gomega.HaveOccurred())
			g.Expect(xdsSocketAddress.GetPortValue()).To(gomega.Equal(uint32(xdsPort)), "xds socket port points to gloo service, in installation namespace")
		}).
			WithContext(ctx).
			WithTimeout(time.Second * 10).
			WithPolling(time.Millisecond * 200).
			Should(gomega.Succeed())
	}
}

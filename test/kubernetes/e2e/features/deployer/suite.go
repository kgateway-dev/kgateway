package deployer

import (
	"context"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/utils/envoyutils/admincli"
	"github.com/solo-io/gloo/test/kubernetes/testutils/runtime"
	"time"

	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/stretchr/testify/suite"
)

// FeatureSuite is the entire Suite of tests for the "deployer" feature
type FeatureSuite struct {
	suite.Suite

	ctx context.Context

	// testInstallation contains all the metadata/utilities necessary to execute a series of tests
	// against an installation of Gloo Gateway
	testInstallation *e2e.TestInstallation
}

func NewFeatureSuite(ctx context.Context, testInst *e2e.TestInstallation) *FeatureSuite {
	return &FeatureSuite{
		ctx:              ctx,
		testInstallation: testInst,
	}
}

func (s *FeatureSuite) SetupSuite() {
}

func (s *FeatureSuite) TearDownSuite() {
}

func (s *FeatureSuite) BeforeTest(suiteName, testName string) {
}

func (s *FeatureSuite) AfterTest(suiteName, testName string) {
}

func (s *FeatureSuite) TestProvisionDeploymentAndService() {
	s.T().Cleanup(func() {
		err := s.testInstallation.Actions.Kubectl().Client().DeleteFile(s.ctx, deployerProvisionManifestFile)
		s.NoError(err, "can delete manifest")
		s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, proxyService, proxyDeployment)
	})

	err := s.testInstallation.Actions.Kubectl().Client().ApplyFile(s.ctx, deployerProvisionManifestFile)
	s.NoError(err, "can apply manifest")
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, proxyService, proxyDeployment)
}

func (s *FeatureSuite) TestConfigureProxiesFromGatewayParameters() {
	s.T().Cleanup(func() {
		err := s.testInstallation.Actions.Kubectl().Client().DeleteFile(s.ctx, gwParametersManifestFile)
		s.NoError(err, "can delete manifest")
		s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, gwParams)

		err = s.testInstallation.Actions.Kubectl().Client().DeleteFile(s.ctx, deployerProvisionManifestFile)
		s.NoError(err, "can delete manifest")
		s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, proxyService, proxyDeployment)
	})

	err := s.testInstallation.Actions.Kubectl().Client().ApplyFile(s.ctx, deployerProvisionManifestFile)
	s.NoError(err, "can apply manifest")
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, proxyService, proxyDeployment)

	err = s.testInstallation.Actions.Kubectl().Client().ApplyFile(s.ctx, gwParametersManifestFile)
	s.NoError(err, "can apply manifest")
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, gwParams)
	s.testInstallation.Assertions.EventuallyRunningReplicas(s.ctx, proxyDeployment.ObjectMeta, Equal(1))
	// We assert that we can port-forward requests to the proxy deployment, and then execute requests against the server
	s.testInstallation.Assertions.AssertEnvoyAdminApi(
		s.ctx,
		proxyDeployment.ObjectMeta,
		serverInfoLogLevelAssertion(s.testInstallation),
	)
}

func serverInfoLogLevelAssertion(testInstallation *e2e.TestInstallation) func(ctx context.Context, adminClient *admincli.Client) {
	return func(ctx context.Context, adminClient *admincli.Client) {
		if testInstallation.TestCluster.RuntimeContext.RunSource != runtime.LocalDevelopment {
			// There are failures when running this command in CI
			// Those are currently being investigated
			return
		}
		testInstallation.Assertions.Gomega.Eventually(func(g Gomega) {
			serverInfo, err := adminClient.GetServerInfo(ctx)
			g.Expect(err).NotTo(HaveOccurred(), "can get server info")
			g.Expect(serverInfo.GetCommandLineOptions().GetLogLevel()).To(
				Equal("debug"), "defined on the GatewayParameters CR")
			g.Expect(serverInfo.GetCommandLineOptions().GetComponentLogLevel()).To(
				Equal("connection:trace,upstream:debug"), "defined on the GatewayParameters CR")
		}).
			WithContext(ctx).
			WithTimeout(time.Second * 10).
			WithPolling(time.Millisecond * 200).
			Should(Succeed())
	}
}

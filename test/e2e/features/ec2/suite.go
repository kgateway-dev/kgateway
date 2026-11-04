//go:build e2e

package ec2

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"
	"google.golang.org/protobuf/encoding/protojson"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/testutils/localstack"
	"github.com/kgateway-dev/kgateway/v2/test/envoyutils/admincli"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

type testingSuite struct {
	suite.Suite
	ctx               context.Context
	testInstallation  *e2e.TestInstallation
	localstackURL     string
	expectedPrivateIP string
}

var _ e2e.NewSuiteFunc = NewTestingSuite

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		ctx:              ctx,
		testInstallation: testInst,
	}
}

func (s *testingSuite) SetupSuite() {
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, setupManifest)
	s.Require().NoError(err, "can apply "+setupManifest)

	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, awsCliPodManifest)
	s.Require().NoError(err, "can apply "+awsCliPodManifest)

	s.testInstallation.AssertionsT(s.T()).EventuallyPodReady(s.ctx, testNamespace, awsCliPodName, 30*time.Second)

	s.extractLocalstackEndpoint()
	s.cleanupEc2Instances()
	s.expectedPrivateIP = s.createEc2Instance()
}

func (s *testingSuite) TearDownSuite() {
	if !testutils.ShouldSkipCleanup(s.T()) {
		s.cleanupEc2Instances()
		err := s.testInstallation.Actions.Kubectl().DeleteFileSafe(s.ctx, awsCliPodManifest)
		s.NoError(err, "can delete aws cli pod manifest")
		err = s.testInstallation.Actions.Kubectl().DeleteFileSafe(s.ctx, setupManifest)
		s.NoError(err, "can delete setup manifest")
	}
}

func (s *testingSuite) BeforeTest(_, _ string) {
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, ec2BackendManifest)
	s.Require().NoError(err, "can apply "+ec2BackendManifest)
}

func (s *testingSuite) AfterTest(_, _ string) {
	if testutils.ShouldSkipCleanup(s.T()) {
		return
	}
	err := s.testInstallation.Actions.Kubectl().DeleteFileSafe(s.ctx, ec2BackendManifest)
	s.NoError(err, "can delete "+ec2BackendManifest)
}

func (s *testingSuite) TestEc2BackendDiscovery() {
	s.testInstallation.AssertionsT(s.T()).EventuallyHTTPRouteCondition(
		s.ctx,
		routeName,
		testNamespace,
		gwv1.RouteConditionAccepted,
		metav1.ConditionTrue,
	)
	s.testInstallation.AssertionsT(s.T()).EventuallyBackendCondition(
		s.ctx,
		backendName,
		testNamespace,
		"Accepted",
		metav1.ConditionTrue,
	)

	s.testInstallation.AssertionsT(s.T()).AssertEnvoyAdminApi(s.ctx, proxyObjectMeta, func(ctx context.Context, adminClient *admincli.Client) {
		s.testInstallation.AssertionsT(s.T()).Gomega.Eventually(func(g gomega.Gomega) {
			clusters, err := adminClient.GetDynamicClusters(ctx)
			g.Expect(err).NotTo(gomega.HaveOccurred(), "can get dynamic clusters")

			cluster, ok := clusters[ec2ClusterName]
			g.Expect(ok).To(gomega.BeTrue(), "ec2 backend cluster should be present in Envoy xDS")
			g.Expect(cluster).NotTo(gomega.BeNil())
			g.Expect(cluster.GetType()).To(gomega.Equal(envoyclusterv3.Cluster_EDS))
			g.Expect(cluster.GetEdsClusterConfig()).NotTo(gomega.BeNil(), "ec2 backend should use EDS")
			g.Expect(cluster.GetIgnoreHealthOnHostRemoval()).To(gomega.BeTrue(), "ec2 backend should ignore health on host removal")

			cfgDump, err := adminClient.GetConfigDump(ctx, map[string]string{
				"include_eds": "on",
			})
			g.Expect(err).NotTo(gomega.HaveOccurred(), "can get config dump including EDS resources")

			cfgJSON, err := protojson.Marshal(cfgDump)
			g.Expect(err).NotTo(gomega.HaveOccurred(), "can marshal config dump")

			g.Expect(string(cfgJSON)).To(gomega.ContainSubstring(ec2ClusterName), "config dump should include ec2 cluster name")
			g.Expect(string(cfgJSON)).To(gomega.ContainSubstring(s.expectedPrivateIP), "config dump should include discovered private IP")
			g.Expect(string(cfgJSON)).To(gomega.ContainSubstring(fmt.Sprintf("\"portValue\":%d", ec2Port)), "config dump should include discovered endpoint port")
		}).
			WithContext(ctx).
			WithTimeout(120 * time.Second).
			WithPolling(2 * time.Second).
			Should(gomega.Succeed())
	})
}

func (s *testingSuite) extractLocalstackEndpoint() {
	endpoint, found, err := localstack.EndpointURL(s.ctx, s.testInstallation.ClusterContext.Client)
	s.Require().NoError(err, "can resolve localstack endpoint")
	s.Require().True(found, "localstack service must exist")

	s.localstackURL = endpoint
}

func (s *testingSuite) cleanupEc2Instances() {
	s.runAwsShell(
		fmt.Sprintf(
			`ids="$(aws ec2 describe-instances --region %s --endpoint-url %s --filters Name=tag:app,Values=%s Name=tag:suite,Values=%s Name=instance-state-name,Values=pending,running,stopping,stopped --query 'Reservations[].Instances[].InstanceId' --output text)"; if [ -n "$ids" ] && [ "$ids" != "None" ]; then aws ec2 terminate-instances --region %s --endpoint-url %s --instance-ids $ids >/dev/null; aws ec2 wait instance-terminated --region %s --endpoint-url %s --instance-ids $ids; fi`,
			ec2Region,
			s.localstackURL,
			ec2TagApp,
			ec2TagSuite,
			ec2Region,
			s.localstackURL,
			ec2Region,
			s.localstackURL,
		),
	)
}

func (s *testingSuite) createEc2Instance() string {
	instanceID := s.runAwsCommand(
		"aws", "ec2", "run-instances",
		"--region", ec2Region,
		"--endpoint-url", s.localstackURL,
		"--image-id", "ami-ff0fea8310f3",
		"--count", "1",
		"--instance-type", "t3.nano",
		"--tag-specifications", fmt.Sprintf("ResourceType=instance,Tags=[{Key=app,Value=%s},{Key=suite,Value=%s}]", ec2TagApp, ec2TagSuite),
		"--query", "Instances[0].InstanceId",
		"--output", "text",
	)
	s.Require().NotEmpty(instanceID, "created instance should have an ID")

	s.runAwsCommand(
		"aws", "ec2", "wait", "instance-running",
		"--region", ec2Region,
		"--endpoint-url", s.localstackURL,
		"--instance-ids", instanceID,
	)

	privateIP := s.runAwsCommand(
		"aws", "ec2", "describe-instances",
		"--region", ec2Region,
		"--endpoint-url", s.localstackURL,
		"--instance-ids", instanceID,
		"--query", "Reservations[0].Instances[0].PrivateIpAddress",
		"--output", "text",
	)
	s.Require().NotEmpty(privateIP, "created instance should have a private IP")
	return privateIP
}

func (s *testingSuite) runAwsCommand(args ...string) string {
	var out bytes.Buffer
	err := s.testInstallation.Actions.Kubectl().
		WithReceiver(&out).
		RunCommand(s.ctx, append([]string{"exec", "-n", testNamespace, awsCliPodName, "--"}, args...)...)
	s.Require().NoError(err, "aws cli command should succeed: %s", strings.Join(args, " "))
	return strings.TrimSpace(out.String())
}

func (s *testingSuite) runAwsShell(script string) {
	_ = s.runAwsCommand("sh", "-c", script)
}

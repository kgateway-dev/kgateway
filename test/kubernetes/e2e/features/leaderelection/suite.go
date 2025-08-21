package leaderelection

import (
	"context"
	"strings"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/tests/base"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

type testingSuite struct {
	*base.BaseTestingSuite
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		base.NewBaseTestingSuite(ctx, testInst, setup, testCases),
	}
}

func (s *testingSuite) TestLeaderAndFollowerAction() {
	leader := s.getLeader()

	// Scale the deployment to 2 replicas so the other can take over when the leader is killed
	err := s.TestInstallation.Actions.Kubectl().Scale(s.Ctx, s.TestInstallation.Metadata.InstallNamespace, defaults.KGatewayDeployment, 2)
	s.NoError(err)
	defer func() {
		err = s.TestInstallation.Actions.Kubectl().Scale(s.Ctx, s.TestInstallation.Metadata.InstallNamespace, defaults.KGatewayDeployment, 1)
		s.NoError(err)
	}()

	// Kill the leader. Translation should still occur but the  should not be written while a new leader is elected.
	s.killLeader(leader)

	// Since the route does not exist, it should return a 404
	s.assertCurlResponseCode(404)

	// Create a route. The following should happen in order :
	// - It should be translated by the follower
	// - It should not have a status set since the leader is deleted but the lease has not expired
	// - Once a leader is elected, it should be accepted
	err = s.TestInstallation.Actions.Kubectl().ApplyFile(s.Ctx, routeManifest)
	s.NoError(err)
	defer func() {
		err = s.TestInstallation.Actions.Kubectl().DeleteFile(s.Ctx, routeManifest)
		s.NoError(err)
	}()

	s.assertCurlResponseCode(200)
	s.assertRouteHasNoStatus()
	s.TestInstallation.Assertions.EventuallyHTTPRouteCondition(s.Ctx, routeObjectMeta.Name, routeObjectMeta.Namespace, gwv1.RouteConditionAccepted, metav1.ConditionTrue)

	// Verify that a new leader was elected
	s.leadershipChanges(leader)
}

// Certain CRs such as backends have their status written in an event handler rather than through translation.
// This test verifies that status writing for such resources is handled by the leader.
func (s *testingSuite) TestLeaderWritesBackendStatus() {
	leader := s.getLeader()

	// Scale the deployment to 2 replicas so the other can take over when the leader is killed
	err := s.TestInstallation.Actions.Kubectl().Scale(s.Ctx, s.TestInstallation.Metadata.InstallNamespace, defaults.KGatewayDeployment, 2)
	s.NoError(err)
	defer func() {
		err = s.TestInstallation.Actions.Kubectl().Scale(s.Ctx, s.TestInstallation.Metadata.InstallNamespace, defaults.KGatewayDeployment, 1)
		s.NoError(err)
	}()

	// Kill the leader. No status should be written until a new leader has been elected.
	s.killLeader(leader)

	// The backend status is written in an event handler and not part of translation per-se.
	// This verifies that the status of resources not parsed through translation is also written by the leader.
	err = s.TestInstallation.Actions.Kubectl().ApplyFile(s.Ctx, backendManifest)
	s.NoError(err)
	defer func() {
		err = s.TestInstallation.Actions.Kubectl().DeleteFile(s.Ctx, backendManifest)
		s.NoError(err)
	}()

	s.assertBackendHasNoStatus()

	begin := time.Now()
	s.TestInstallation.Assertions.EventuallyBackendCondition(s.Ctx, "httpbin-static", "default", "Accepted", metav1.ConditionTrue)
	diff := time.Since(begin)

	// The time to deploy the write the status is greater than the lease renewal period.
	s.Greater(diff, leaseRenewPeriod)

	// Verify that a new leader was elected
	s.leadershipChanges(leader)
}

func (s *testingSuite) TestLeaderDeploysProxy() {
	leader := s.getLeader()

	// Scale the deployment to 2 replicas so the other can take over when the leader is killed
	err := s.TestInstallation.Actions.Kubectl().Scale(s.Ctx, s.TestInstallation.Metadata.InstallNamespace, defaults.KGatewayDeployment, 2)
	s.NoError(err)
	defer func() {
		err = s.TestInstallation.Actions.Kubectl().Scale(s.Ctx, s.TestInstallation.Metadata.InstallNamespace, defaults.KGatewayDeployment, 1)
		s.NoError(err)
	}()

	// Kill the leader. When a gateway is created, it should not be deployed until a new leader is elected.
	s.killLeader(leader)

	// Create a gateway. It should not be deployed until a new leader is elected
	err = s.TestInstallation.Actions.Kubectl().ApplyFile(s.Ctx, gatewayManifest)
	s.NoError(err)
	defer func() {
		err = s.TestInstallation.Actions.Kubectl().DeleteFile(s.Ctx, gatewayManifest)
		s.NoError(err)
	}()

	begin := time.Now()
	s.TestInstallation.Assertions.EventuallyObjectsExist(s.Ctx, proxyDeployment, proxyService)
	diff := time.Since(begin)

	// The time to deploy the proxy is greater than the lease renewal period.
	s.Greater(diff, leaseRenewPeriod)

	// Verify that a new leader was elected
	s.leadershipChanges(leader)
}

func (s *testingSuite) getLeader() string {
	var leaderPodName string
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		holder, err := s.TestInstallation.Actions.Kubectl().GetLeaseHolder(s.Ctx, s.TestInstallation.Metadata.InstallNamespace, wellknown.LeaderElectionID)
		assert.NoError(c, err, "failed to get lease")
		// Get the name of the pod that holds the lease
		// kgateway-6bb7674b97-cn6dd_f14c6a7e-ba31-40a7-95fb-806111275cd3 -> kgateway-6bb7674b97-cn6dd
		leaderPodName = strings.Split(holder, "_")[0]

		// Ensure the lease holder is in the list of running pods. This prevents fetching a stale lease when the leader changes
		pods, err := s.TestInstallation.Actions.Kubectl().GetPodsInNsWithLabel(s.Ctx, s.TestInstallation.Metadata.InstallNamespace, defaults.KGatewayPodLabel)
		assert.NoError(c, err, "failed to get lease")
		assert.Contains(c, pods, leaderPodName)
	}, 120*time.Second, 10*time.Second)
	return leaderPodName
}

func (s *testingSuite) leadershipChanges(oldLeader string) string {
	var holder string
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		holder = s.getLeader()
		assert.NotEqual(c, holder, oldLeader, "leadership did not change")
	}, 30*time.Second, 10*time.Second)
	return holder
}

func (s *testingSuite) killLeader(leader string) {
	// Kill the leader so another pod can assume leadership
	_, _, err := s.TestInstallation.Actions.Kubectl().Execute(s.Ctx, "delete", "pod", "-n", s.TestInstallation.Metadata.InstallNamespace, leader)
	s.NoError(err)
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		_, _, err := s.TestInstallation.Actions.Kubectl().Execute(s.Ctx, "get", "pod", "-n", s.TestInstallation.Metadata.InstallNamespace, leader)
		assert.Error(c, err, "Failed to delete leader")
	}, 120*time.Second, 1*time.Second)
}

func (s *testingSuite) assertCurlResponseCode(code int) {
	s.TestInstallation.Assertions.AssertEventualCurlResponse(
		s.Ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHostHeader("www.example.com"),
			curl.WithPath("/status/200"),
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
		},
		&matchers.HttpResponse{
			StatusCode: code,
		},
		20*time.Second,
		2*time.Second,
	)
}

func (s *testingSuite) assertRouteHasNoStatus() {
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		route := &gwv1.HTTPRoute{}
		err := s.TestInstallation.ClusterContext.Client.Get(s.Ctx, types.NamespacedName{Name: routeObjectMeta.Name, Namespace: routeObjectMeta.Namespace}, route)
		assert.NoError(c, err, "failed to get HTTPRoute")
		assert.Empty(c, route.Status.Parents)
	}, 120*time.Second, 1*time.Second)
}

func (s *testingSuite) assertBackendHasNoStatus() {
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		backend := &v1alpha1.Backend{}
		err := s.TestInstallation.ClusterContext.Client.Get(s.Ctx, types.NamespacedName{Name: "httpbin-static", Namespace: "default"}, backend)
		assert.NoError(c, err, "failed to get Backend")
		assert.Empty(c, backend.Status.Conditions)
	}, 120*time.Second, 1*time.Second)
}

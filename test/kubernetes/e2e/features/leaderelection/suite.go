package leaderelection

import (
	"context"
	"strings"
	"time"

	"github.com/onsi/gomega"
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
	var holder string
	var err error
	s.TestInstallation.Assertions.Gomega.Eventually(func(g gomega.Gomega) {
		holder, err = s.TestInstallation.Actions.Kubectl().GetLeaseHolder(s.Ctx, s.TestInstallation.Metadata.InstallNamespace, wellknown.LeaderElectionID)
		g.Expect(err).NotTo(gomega.HaveOccurred(), "failed to get lease")
		// This is the name of the pod that holds the lease
		holder = strings.Split(holder, "_")[0]

		// Ensure the lease holder is in the list of running pods. This prevents fetching a stale lease when the leader changes
		pods, err := s.TestInstallation.Actions.Kubectl().GetPodsInNsWithLabel(s.Ctx, s.TestInstallation.Metadata.InstallNamespace, defaults.KGatewayPodLabel)
		g.Expect(err).NotTo(gomega.HaveOccurred(), "failed to pods")
		g.Expect(pods).To(gomega.ContainElement(holder))
	}, "30s", "10s").Should(gomega.Succeed())
	return holder
}

func (s *testingSuite) leadershipChanges(oldLeader string) string {
	var holder string
	s.TestInstallation.Assertions.Gomega.Eventually(func(g gomega.Gomega) {
		holder = s.getLeader()
		g.Expect(holder).ToNot(gomega.Equal(oldLeader))
	}, "30s", "10s").Should(gomega.Succeed())
	return holder
}

func (s *testingSuite) killLeader(leader string) {
	// Kill the leader so another pod can assume leadership
	_, _, err := s.TestInstallation.Actions.Kubectl().Execute(s.Ctx, "delete", "pod", "-n", s.TestInstallation.Metadata.InstallNamespace, leader)
	s.NoError(err)
	s.TestInstallation.Assertions.Gomega.Eventually(func(g gomega.Gomega) {
		_, _, err := s.TestInstallation.Actions.Kubectl().Execute(s.Ctx, "get", "pod", "-n", s.TestInstallation.Metadata.InstallNamespace, leader)
		g.Expect(err).To(gomega.HaveOccurred(), "Failed to delete leader")
	}, "120s", "1s").Should(gomega.Succeed())
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
	s.TestInstallation.Assertions.Gomega.Eventually(func(g gomega.Gomega) {
		route := &gwv1.HTTPRoute{}
		err := s.TestInstallation.ClusterContext.Client.Get(s.Ctx, types.NamespacedName{Name: routeObjectMeta.Name, Namespace: routeObjectMeta.Namespace}, route)
		g.Expect(err).NotTo(gomega.HaveOccurred(), "failed to get HTTPRoute")
		g.Expect(route.Status.Parents).To(gomega.BeEmpty())
	}, "120s", "1s").Should(gomega.Succeed())
}

func (s *testingSuite) assertBackendHasNoStatus() {
	s.TestInstallation.Assertions.Gomega.Eventually(func(g gomega.Gomega) {
		backend := &v1alpha1.Backend{}
		err := s.TestInstallation.ClusterContext.Client.Get(s.Ctx, types.NamespacedName{Name: "httpbin-static", Namespace: "default"}, backend)
		g.Expect(err).NotTo(gomega.HaveOccurred(), "failed to get Backend")
		g.Expect(backend.Status.Conditions).To(gomega.BeEmpty())
	}, "120s", "1s").Should(gomega.Succeed())
}

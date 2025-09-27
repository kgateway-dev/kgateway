package zero_downtime_rollout

import (
	"context"
	"net/http"
	"time"

	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils/kubectl"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/tests/base"
)

type testingSuite struct {
	*base.BaseTestingSuite
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		base.NewBaseTestingSuite(ctx, testInst, setup, testCases),
	}
}

func (s *testingSuite) TestZeroDowntimeRollout() {
	// Ensure the gateway pod is up and running.
	s.TestInstallation.Assertions.EventuallyPodsRunning(s.Ctx,
		proxyObjectMeta.GetNamespace(), metav1.ListOptions{
			LabelSelector: "app.kubernetes.io/name=" + proxyObjectMeta.GetName(),
		})

	s.TestInstallation.Assertions.AssertEventualCurlResponse(
		s.Ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyObjectMeta)),
			curl.WithHostHeader("example.com"),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
		})

	kCli := kubectl.NewCli()

	// Send traffic to the gateway pod while we restart the deployment.
	// Run this for 30s which is long enough to restart the deployment since there's no easy way
	// to stop this command once the test is over.
	// This executes 800 req @ 4 req/sec = 20s (3 * terminationGracePeriodSeconds (5) + buffer).
	// kubectl exec -n hey heygw -- hey -disable-keepalive -c 4 -q 10 --cpus 1 -n 800 -m GET -t 1 -host example.com http://gw.default.svc.cluster.local:8080.
	args := []string{"exec", "-n", "hey", "heygw", "--", "hey", "-disable-keepalive", "-c", "4", "-q", "10", "--cpus", "1", "-n", "800", "-m", "GET", "-t", "1", "-host", "example.com", "http://gw.default.svc.cluster.local:8080"}

	cmd := kCli.Command(s.Ctx, args...)

	if err := cmd.Start(); err != nil {
		s.T().Fatal("error starting command", err)
	}

	// Restart the deployment, twice.
	// There should be no downtime, since the gateway pod
	// should have readiness probes configured.
	err := kCli.RestartDeploymentAndWait(s.Ctx, "gw")
	s.Require().NoError(err)

	time.Sleep(time.Second)

	err = kCli.RestartDeploymentAndWait(s.Ctx, "gw")
	s.Require().NoError(err)

	if err := cmd.Wait(); err != nil {
		s.T().Fatal("error waiting for command to finish", err)
	}

	// Verify that there were no errors.
	s.Contains(string(cmd.Output()), "[200]	800 responses")
	s.NotContains(string(cmd.Output()), "Error distribution")
}

func (s *testingSuite) TestZeroDowntimeRolloutAgentGateway() {
	// Ensure the agentgateway pod is up and running.
	s.TestInstallation.Assertions.EventuallyPodsRunning(s.Ctx,
		agentgatewayObjectMeta.GetNamespace(), metav1.ListOptions{
			LabelSelector: "app.kubernetes.io/name=" + agentgatewayObjectMeta.GetName(),
		})

	s.TestInstallation.Assertions.AssertEventualCurlResponse(
		s.Ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(agentgatewayObjectMeta)),
			curl.WithHostHeader("example.com"),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
		})

	kCli := kubectl.NewCli()

	// Send traffic to the gateway pod while we restart the deployment.
	// Run this for 30s which is long enough to restart the deployment since there's no easy way
	// to stop this command once the test is over.
	// This executes 800 req @ 4 req/sec = 20s (3 * terminationGracePeriodSeconds (5) + buffer).
	// kubectl exec -n hey heyagw -- hey -disable-keepalive -c 4 -q 10 --cpus 1 -n 800 -m GET -t 1 -host example.com http://agentgw.default.svc.cluster.local:8080.
	args := []string{"exec", "-n", "hey", "heyagw", "--", "hey", "-disable-keepalive", "-c", "4", "-q", "10", "--cpus", "1", "-n", "800", "-m", "GET", "-t", "1", "-host", "example.com", "http://agentgw.default.svc.cluster.local:8080"}

	cmd := kCli.Command(s.Ctx, args...)

	if err := cmd.Start(); err != nil {
		s.T().Fatal("error starting command", err)
	}

	// Restart the deployment, twice.
	// There should be no downtime, since the gateway pod
	// should have readiness probes configured.
	err := kCli.RestartDeploymentAndWait(s.Ctx, "agentgw")
	s.Require().NoError(err)

	time.Sleep(time.Second)

	err = kCli.RestartDeploymentAndWait(s.Ctx, "agentgw")
	s.Require().NoError(err)

	if err := cmd.Wait(); err != nil {
		s.T().Fatal("error waiting for command to finish", err)
	}

	// Verify that there were no errors.
	s.Contains(string(cmd.Output()), "[200]	800 responses")
	s.NotContains(string(cmd.Output()), "Error distribution")
}

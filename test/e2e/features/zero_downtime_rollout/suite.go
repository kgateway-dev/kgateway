//go:build e2e

package zero_downtime_rollout

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils/kubectl"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/common"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/helpers"
)

var (
	serviceManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "service.yaml")
	setupManifest   = filepath.Join(fsutils.MustGetThisDir(), "testdata", "setup.yaml")

	gatewayName = "gw"
)

type testingSuiteKgateway struct {
	*base.BaseTestingSuite
}

func NewTestingSuiteKgateway(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuiteKgateway{
		base.NewBaseTestingSuite(
			ctx,
			testInst,
			base.TestCase{
				Manifests: []string{serviceManifest, setupManifest},
			},
			map[string]*base.TestCase{
				"TestZeroDowntimeRollout":           {},
				"TestZeroDowntimeControllerRestart": {},
			},
		),
	}
}

// startTrafficAndAssertNoErrors runs load through the gateway while restartFunc
// executes. It fails if hey reports any transport errors or non-200 statuses.
func (s *testingSuiteKgateway) startTrafficAndAssertNoErrors(duration time.Duration, restartFunc func()) {
	kCli := kubectl.NewCli()
	args := []string{
		"exec", "-n", "hey", "heygw", "--",
		"hey", "-disable-keepalive",
		"-c", "4", "-q", "10", "--cpus", "1",
		"-z", duration.String(),
		"-m", "GET", "-t", "1",
		"-host", "example.com",
		"http://gw.default.svc.cluster.local:8080",
	}
	cmdCtx, cancel := context.WithCancel(s.Ctx)
	cmd := kCli.Command(cmdCtx, args...)

	if err := cmd.Start(); err != nil {
		cancel()
		s.T().Fatal("error starting command", err)
	}

	waited := false
	defer func() {
		cancel()
		if !waited {
			_ = cmd.Wait()
		}
	}()

	restartFunc()

	waitErr := cmd.Wait()
	waited = true
	if waitErr != nil {
		s.T().Fatalf("error waiting for command to finish: %v\ncommand output:\n%s", waitErr, string(cmd.Output()))
	}

	output := string(cmd.Output())
	s.NotContains(output, "Error distribution")

	seenStatusLine := false
	for line := range strings.SplitSeq(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "[") {
			continue
		}
		seenStatusLine = true
		s.True(strings.HasPrefix(trimmed, "[200]\t"), "unexpected hey status distribution line: %s\nfull output:\n%s", trimmed, output)
	}
	s.True(seenStatusLine, "hey output did not include a status distribution:\n%s", output)
}

func (s *testingSuiteKgateway) TestZeroDowntimeRollout() {
	// Ensure the gateway pod is up and running.
	s.TestInstallation.AssertionsT(s.T()).EventuallyPodsRunning(s.Ctx,
		"default", metav1.ListOptions{
			LabelSelector: defaults.WellKnownAppLabel + "=" + gatewayName,
		})

	common.BaseGateway.Send(
		s.T(),
		&testmatchers.HttpResponse{StatusCode: 200},
		curl.WithHostHeader("example.com"),
		curl.WithPort(8080),
	)

	kCli := kubectl.NewCli()

	// Keep traffic running across the entire rollout window instead of relying
	// on a fixed request count that may finish before both restarts complete.
	s.startTrafficAndAssertNoErrors(30*time.Second, func() {
		err := kCli.RestartDeploymentAndWait(s.Ctx, gatewayName)
		s.Require().NoError(err)

		time.Sleep(time.Second)

		err = kCli.RestartDeploymentAndWait(s.Ctx, gatewayName)
		s.Require().NoError(err)
	})
}

func (s *testingSuiteKgateway) TestZeroDowntimeControllerRestart() {
	// Ensure the gateway pod is up and running before exercising controller restarts.
	s.TestInstallation.AssertionsT(s.T()).EventuallyPodsRunning(s.Ctx,
		"default", metav1.ListOptions{
			LabelSelector: defaults.WellKnownAppLabel + "=" + gatewayName,
		})

	common.BaseGateway.Send(
		s.T(),
		&testmatchers.HttpResponse{StatusCode: 200},
		curl.WithHostHeader("example.com"),
		curl.WithPort(8080),
	)

	kCli := kubectl.NewCli()
	installNamespace := s.TestInstallation.Metadata.InstallNamespace

	// Restart the controller while traffic is in flight. The test install
	// publishes not-ready controller endpoints so Envoy can reconnect before
	// the control plane finishes warming, which widens the xDS race window.
	s.startTrafficAndAssertNoErrors(30*time.Second, func() {
		err := kCli.RestartDeploymentAndWait(s.Ctx, helpers.DefaultKgatewayDeploymentName, "-n", installNamespace)
		s.Require().NoError(err)

		time.Sleep(time.Second)

		err = kCli.RestartDeploymentAndWait(s.Ctx, helpers.DefaultKgatewayDeploymentName, "-n", installNamespace)
		s.Require().NoError(err)
	})

	// Apply a fresh route after the restart to prove the controller is pushing
	// new xDS, not just letting Envoy coast on cached config.
	postRestartRoute := `apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: post-restart-route
spec:
  parentRefs:
    - name: gw
  hostnames:
    - "post-restart.example.com"
  rules:
    - backendRefs:
        - name: example-svc
          port: 8080
`

	err := s.TestInstallation.ClusterContext.IstioClient.ApplyYAMLContents("default", postRestartRoute)
	s.Require().NoError(err)
	defer func() {
		_ = kCli.RunCommand(s.Ctx, "delete", "httproute", "post-restart-route", "-n", "default", "--ignore-not-found")
	}()

	common.BaseGateway.Send(
		s.T(),
		&testmatchers.HttpResponse{StatusCode: 200},
		curl.WithHostHeader("post-restart.example.com"),
		curl.WithPort(8080),
	)
}

//go:build e2e

package controlplanetls

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/threadsafe"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	testdefaults "github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/envoyutils/admincli"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

const (
	xdsWiretapContainer = "xds-wiretap"
	xdsWiretapPcap      = "/tmp/xds.pcap"
	xdsWiretapLog       = "/tmp/xds-tcpdump.log"
	xdsWiretapPID       = "/tmp/xds-tcpdump.pid"
	xdsWireProbeRoute   = "xds-wire-probe"
)

// AssertXDSWireTrafficIsEncrypted captures xDS traffic between the proxy and
// the controller and asserts that no plaintext xDS payload appears on the
// wire.
func AssertXDSWireTrafficIsEncrypted(ctx context.Context, t *testing.T, testInstallation e2e.Installation) {
	t.Helper()

	testInstallation.Assertions(t).EventuallyPodsRunning(ctx, "default", metav1.ListOptions{
		LabelSelector: testdefaults.HttpbinLabelSelector,
	})
	testInstallation.Assertions(t).EventuallyReadyReplicas(ctx, metav1.ObjectMeta{
		Name:      "gw",
		Namespace: "default",
	}, gomega.Equal(1), 120*time.Second, 500*time.Millisecond)

	proxyPod := getReadyProxyPod(ctx, t, testInstallation)
	startXDSCapture(ctx, t, testInstallation, proxyPod)
	testutils.Cleanup(t, func() {
		// Best-effort: if a t.Fatalf between start and stopXDSCapture left tcpdump
		// running, kill it now and surface the log so early failures aren't blind.
		stdout, _, _ := testInstallation.ActionsProvider().Kubectl().Execute(
			context.Background(),
			"exec", "-n", proxyPod.Namespace, proxyPod.Name,
			"-c", xdsWiretapContainer, "--",
			"sh", "-c",
			fmt.Sprintf("if [ -s %[1]s ]; then kill -INT $(cat %[1]s) 2>/dev/null || true; fi; cat %[2]s 2>/dev/null || true", xdsWiretapPID, xdsWiretapLog),
		)
		if t.Failed() && stdout != "" {
			t.Logf("xDS wiretap tcpdump log on failure:\n%s", stdout)
		}
	})

	markerHost := fmt.Sprintf("xds-wire-%d.example.com", time.Now().UnixNano())
	markerRoute := xdsWireProbeRouteManifest(markerHost)
	testutils.Cleanup(t, func() {
		_ = testInstallation.ActionsProvider().Kubectl().Delete(context.Background(), []byte(markerRoute))
	})
	if err := testInstallation.ActionsProvider().Kubectl().Apply(ctx, []byte(markerRoute)); err != nil {
		t.Fatalf("failed to apply xDS wire probe route: %v", err)
	}
	testInstallation.Assertions(t).EventuallyHTTPRouteCondition(
		ctx,
		xdsWireProbeRoute,
		"default",
		gwv1.RouteConditionAccepted,
		metav1.ConditionTrue,
	)
	assertEnvoyReceivedRouteHost(ctx, t, testInstallation, markerHost)

	capture, tcpdumpLog := stopXDSCapture(ctx, t, testInstallation, proxyPod)
	if len(capture) <= 24 {
		t.Fatalf("xDS packet capture did not contain packet payloads; tcpdump log:\n%s", tcpdumpLog)
	}
	assertXDSCaptureDoesNotContainPlaintext(t, capture, markerHost, tcpdumpLog)
}

func assertEnvoyReceivedRouteHost(ctx context.Context, t *testing.T, testInstallation e2e.Installation, markerHost string) {
	t.Helper()

	testInstallation.Assertions(t).AssertEnvoyAdminApi(
		ctx,
		metav1.ObjectMeta{Name: "gw", Namespace: "default"},
		func(ctx context.Context, adminClient *admincli.Client) {
			testInstallation.Assertions(t).Gomega.Eventually(func(g gomega.Gomega) {
				var configDump threadsafe.Buffer
				err := adminClient.ConfigDumpCmd(ctx, map[string]string{
					"resource": "dynamic_route_configs",
				}).WithStdout(&configDump).Run().Cause()
				g.Expect(err).NotTo(gomega.HaveOccurred(), "can get Envoy route config dump")
				g.Expect(configDump.String()).To(gomega.ContainSubstring(markerHost), "Envoy should receive the marked route over xDS")
			}).
				WithContext(ctx).
				WithTimeout(30 * time.Second).
				WithPolling(500 * time.Millisecond).
				Should(gomega.Succeed())
		},
	)
}

func getReadyProxyPod(ctx context.Context, t *testing.T, testInstallation e2e.Installation) *corev1.Pod {
	t.Helper()

	var proxyPod *corev1.Pod
	testInstallation.Assertions(t).Gomega.Eventually(func(g gomega.Gomega) {
		var readyProxyPod *corev1.Pod
		pods, err := testInstallation.Cluster().Clientset.CoreV1().Pods("default").List(ctx, metav1.ListOptions{
			LabelSelector: testdefaults.WellKnownAppLabel + "=gw",
		})
		g.Expect(err).NotTo(gomega.HaveOccurred(), "failed to list proxy pods")
		g.Expect(pods.Items).NotTo(gomega.BeEmpty(), "expected a proxy pod")

		for i := range pods.Items {
			pod := &pods.Items[i]
			if pod.Status.Phase != corev1.PodRunning || !podContainerReady(pod, xdsWiretapContainer) {
				continue
			}
			readyProxyPod = pod.DeepCopy()
			break
		}
		g.Expect(readyProxyPod).NotTo(gomega.BeNil(), "expected a running proxy pod with a ready xDS wiretap sidecar")
		proxyPod = readyProxyPod
	}).
		WithContext(ctx).
		WithTimeout(60 * time.Second).
		WithPolling(500 * time.Millisecond).
		Should(gomega.Succeed())

	return proxyPod
}

func podContainerReady(pod *corev1.Pod, containerName string) bool {
	for _, status := range pod.Status.ContainerStatuses {
		if status.Name == containerName {
			return status.Ready
		}
	}
	return false
}

func startXDSCapture(ctx context.Context, t *testing.T, testInstallation e2e.Installation, proxyPod *corev1.Pod) {
	t.Helper()

	filter := fmt.Sprintf("tcp port %d and (((ip[2:2] - ((ip[0]&0xf)<<2)) - ((tcp[12]&0xf0)>>2)) != 0)", wellknown.DefaultXdsPort)
	script := fmt.Sprintf(
		"rm -f %[1]s %[2]s %[3]s; nohup tcpdump -U -i any -s 0 -w %[1]s '%[4]s' >%[2]s 2>&1 & echo $! >%[3]s",
		xdsWiretapPcap,
		xdsWiretapLog,
		xdsWiretapPID,
		filter,
	)
	execWiretap(ctx, t, testInstallation, proxyPod, script)

	testInstallation.Assertions(t).Gomega.Eventually(func(g gomega.Gomega) {
		out := execWiretap(ctx, t, testInstallation, proxyPod, fmt.Sprintf("cat %s 2>/dev/null || true", xdsWiretapLog))
		g.Expect(out).To(gomega.ContainSubstring("listening on"), "tcpdump should be listening before the route update")
	}).
		WithContext(ctx).
		WithTimeout(10 * time.Second).
		WithPolling(200 * time.Millisecond).
		Should(gomega.Succeed())
}

func stopXDSCapture(ctx context.Context, t *testing.T, testInstallation e2e.Installation, proxyPod *corev1.Pod) ([]byte, string) {
	t.Helper()

	stopScript := fmt.Sprintf(
		"if [ -s %[1]s ]; then kill -INT $(cat %[1]s) 2>/dev/null || true; fi; sleep 1; cat %[2]s 2>/dev/null || true",
		xdsWiretapPID,
		xdsWiretapLog,
	)
	tcpdumpLog := execWiretap(ctx, t, testInstallation, proxyPod, stopScript)
	capture := execWiretap(ctx, t, testInstallation, proxyPod, fmt.Sprintf("cat %s", xdsWiretapPcap))

	return []byte(capture), tcpdumpLog
}

func execWiretap(ctx context.Context, t *testing.T, testInstallation e2e.Installation, proxyPod *corev1.Pod, script string) string {
	t.Helper()

	stdout, stderr, err := testInstallation.ActionsProvider().Kubectl().Execute(
		ctx,
		"exec",
		"-n",
		proxyPod.Namespace,
		proxyPod.Name,
		"-c",
		xdsWiretapContainer,
		"--",
		"sh",
		"-c",
		script,
	)
	if err != nil {
		t.Fatalf("failed to execute wiretap command %q: %v\nstdout:\n%s\nstderr:\n%s", script, err, stdout, stderr)
	}
	return stdout
}

func assertXDSCaptureDoesNotContainPlaintext(t *testing.T, capture []byte, markerHost string, tcpdumpLog string) {
	t.Helper()

	for _, plaintext := range [][]byte{
		[]byte(markerHost),
		[]byte("type.googleapis.com/envoy."),
		[]byte("envoy.config."),
		[]byte("StreamAggregatedResources"),
		[]byte("PRI * HTTP/2.0"),
	} {
		if bytes.Contains(capture, plaintext) {
			t.Fatalf(
				"xDS packet capture contained plaintext %q; xDS traffic between the proxy and controller is not encrypted\npcap size: %d bytes\ntcpdump log:\n%s",
				string(plaintext),
				len(capture),
				tcpdumpLog,
			)
		}
	}
}

func xdsWireProbeRouteManifest(host string) string {
	return fmt.Sprintf(`apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: %s
  namespace: default
spec:
  parentRefs:
    - name: gw
  hostnames:
    - %q
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: /
      backendRefs:
        - name: httpbin
          port: 8000
`, xdsWireProbeRoute, host)
}

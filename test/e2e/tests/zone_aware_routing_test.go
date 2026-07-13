//go:build e2e

package tests_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycommonv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/load_balancing_policies/common/v3"
	envoyroundrobinv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/load_balancing_policies/round_robin/v3"
	"github.com/onsi/gomega"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils/portforward"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/testutils/cluster"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/testutils/install"
	testruntime "github.com/kgateway-dev/kgateway/v2/test/e2e/testutils/runtime"
	"github.com/kgateway-dev/kgateway/v2/test/envoyutils/admincli"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

const (
	zoneAwareClusterNameEnv = "ZONE_AWARE_CLUSTER_NAME"
	defaultZoneAwareCluster = "kgw-zone-aware"
	zoneAwareNamespace      = "zone-aware"
	zoneAwareGateway        = "zone-gw"
	zoneAwareService        = "zone-backend"
	zoneAwareBackendCluster = "kube_zone-aware_zone-backend_80"

	zoneA = "us-east-1a"
	zoneB = "us-east-1b"
	zoneC = "us-east-1c"
)

var zoneAwareZones = []string{zoneA, zoneB, zoneC}

// Zone-aware e2e tests require multi-node setup, thus not registered to e2e suite and should be run manually.
// See the zone-aware-routing.md guide for details.

func TestZoneAwareRouting(t *testing.T) {
	ctx := context.Background()
	testInstallation := newZoneAwareTestInstallation(t)

	testutils.Cleanup(t, func() {
		cleanupCtx := context.Background()
		if t.Failed() {
			testInstallation.PreFailHandler(cleanupCtx, t)
		}
		_ = deleteZoneAwarePolicies(cleanupCtx, testInstallation)
		_ = testInstallation.Actions.Kubectl().DeleteFileSafe(cleanupCtx, zoneAwareTestData("base.yaml"))
	})

	assertions := testInstallation.AssertionsT(t)
	assertions.EventuallyReadyReplicas(ctx, metav1.ObjectMeta{
		Name:      "kgateway",
		Namespace: "kgateway-system",
	}, gomega.BeNumerically(">=", 1), 2*time.Minute)

	require.NoError(t, deleteZoneAwarePolicies(ctx, testInstallation))
	applyZoneAwareManifest(t, ctx, testInstallation, zoneAwareTestData("base.yaml"))

	waitForZoneAwareResources(t, ctx, testInstallation)
	waitForZoneAwareCluster(t, ctx, testInstallation, false)

	t.Run("default routing is evenly distributed across zones", func(t *testing.T) {
		counts := eventuallyDistribution(t, ctx, testInstallation, 90, expectEvenDistribution)
		t.Logf("default distribution: %s", counts)
	})

	t.Run("prefer local routes all traffic to the local zone with enough capacity", func(t *testing.T) {
		applyZoneAwareManifest(t, ctx, testInstallation, zoneAwareTestData("prefer-local.yaml"))
		waitForZoneAwareCluster(t, ctx, testInstallation, true)
		waitForEndpointCount(t, ctx, testInstallation, 3)

		counts := eventuallyDistribution(t, ctx, testInstallation, 90, expectAllLocal)
		t.Logf("prefer-local distribution: %s", counts)
	})

	t.Run("prefer local spills over when local capacity is insufficient", func(t *testing.T) {
		scaleZoneAwareDeployment(t, ctx, testInstallation, "zone-backend-b", 3)
		scaleZoneAwareDeployment(t, ctx, testInstallation, "zone-backend-c", 2)
		waitForEndpointCount(t, ctx, testInstallation, 6)

		counts := eventuallyDistribution(t, ctx, testInstallation, 120, expectSomeCrossZone)
		t.Logf("prefer-local insufficient-capacity distribution: %s", counts)
	})

	t.Run("force local keeps traffic local even when local capacity is insufficient", func(t *testing.T) {
		applyZoneAwareManifest(t, ctx, testInstallation, zoneAwareTestData("force-local.yaml"))
		waitForZoneAwareCluster(t, ctx, testInstallation, true)
		waitForEndpointCount(t, ctx, testInstallation, 6)

		counts := eventuallyDistribution(t, ctx, testInstallation, 120, expectAllLocal)
		t.Logf("force-local distribution: %s", counts)
	})
}

func newZoneAwareTestInstallation(t *testing.T) *e2e.TestInstallation {
	t.Helper()

	clusterName := os.Getenv(zoneAwareClusterNameEnv)
	if clusterName == "" {
		clusterName = defaultZoneAwareCluster
	}

	runtimeContext := testruntime.Context{
		ClusterName: clusterName,
		RunSource:   testruntime.LocalDevelopment,
	}
	clusterContext := cluster.MustKindContext(clusterName)
	installContext := &install.Context{
		InstallNamespace:          "kgateway-system",
		ProfileValuesManifestFile: e2e.CommonRecommendationManifest,
		ValuesManifestFile:        e2e.EmptyValuesManifestPath,
	}

	return e2e.CreateTestInstallationForCluster(t, runtimeContext, clusterContext, installContext)
}

func waitForZoneAwareResources(t *testing.T, ctx context.Context, ti *e2e.TestInstallation) {
	t.Helper()

	for _, deployment := range []string{"zone-backend-a", "zone-backend-b", "zone-backend-c", zoneAwareGateway} {
		require.NoError(t,
			ti.Actions.Kubectl().DeploymentRolloutStatus(ctx, deployment, "-n", zoneAwareNamespace, "--timeout=180s"),
			"deployment %s should roll out", deployment)
	}

	assertions := ti.AssertionsT(t)
	assertions.EventuallyGatewayCondition(ctx, zoneAwareGateway, zoneAwareNamespace, gwv1.GatewayConditionProgrammed, metav1.ConditionTrue, 2*time.Minute)
	assertions.EventuallyHTTPRouteCondition(ctx, "zone-route", zoneAwareNamespace, gwv1.RouteConditionAccepted, metav1.ConditionTrue, 2*time.Minute)
	assertions.EventuallyReadyReplicas(ctx, metav1.ObjectMeta{
		Name:      zoneAwareGateway,
		Namespace: zoneAwareNamespace,
	}, gomega.Equal(3), 2*time.Minute)
	waitForGatewayPodsInZones(t, ctx, ti)
	requireGatewayPodTopologyLabels(t, ctx, ti)
	waitForEndpointCount(t, ctx, ti, 3)
}

func requireGatewayPodTopologyLabels(t *testing.T, ctx context.Context, ti *e2e.TestInstallation) {
	t.Helper()

	nodes := &corev1.NodeList{}
	require.NoError(t, ti.ClusterContext.Client.List(ctx, nodes), "list nodes")
	nodeZones := map[string]string{}
	for _, node := range nodes.Items {
		nodeZones[node.Name] = node.Labels[corev1.LabelTopologyZone]
	}

	pods := &corev1.PodList{}
	require.NoError(t, ti.ClusterContext.Client.List(ctx, pods,
		crclient.InNamespace(zoneAwareNamespace),
		crclient.MatchingLabels{wellknown.GatewayNameLabel: zoneAwareGateway},
	), "list gateway pods")

	for _, pod := range pods.Items {
		if pod.Spec.NodeName == "" || !podReady(&pod) {
			continue
		}
		require.Equal(t, nodeZones[pod.Spec.NodeName], pod.Labels[corev1.LabelTopologyZone],
			"gateway pod %s must carry its node's topology.kubernetes.io/zone label", pod.Name)
	}
}

func waitForZoneAwareCluster(t *testing.T, ctx context.Context, ti *e2e.TestInstallation, expectZoneAware bool) {
	t.Helper()

	withZoneAGatewayEnvoyAdmin(t, ctx, ti, func(ctx context.Context, adminClient *admincli.Client) {
		var lastErr error
		require.Eventually(t, func() bool {
			clusters, err := adminClient.GetDynamicClusters(ctx)
			if err != nil {
				lastErr = fmt.Errorf("get dynamic clusters: %w", err)
				return false
			}

			cluster, ok := clusters[zoneAwareBackendCluster]
			if !ok {
				lastErr = fmt.Errorf("cluster %s not found", zoneAwareBackendCluster)
				return false
			}

			if expectZoneAware {
				// A BackendConfigPolicy loadBalancer always produces a typed
				// load_balancing_policy; zone-aware config lives in the typed
				// policy's locality_lb_config, not in common_lb_config.
				localityLbConfig, err := typedRoundRobinLocalityLbConfig(cluster)
				if err != nil {
					lastErr = err
					return false
				}
				if localityLbConfig.GetZoneAwareLbConfig() == nil {
					lastErr = fmt.Errorf("cluster %s does not have zone_aware_lb_config in its typed load_balancing_policy", zoneAwareBackendCluster)
					return false
				}
				return true
			}

			// Without a BackendConfigPolicy the cluster must default to
			// locality-weighted LB in common_lb_config so that Envoy's implicit
			// zone-aware defaults do not kick in.
			if cluster.GetLoadBalancingPolicy() != nil {
				lastErr = fmt.Errorf("cluster %s unexpectedly has a typed load_balancing_policy", zoneAwareBackendCluster)
				return false
			}
			if cluster.GetCommonLbConfig().GetLocalityWeightedLbConfig() == nil {
				lastErr = fmt.Errorf("cluster %s does not default to locality_weighted_lb_config, got: %v", zoneAwareBackendCluster, cluster.GetCommonLbConfig())
				return false
			}
			return true
		}, 90*time.Second, time.Second, "last xDS error: %v", lastErr)
	})
}

func typedRoundRobinLocalityLbConfig(cluster *envoyclusterv3.Cluster) (*envoycommonv3.LocalityLbConfig, error) {
	policies := cluster.GetLoadBalancingPolicy().GetPolicies()
	if len(policies) == 0 {
		return nil, fmt.Errorf("cluster %s has no typed load_balancing_policy", cluster.GetName())
	}

	typedConfig := policies[0].GetTypedExtensionConfig().GetTypedConfig()
	roundRobin := &envoyroundrobinv3.RoundRobin{}
	if err := typedConfig.UnmarshalTo(roundRobin); err != nil {
		return nil, fmt.Errorf("cluster %s load_balancing_policy is not round_robin: %w", cluster.GetName(), err)
	}
	return roundRobin.GetLocalityLbConfig(), nil
}

func withZoneAGatewayEnvoyAdmin(
	t *testing.T,
	ctx context.Context,
	ti *e2e.TestInstallation,
	adminAssertion func(ctx context.Context, adminClient *admincli.Client),
) {
	t.Helper()

	gatewayPod := waitForGatewayPodInZone(t, ctx, ti, zoneA)
	portForwarder, err := ti.ClusterContext.Cli.StartPortForward(ctx,
		portforward.WithPod(gatewayPod, zoneAwareNamespace),
		portforward.WithRemotePort(int(wellknown.EnvoyAdminPort)),
	)
	require.NoError(t, err, "can open Envoy admin port-forward for pod %s", gatewayPod)
	defer func() {
		portForwarder.Close()
		portForwarder.WaitForStop()
	}()

	adminClient := admincli.NewClient().
		WithReceiver(io.Discard).
		WithCurlOptions(
			curl.WithRetries(10, 1, 10),
			curl.WithRetryConnectionRefused(true),
			curl.WithHostPort(portForwarder.Address()),
		)

	adminAssertion(ctx, adminClient)
}

func waitForGatewayPodsInZones(t *testing.T, ctx context.Context, ti *e2e.TestInstallation) {
	t.Helper()

	var lastCounts zoneCounts
	require.Eventually(t, func() bool {
		podsByZone, err := readyGatewayPodsByZone(ctx, ti)
		if err != nil {
			return false
		}

		lastCounts = zoneCounts{
			zoneA: len(podsByZone[zoneA]),
			zoneB: len(podsByZone[zoneB]),
			zoneC: len(podsByZone[zoneC]),
		}
		for _, zone := range zoneAwareZones {
			if lastCounts[zone] == 0 {
				return false
			}
		}
		return true
	}, 2*time.Minute, time.Second, "expected ready gateway pods in every zone, got %s", lastCounts)
}

func waitForGatewayPodInZone(t *testing.T, ctx context.Context, ti *e2e.TestInstallation, zone string) string {
	t.Helper()

	var gatewayPod string
	require.Eventually(t, func() bool {
		podsByZone, err := readyGatewayPodsByZone(ctx, ti)
		if err != nil {
			return false
		}

		pods := podsByZone[zone]
		if len(pods) == 0 {
			return false
		}
		gatewayPod = pods[0]
		return true
	}, 2*time.Minute, time.Second, "expected ready gateway pod in zone %s", zone)

	return gatewayPod
}

func readyGatewayPodsByZone(ctx context.Context, ti *e2e.TestInstallation) (map[string][]string, error) {
	nodes := &corev1.NodeList{}
	if err := ti.ClusterContext.Client.List(ctx, nodes); err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}

	nodeZones := map[string]string{}
	for _, node := range nodes.Items {
		nodeZones[node.Name] = node.Labels[corev1.LabelTopologyZone]
	}

	pods := &corev1.PodList{}
	if err := ti.ClusterContext.Client.List(ctx, pods,
		crclient.InNamespace(zoneAwareNamespace),
		crclient.MatchingLabels{wellknown.GatewayNameLabel: zoneAwareGateway},
	); err != nil {
		return nil, fmt.Errorf("list gateway pods: %w", err)
	}

	podsByZone := map[string][]string{}
	for _, pod := range pods.Items {
		if pod.Spec.NodeName == "" || !podReady(&pod) {
			continue
		}
		zone := nodeZones[pod.Spec.NodeName]
		if zone == "" {
			continue
		}
		podsByZone[zone] = append(podsByZone[zone], pod.Name)
	}
	return podsByZone, nil
}

func podReady(pod *corev1.Pod) bool {
	if pod.Status.Phase != corev1.PodRunning {
		return false
	}
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func waitForEndpointCount(t *testing.T, ctx context.Context, ti *e2e.TestInstallation, expected int) {
	t.Helper()

	var lastCount int
	require.Eventually(t, func() bool {
		endpointSlices := &discoveryv1.EndpointSliceList{}
		err := ti.ClusterContext.Client.List(ctx, endpointSlices,
			crclient.InNamespace(zoneAwareNamespace),
			crclient.MatchingLabels{"kubernetes.io/service-name": zoneAwareService},
		)
		if err != nil {
			return false
		}

		lastCount = 0
		for _, endpointSlice := range endpointSlices.Items {
			for _, endpoint := range endpointSlice.Endpoints {
				if endpoint.Conditions.Ready != nil && !*endpoint.Conditions.Ready {
					continue
				}
				lastCount += len(endpoint.Addresses)
			}
		}
		return lastCount == expected
	}, 90*time.Second, time.Second, "expected %d ready endpoints for %s, got %d", expected, zoneAwareService, lastCount)
}

func scaleZoneAwareDeployment(t *testing.T, ctx context.Context, ti *e2e.TestInstallation, deployment string, replicas uint) {
	t.Helper()

	require.NoError(t,
		ti.Actions.Kubectl().Scale(ctx, zoneAwareNamespace, "deployment/"+deployment, replicas),
		"scale deployment %s", deployment)
	require.NoError(t,
		ti.Actions.Kubectl().DeploymentRolloutStatus(ctx, deployment, "-n", zoneAwareNamespace, "--timeout=180s"),
		"deployment %s should roll out after scaling", deployment)
}

type zoneCounts map[string]int

func (c zoneCounts) total() int {
	total := 0
	for _, count := range c {
		total += count
	}
	return total
}

func (c zoneCounts) remoteTotal() int {
	return c[zoneB] + c[zoneC]
}

func (c zoneCounts) String() string {
	return fmt.Sprintf("%s=%d %s=%d %s=%d", zoneA, c[zoneA], zoneB, c[zoneB], zoneC, c[zoneC])
}

func eventuallyDistribution(
	t *testing.T,
	ctx context.Context,
	ti *e2e.TestInstallation,
	requests int,
	expect func(zoneCounts) error,
) zoneCounts {
	t.Helper()

	var lastCounts zoneCounts
	var lastErr error
	gatewayPod := waitForGatewayPodInZone(t, ctx, ti, zoneA)
	require.Eventually(t, func() bool {
		counts, err := collectDistribution(ctx, ti, gatewayPod, requests)
		lastCounts = counts
		if err != nil {
			lastErr = err
			return false
		}
		lastErr = expect(counts)
		return lastErr == nil
	}, 90*time.Second, 5*time.Second, "last distribution: %s; error: %v", lastCounts, lastErr)

	return lastCounts
}

func collectDistribution(ctx context.Context, ti *e2e.TestInstallation, gatewayPod string, requests int) (zoneCounts, error) {
	// Bound the whole attempt so an unreachable gateway fails this Eventually tick
	// quickly instead of blocking for up to requests x the per-request client timeout.
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	portForwarder, err := ti.ClusterContext.Cli.StartPortForward(ctx,
		portforward.WithPod(gatewayPod, zoneAwareNamespace),
		portforward.WithRemotePort(8080),
	)
	if err != nil {
		return nil, fmt.Errorf("start gateway port-forward: %w", err)
	}
	defer func() {
		portForwarder.Close()
		portForwarder.WaitForStop()
	}()

	httpClient := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			DisableKeepAlives: true,
		},
	}

	counts := zoneCounts{
		zoneA: 0,
		zoneB: 0,
		zoneC: 0,
	}
	for range requests {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+portForwarder.Address()+"/", nil)
		if err != nil {
			return counts, fmt.Errorf("build request: %w", err)
		}
		req.Close = true

		resp, err := httpClient.Do(req)
		if err != nil {
			return counts, fmt.Errorf("request through gateway: %w", err)
		}
		body, readErr := io.ReadAll(resp.Body)
		closeErr := resp.Body.Close()
		if readErr != nil {
			return counts, fmt.Errorf("read response body: %w", readErr)
		}
		if closeErr != nil {
			return counts, fmt.Errorf("close response body: %w", closeErr)
		}
		if resp.StatusCode != http.StatusOK {
			return counts, fmt.Errorf("unexpected status %d with body %q", resp.StatusCode, string(body))
		}

		zone := strings.TrimSpace(string(body))
		if _, ok := counts[zone]; !ok {
			return counts, fmt.Errorf("unexpected backend zone response %q", zone)
		}
		counts[zone]++
	}

	return counts, nil
}

// expectEvenDistribution checks that each of the 3 zones received between 20%
// and 60% of traffic. With equal-weight endpoints and zone-aware off, the expected
// share is ~33% per zone.
func expectEvenDistribution(counts zoneCounts) error {
	total := counts.total()
	if total == 0 {
		return fmt.Errorf("no responses recorded")
	}
	for _, zone := range zoneAwareZones {
		if counts[zone] < total/5 || counts[zone] > total*3/5 {
			return fmt.Errorf("zone %s count %d is outside expected even-distribution range [%d, %d] for total %d",
				zone, counts[zone], total/5, total*3/5, total)
		}
	}
	return nil
}

func expectAllLocal(counts zoneCounts) error {
	total := counts.total()
	if total == 0 {
		return fmt.Errorf("no responses recorded")
	}
	if counts[zoneA] != total {
		return fmt.Errorf("expected all %d requests to route to %s, got %s", total, zoneA, counts)
	}
	return nil
}

// expectSomeCrossZone checks for when zone-aware routing is on, but local
// capacity is insufficient (1 of 6). Envoy should send more traffic to the
// local zone than a proportional share, while spilling the excess cross-zone.
func expectSomeCrossZone(counts zoneCounts) error {
	total := counts.total()
	if total == 0 {
		return fmt.Errorf("no responses recorded")
	}
	if counts[zoneA] == 0 {
		return fmt.Errorf("expected some local-zone traffic, got %s", counts)
	}
	if counts.remoteTotal() == 0 {
		return fmt.Errorf("expected some cross-zone spillover, got %s", counts)
	}
	if counts[zoneA] <= counts.remoteTotal()/3 {
		return fmt.Errorf("expected local zone to receive more than proportional share, got %s", counts)
	}
	return nil
}

func applyZoneAwareManifest(t *testing.T, ctx context.Context, ti *e2e.TestInstallation, manifest string) {
	t.Helper()
	require.NoError(t, ti.Actions.Kubectl().ApplyFile(ctx, manifest), "apply %s", manifest)
}

func deleteZoneAwarePolicies(ctx context.Context, ti *e2e.TestInstallation) error {
	if err := ti.Actions.Kubectl().DeleteFileSafe(ctx, zoneAwareTestData("force-local.yaml")); err != nil {
		return err
	}
	return ti.Actions.Kubectl().DeleteFileSafe(ctx, zoneAwareTestData("prefer-local.yaml"))
}

func zoneAwareTestData(name string) string {
	return filepath.Join(testutils.GitRootDirectory(), "test", "e2e", "features", "zoneaware", "testdata", name)
}

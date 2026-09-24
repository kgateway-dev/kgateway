//go:build e2e

package loadtesting

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	discoveryv3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	resourcev3 "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/testutils/cluster"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

func TestValidationMetricsDeltaClampsCounterResets(t *testing.T) {
	before := ValidationMetrics{
		Calls:            10,
		CacheHits:        9,
		CacheMisses:      8,
		Valid:            7,
		InvalidXDS:       6,
		InvocationErrors: 5,
		DurationCount:    4,
		DurationSeconds:  3.5,
		ByCaller: map[string]ValidationCallerMetrics{
			"route_full": {
				Calls:            10,
				CacheHits:        9,
				CacheMisses:      8,
				Valid:            7,
				InvalidXDS:       6,
				InvocationErrors: 5,
				DurationCount:    4,
				DurationSeconds:  3.5,
			},
		},
	}
	after := ValidationMetrics{
		Calls:            1,
		CacheHits:        1,
		CacheMisses:      1,
		Valid:            1,
		InvalidXDS:       1,
		InvocationErrors: 1,
		DurationCount:    1,
		DurationSeconds:  1,
		ByCaller: map[string]ValidationCallerMetrics{
			"route_full": {
				Calls:            1,
				CacheHits:        1,
				CacheMisses:      1,
				Valid:            1,
				InvalidXDS:       1,
				InvocationErrors: 1,
				DurationCount:    1,
				DurationSeconds:  1,
			},
		},
	}

	delta := after.Delta(before)

	assert.Zero(t, delta.Calls)
	assert.Zero(t, delta.CacheHits)
	assert.Zero(t, delta.CacheMisses)
	assert.Zero(t, delta.Valid)
	assert.Zero(t, delta.InvalidXDS)
	assert.Zero(t, delta.InvocationErrors)
	assert.Zero(t, delta.DurationCount)
	assert.Zero(t, delta.DurationSeconds)

	caller := delta.ByCaller["route_full"]
	assert.Zero(t, caller.Calls)
	assert.Zero(t, caller.CacheHits)
	assert.Zero(t, caller.CacheMisses)
	assert.Zero(t, caller.Valid)
	assert.Zero(t, caller.InvalidXDS)
	assert.Zero(t, caller.InvocationErrors)
	assert.Zero(t, caller.DurationCount)
	assert.Zero(t, caller.DurationSeconds)
}

func TestFleetReadinessRequiresEveryLiveStream(t *testing.T) {
	clients := make([]*syntheticClient, 100)
	for i := range clients {
		clients[i] = &syntheticClient{done: make(chan struct{})}
		if i < 34 {
			clients[i].acks.Store(3)
		}
	}
	assert.Equal(t, 34, servedStreams(clients), "multiple resource ACKs must not count as additional served streams")
	for _, c := range clients {
		c.acks.Store(1)
	}
	assert.Equal(t, 100, servedStreams(clients))
	close(clients[0].done)
	assert.Equal(t, 99, servedStreams(clients), "terminated streams must not count as served")
	assert.Zero(t, servedStreams([]*syntheticClient{{done: make(chan struct{})}}), "older streams cannot satisfy a new stream's readiness")
}

func TestBenchConvergenceRequiresQuietWindow(t *testing.T) {
	oldFleetTimeout, oldCostTimeout := fleetIterTimeout, benchIterationTimeout
	oldFleetSettle, oldCostSettle := fleetSettleMillis, benchSettleMillis
	testutils.Cleanup(t, func() {
		fleetIterTimeout, benchIterationTimeout = oldFleetTimeout, oldCostTimeout
		fleetSettleMillis, benchSettleMillis = oldFleetSettle, oldCostSettle
	})
	fleetIterTimeout, benchIterationTimeout = 300*time.Millisecond, 300*time.Millisecond
	fleetSettleMillis, benchSettleMillis = 1000, 1000
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(w, "kgateway_xds_snapshot_transforms_total 1")
	}))
	defer server.Close()
	fleet := &XdsFleetSuite{metricsURL: server.URL}
	fleet.SetT(t)
	_, ok := fleet.waitConverged(0, true)
	assert.False(t, ok, "a transform without a full quiet window must time out")
	cost := &XdsCostSuite{metricsURL: server.URL}
	cost.SetT(t)
	_, ok = cost.waitConverged(0, time.Now())
	assert.False(t, ok, "cost suite must also require a full quiet window")
}

func TestBenchQuietWindow(t *testing.T) {
	for _, changing := range []bool{false, true} {
		t.Run(fmt.Sprintf("changing=%v", changing), func(t *testing.T) {
			transforms := 1
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if changing {
					transforms++
				}
				fmt.Fprintf(w, "kgateway_xds_snapshot_transforms_total %d\n", transforms)
			}))
			defer server.Close()
			cost := &XdsCostSuite{metricsURL: server.URL}
			cost.SetT(t)
			assert.Equal(t, !changing, cost.waitQuiet(100*time.Millisecond, time.Second),
				"cost measurements require an uninterrupted quiet window")
			fleet := &XdsFleetSuite{metricsURL: server.URL}
			fleet.SetT(t)
			assert.Equal(t, !changing, fleet.waitQuiet(100*time.Millisecond, time.Second),
				"fleet measurements require an uninterrupted quiet window")
		})
	}
}

func TestBenchmarkEnvRestoresValueSources(t *testing.T) {
	original := map[string]*corev1.EnvVar{
		"SECRET":  {Name: "SECRET", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{Key: "secret"}}},
		"CONFIG":  {Name: "CONFIG", ValueFrom: &corev1.EnvVarSource{ConfigMapKeyRef: &corev1.ConfigMapKeySelector{Key: "config"}}},
		"FIELD":   {Name: "FIELD", ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.name"}}},
		"LITERAL": {Name: "LITERAL", Value: "original"},
		"ADDED":   nil,
	}
	current := []corev1.EnvVar{{Name: "SECRET", Value: "override"}, {Name: "ADDED", Value: "temporary"}, {Name: "UNRELATED", Value: "keep"}}
	restored := restoredBenchmarkEnv(current, original)
	assert.Len(t, restored, 5)
	assert.Contains(t, restored, current[2])
	for _, env := range original {
		if env != nil {
			assert.Contains(t, restored, *env)
		}
	}
}

func TestBenchmarkResourceRestoration(t *testing.T) {
	for _, original := range []corev1.ResourceRequirements{
		{},
		{Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("2Gi")}},
		{Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("2Gi")}, Requests: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("1Gi")}},
	} {
		scheme := runtime.NewScheme()
		require.NoError(t, appsv1.AddToScheme(scheme))
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "controller", Namespace: "test"},
			Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "controller", Resources: corev1.ResourceRequirements{
					Limits:   corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("8Gi")},
					Requests: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("8Gi")},
				}},
				{Name: "sidecar", Image: "unchanged"},
			}}}},
		}
		kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(deployment).Build()
		require.NoError(t, updateBenchmarkContainer(context.Background(), kube, "test", "controller", "controller", func(c *corev1.Container) {
			c.Resources = *original.DeepCopy()
		}))
		var restored appsv1.Deployment
		require.NoError(t, kube.Get(context.Background(), client.ObjectKeyFromObject(deployment), &restored))
		assert.Equal(t, original, restored.Spec.Template.Spec.Containers[0].Resources)
		assert.Equal(t, deployment.Spec.Template.Spec.Containers[1], restored.Spec.Template.Spec.Containers[1])
	}
}

func TestFleetOpenRetryPreservesPreviousConnection(t *testing.T) {
	conn, err := grpc.NewClient("localhost:0", grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Force stream opens to fail deterministically.
	fleet := &XdsFleetSuite{
		LoadTestingSuite: LoadTestingSuite{ctx: ctx},
		conns:            []*grpc.ClientConn{conn}, activeConn: conn, activeConnStreams: 1,
		xdsAddrs: []string{"localhost:0"},
	}
	defer func() {
		for _, c := range fleet.conns {
			_ = c.Close()
		}
	}()
	_, err = fleet.openStreamRetrying("role", "pod.namespace")
	require.Error(t, err)
	assert.NotEqual(t, connectivity.Shutdown, conn.GetState(), "retry must not close the connection carrying earlier streams")
	assert.Contains(t, fleet.conns, conn, "original connection remains owned for teardown")
	assert.Greater(t, len(fleet.conns), 1, "retry should allocate fresh connections")
}

func TestFleetCrashEmitsVerdictWithUnavailableMetrics(t *testing.T) {
	oldGateways, oldWaves, oldStreams := fleetGateways, fleetWaves, fleetStreamsPerGateway
	testutils.Cleanup(t, func() {
		fleetGateways, fleetWaves, fleetStreamsPerGateway = oldGateways, oldWaves, oldStreams
	})
	// Skip network stream creation and exercise the wave observation path.
	fleetGateways, fleetWaves, fleetStreamsPerGateway = 1, 1, 0
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "controller-pod", Namespace: "test"},
		Status:     corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{Name: "controller", RestartCount: 2}}},
	}
	kube := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(pod).WithObjects(pod).Build()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		var current corev1.Pod
		if err := kube.Get(context.Background(), client.ObjectKeyFromObject(pod), &current); err != nil {
			t.Error(err)
		}
		current.Status.ContainerStatuses[0].RestartCount = 3
		if err := kube.Status().Update(context.Background(), &current); err != nil {
			t.Error(err)
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	outputPath := filepath.Join(t.TempDir(), "verdict.jsonl")
	output, err := os.Create(outputPath)
	require.NoError(t, err)
	defer output.Close()
	fleet := &XdsFleetSuite{
		LoadTestingSuite: LoadTestingSuite{
			ctx:              context.Background(),
			testInstallation: &e2e.TestInstallation{ClusterContext: &cluster.Context{Client: kube}},
		},
		installNamespace: "test", controllerDeployment: "controller",
		gateways: []string{"gateway"}, metricsURL: server.URL, out: output,
	}
	fleet.SetT(t)
	fleet.TestXdsFleet()
	assert.False(t, fleet.waitServed(nil, time.Second), "a restart must prevent readiness even if all streams are ready")
	_, converged := fleet.waitConverged(0, true)
	assert.False(t, converged, "a restart must stop convergence polling")
	data, err := os.ReadFile(outputPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "xds_fleet_verdict")
	assert.Contains(t, string(data), "controller restarted")
	assert.Contains(t, string(data), "\"survived_gateways\":0")
	assert.NotContains(t, string(data), "xds_fleet_wave", "unavailable metrics must not produce a zero-valued wave")
}

func TestFleetQuietRejectsUnavailableMetrics(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	fleet := &XdsFleetSuite{metricsURL: server.URL}
	fleet.SetT(t)
	assert.False(t, fleet.waitQuiet(0, 100*time.Millisecond), "an HTTP failure is not a quiet controller")
	_, err := readControllerSample(server.URL)
	require.ErrorContains(t, err, "503")
}

func TestFleetClientCountAccountsForRepeatedIdentities(t *testing.T) {
	oldLocality, oldStreams, oldZones, oldIdentity := fleetPodLocality, fleetStreamsPerGateway, fleetZones, fleetIdentityIncludeNode
	testutils.Cleanup(t, func() {
		fleetPodLocality, fleetStreamsPerGateway, fleetZones, fleetIdentityIncludeNode = oldLocality, oldStreams, oldZones, oldIdentity
	})
	for _, tc := range []struct {
		name        string
		locality    bool
		streams     int
		includeNode string
		want        int
	}{
		{"role only", false, 10, "", 1},
		{"distinct nodes", true, 2, "", 2},
		{"repeated nodes", true, 10, "true", 6},
		{"distinct zones", true, 2, "false", 2},
		{"repeated zones", true, 10, "false", 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fleetPodLocality, fleetStreamsPerGateway, fleetZones, fleetIdentityIncludeNode = tc.locality, tc.streams, 3, tc.includeNode
			assert.Equal(t, tc.want, (&XdsFleetSuite{}).clientsPerGateway())
		})
	}
}

func TestFleetEndpointSubscriptions(t *testing.T) {
	response := &discoveryv3.DiscoveryResponse{}
	for _, cluster := range []*envoyclusterv3.Cluster{
		{Name: "inline", ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_STATIC}},
		{Name: "service", ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_EDS}},
		{Name: "named-service", ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_EDS}, EdsClusterConfig: &envoyclusterv3.Cluster_EdsClusterConfig{ServiceName: "endpoint-name"}},
	} {
		resource, err := utils.MessageToAny(cluster)
		require.NoError(t, err)
		response.Resources = append(response.Resources, resource)
	}
	names, err := fleetEndpointNames(response, "local-cluster")
	require.NoError(t, err)
	assert.Equal(t, []string{"local-cluster", "service", "endpoint-name"}, names)
}

func TestBenchmarkMetricsTrackSuccessfulGateways(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(w, `# TYPE kgateway_xds_snapshot_transforms_total counter
kgateway_xds_snapshot_transforms_total{namespace="bench",gateway="ready",result="success"} 1
kgateway_xds_snapshot_transforms_total{namespace="bench",gateway="failed",result="error"} 2
kgateway_xds_snapshot_transforms_total{namespace="bench",gateway="idle",result="success"} 0`)
	}))
	defer server.Close()
	sample, err := readControllerSample(server.URL)
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"bench/ready": true}, sample.TransformedGateways)
}

func TestBenchmarkEnvPreservesExplicitEmptyValue(t *testing.T) {
	t.Setenv("KGW_TEST_BENCH_EMPTY", "")
	assert.Empty(t, benchEnvString("KGW_TEST_BENCH_EMPTY", "8Gi"))
}

func TestBenchmarkControllerSelectionUsesAppLabel(t *testing.T) {
	t.Setenv("KGW_LOADTEST_CONTROLLER_APP", "kgateway")
	scheme := runtime.NewScheme()
	require.NoError(t, appsv1.AddToScheme(scheme))
	controller := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "custom-controller", Namespace: "test", Labels: map[string]string{"app.kubernetes.io/name": "kgateway"}},
		Spec:       appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "controller"}}}}},
	}
	decoy := controller.DeepCopy()
	decoy.Name, decoy.Labels = "kgateway-decoy", nil
	kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(controller, decoy).Build()
	base := func() LoadTestingSuite {
		return LoadTestingSuite{ctx: context.Background(), testInstallation: &e2e.TestInstallation{ClusterContext: &cluster.Context{Client: kube}}}
	}
	cost := &XdsCostSuite{LoadTestingSuite: base(), installNamespace: "test"}
	fleet := &XdsFleetSuite{LoadTestingSuite: base(), installNamespace: "test"}
	for _, resolve := range []func() (string, string, error){cost.resolveController, fleet.resolveController} {
		name, container, err := resolve()
		require.NoError(t, err)
		assert.Equal(t, "custom-controller", name)
		assert.Equal(t, "controller", container)
	}
}

type fleetTestStream struct {
	grpc.ClientStream
	responses []*discoveryv3.DiscoveryResponse
	sent      []*discoveryv3.DiscoveryRequest
}

func (s *fleetTestStream) Send(request *discoveryv3.DiscoveryRequest) error {
	s.sent = append(s.sent, request)
	return nil
}

func (s *fleetTestStream) Recv() (*discoveryv3.DiscoveryResponse, error) {
	if len(s.responses) == 0 {
		return nil, io.EOF
	}
	response := s.responses[0]
	s.responses = s.responses[1:]
	return response, nil
}

func TestFleetPumpPreservesEndpointSubscriptionOnACK(t *testing.T) {
	stream := &fleetTestStream{responses: []*discoveryv3.DiscoveryResponse{
		{TypeUrl: resourcev3.ClusterType, VersionInfo: "cds", Nonce: "cds-nonce"},
		{TypeUrl: resourcev3.EndpointType, VersionInfo: "eds", Nonce: "eds-nonce"},
		{TypeUrl: resourcev3.RouteType, VersionInfo: "rds", Nonce: "rds-nonce"},
	}}
	c := &syntheticClient{stream: stream, localClusterName: "local-cluster", done: make(chan struct{})}
	c.pump(&envoycorev3.Node{Id: "proxy"})
	require.Len(t, stream.sent, 4)
	assert.Equal(t, resourcev3.EndpointType, stream.sent[1].TypeUrl)
	assert.Equal(t, []string{"local-cluster"}, stream.sent[1].ResourceNames)
	assert.Equal(t, stream.sent[1].ResourceNames, stream.sent[2].ResourceNames)
	assert.Equal(t, "eds-nonce", stream.sent[2].ResponseNonce)
	assert.Equal(t, "rds-nonce", stream.sent[3].ResponseNonce)
	assert.EqualValues(t, 3, c.acks.Load())
}

func TestFleetConvergence(t *testing.T) {
	oldSettle, oldTimeout := fleetSettleMillis, fleetIterTimeout
	fleetSettleMillis, fleetIterTimeout = 100, 2*time.Second
	testutils.Cleanup(t, func() {
		fleetSettleMillis, fleetIterTimeout = oldSettle, oldTimeout
	})

	for _, tc := range []struct {
		name             string
		requireTransform bool
		values           []int // A negative value represents an unavailable metrics endpoint.
		want             bool
	}{
		{name: "cached reconnect", values: []int{10}, want: true},
		{name: "churn requires a transform", requireTransform: true, values: []int{10}},
		{name: "churn transforms", requireTransform: true, values: []int{11}, want: true},
		{name: "scrape failure after transform", requireTransform: true, values: []int{11, -1, 11}, want: true},
		{name: "unavailable metrics", values: []int{-1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				value := tc.values[min(calls, len(tc.values)-1)]
				calls++
				if value < 0 {
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				fmt.Fprintf(w, "# TYPE kgateway_xds_snapshot_transforms_total counter\nkgateway_xds_snapshot_transforms_total %d\n", value)
			}))
			defer server.Close()
			s := &XdsFleetSuite{metricsURL: server.URL}
			_, got := s.waitConverged(10, tc.requireTransform)
			if got != tc.want {
				t.Fatalf("converged = %v, want %v", got, tc.want)
			}
		})
	}
}

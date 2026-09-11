//go:build e2e

package loadtesting

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoylistenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	discoveryv3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/structpb"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	discoverykv1 "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/xds"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils/portforward"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
)

// XdsFleet is XdsCost at production fan-out. XdsCost tops out around twenty
// Gateways because each one runs real Envoy pods; a production control plane
// serves thousands of Services and hundreds of Gateways, and no single machine
// can host the corresponding sixteen hundred proxies.
//
// This suite reaches that scale by separating the two things a proxy provides:
//
//   - Its *identity*, which is what drives per-client translation. A
//     UniquelyConnectedClient is keyed by role, namespace, labels and locality,
//     and the role is the Gateway. With pod-locality xDS disabled the client is
//     the role alone, so N Gateways are N unique clients, and every replica of
//     one Gateway collapses into the same client — two proxies per Gateway is
//     still one client, which is why the shape below is Gateways, not pods.
//   - Its *xDS stream*, which is what the snapshot is pushed to. Streams are
//     cheap, so the suite opens them directly against the controller's xDS port
//     with the Gateway's role in the node metadata. Two streams per Gateway
//     reproduces two proxies without scheduling them.
//
// The Gateways themselves are real, because the controller only builds a
// snapshot for a role it has translated a Gateway for; their proxy Deployments
// are pinned to zero replicas so no pods are scheduled.
//
// Clients connect in waves, and the controller is measured after each wave, so
// a build that runs out of memory still yields the curve up to where it died
// instead of one failed run. Controller restarts are detected and reported.
//
//	make run-xds-fleet-bench
type XdsFleetSuite struct {
	LoadTestingSuite
	installNamespace     string
	controllerDeployment string
	controllerContainer  string
	originalEnv          map[string]*corev1.EnvVar

	testNamespace string
	gateways      []string

	metricsPF  portforward.PortForwarder
	metricsURL string
	xdsPF      portforward.PortForwarder
	xdsAddr    string

	clients []*syntheticClient
	conns   []*grpc.ClientConn

	out *os.File
}

// Fleet shape. The defaults describe the production shape this was built to
// answer: thousands of Kubernetes Services with a couple of endpoints each, a
// handful of inline-CLA backends, and hundreds of Gateways.
var (
	fleetServices       = benchEnvInt("KGW_FLEET_SERVICES", 6000)
	fleetGateways       = benchEnvInt("KGW_FLEET_GATEWAYS", 800)
	fleetInlineBackends = benchEnvInt("KGW_FLEET_INLINE_BACKENDS", 20)
	// fleetStreamsPerGateway reproduces replica count. It multiplies streams,
	// not unique clients.
	fleetStreamsPerGateway = benchEnvInt("KGW_FLEET_STREAMS_PER_GATEWAY", 2)
	// fleetWaves is how many equal waves the Gateways connect in.
	fleetWaves = benchEnvInt("KGW_FLEET_WAVES", 4)
	// fleetStreamsPerConn trades HTTP/2 streams per connection against the
	// number of TCP connections. It has to be high: these connections run
	// through `kubectl port-forward`, which starts refusing new ones with
	// "error reading server preface: EOF" somewhere past sixty, so a fleet of
	// sixteen hundred streams must not ask for sixty-four connections.
	fleetStreamsPerConn = benchEnvInt("KGW_FLEET_STREAMS_PER_CONN", 100)
	fleetIterations     = benchEnvInt("KGW_FLEET_ITERATIONS", 5)
	fleetCreateWorkers  = benchEnvInt("KGW_FLEET_CREATE_WORKERS", 24)
	fleetSettleMillis   = benchEnvInt("KGW_FLEET_SETTLE_MS", 3000)
	fleetWaveTimeout    = time.Duration(benchEnvInt("KGW_FLEET_WAVE_TIMEOUT_SECONDS", 900)) * time.Second
	fleetIterTimeout    = time.Duration(benchEnvInt("KGW_FLEET_ITERATION_TIMEOUT_SECONDS", 600)) * time.Second
	fleetXdsPort        = benchEnvInt("KGW_FLEET_XDS_PORT", 9977)
	// fleetMemoryLimit caps the controller. Without a limit a build that does
	// not fit consumes the whole machine and takes the cluster with it; with
	// one, not fitting is a clean container restart that this suite detects,
	// and "fits in this much at N clients" becomes the reproducible criterion.
	// Set KGW_FLEET_MEMORY_LIMIT="" to run uncapped.
	fleetMemoryLimit = benchEnvString("KGW_FLEET_MEMORY_LIMIT", "8Gi")
	// fleetZones spreads the fake nodes across topology zones. It only has an
	// effect with pod locality on, where it is what makes client identities and
	// load-balancing contexts genuinely differ.
	fleetZones = benchEnvInt("KGW_FLEET_ZONES", 3)
	// fleetPodLocality runs the fleet the way production runs by default, with
	// pod-locality xDS enabled. That requires every stream to resolve to a real
	// Pod object, so the suite creates fake Nodes and binds a fake Pod to one
	// per stream. Set false to fall back to role-only identities.
	fleetPodLocality = benchEnvString("KGW_FLEET_POD_LOCALITY", "true") == "true"
	// fleetIdentityIncludeNode forwards Settings.XdsIdentityIncludeNode to the
	// controller. Empty leaves the deployment's own value alone; "false" scopes
	// client identity to locality, which collapses a Gateway's replicas within
	// a zone into one client. Only meaningful on a build that has the setting.
	fleetIdentityIncludeNode = benchEnvString("KGW_FLEET_IDENTITY_INCLUDE_NODE", "")
	// fleetExtraEnv sets arbitrary controller environment for the run, as a
	// comma-separated list of KEY=VALUE. It exists so an A/B over a single
	// setting does not need a new knob here each time; the names are snapshotted
	// and restored in teardown like the rest.
	fleetExtraEnv = benchEnvString("KGW_FLEET_EXTRA_ENV", "")
)

// extraEnvPairs splits fleetExtraEnv into KEY=VALUE pairs, dropping anything
// that is not one so a stray comma cannot silently unset a controller variable.
func extraEnvPairs() []string {
	var pairs []string
	for _, kv := range strings.Split(fleetExtraEnv, ",") {
		kv = strings.TrimSpace(kv)
		if k, _, ok := strings.Cut(kv, "="); ok && k != "" {
			pairs = append(pairs, kv)
		}
	}
	return pairs
}

var _ e2e.NewSuiteFunc = NewXdsFleetSuite

func NewXdsFleetSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &XdsFleetSuite{
		LoadTestingSuite: LoadTestingSuite{
			Suite:            suite.Suite{},
			ctx:              ctx,
			testInstallation: testInst,
		},
	}
}

// syntheticClient is one xDS stream carrying a Gateway's identity. It is not a
// simulation of Envoy's behaviour beyond what drives control-plane cost:
// subscribe to the wildcard resources a proxy subscribes to, and acknowledge
// every response so the server considers the client caught up.
type syntheticClient struct {
	role     string
	nodeID   string
	stream   discoveryv3.AggregatedDiscoveryService_StreamAggregatedResourcesClient
	cancel   context.CancelFunc
	acks     atomic.Int64
	recvErrs atomic.Int64
	firstErr atomic.Pointer[string]
	done     chan struct{}
}

func (s *XdsFleetSuite) SetupSuite() {
	if os.Getenv("KGW_ENABLE_XDS_FLEET") != "true" {
		s.T().Skip("XdsFleet mutates the controller deployment; set KGW_ENABLE_XDS_FLEET=true (or use `make run-xds-fleet-bench`) to run it")
	}
	s.installNamespace = s.testInstallation.Metadata.InstallNamespace
	s.testNamespace = fmt.Sprintf("kgw-fleet-%d", time.Now().Unix())

	if path := os.Getenv("KGW_BENCH_OUT"); path != "" {
		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		s.Require().NoError(err)
		s.out = f
	}

	name, container, err := s.resolveController()
	s.Require().NoError(err, "should find the controller deployment in %s", s.installNamespace)
	s.controllerDeployment, s.controllerContainer = name, container

	// Pod-locality xDS off: with it on, a stream whose node id does not resolve
	// to a real Pod is rejected outright, and every synthetic client would need
	// a fake Pod. Off, the client is the Gateway role, which is the identity
	// that drives per-client translation either way.
	// xDS auth off: with it on the server takes the client's identity from a
	// Kubernetes ServiceAccount JWT on the gRPC metadata, so a stream without a
	// real pod's token is rejected outright. Off, the identity comes from the
	// node metadata role, which is what these streams carry. This changes how
	// the role is derived, not what is translated for it.
	extraEnv := extraEnvPairs()
	snapshotNames := []string{
		"KGW_VALIDATION_MODE", "DISABLE_POD_LOCALITY_XDS", "KGW_XDS_AUTH", "KGW_XDS_IDENTITY_INCLUDE_NODE",
	}
	for _, kv := range extraEnv {
		name, _, _ := strings.Cut(kv, "=")
		snapshotNames = append(snapshotNames, name)
	}
	s.Require().NoError(s.snapshotEnv(snapshotNames))
	localityEnv := "DISABLE_POD_LOCALITY_XDS=false"
	if !fleetPodLocality {
		localityEnv = "DISABLE_POD_LOCALITY_XDS=true"
	}
	envs := []string{
		"KGW_VALIDATION_MODE=" + benchValidation,
		localityEnv,
		"KGW_XDS_AUTH=false",
	}
	if fleetIdentityIncludeNode != "" {
		envs = append(envs, "KGW_XDS_IDENTITY_INCLUDE_NODE="+fleetIdentityIncludeNode)
	}
	envs = append(envs, extraEnv...)
	s.Require().NoError(s.setEnv(envs...))

	s.T().Logf("XdsFleet: build=%q validation=%s services=%d gateways=%d inline=%d streams/gw=%d waves=%d extraEnv=%v",
		benchLabel, benchValidation, fleetServices, fleetGateways, fleetInlineBackends, fleetStreamsPerGateway, fleetWaves, extraEnv)

	if fleetMemoryLimit != "" {
		s.T().Logf("capping controller memory at %s for the run", fleetMemoryLimit)
		s.Require().NoError(s.setMemoryLimit(fleetMemoryLimit), "should cap controller memory")
	}

	s.createNamespace()
	s.createZeroReplicaGatewayParameters()
	s.createGateways()
	if fleetPodLocality {
		s.createFakeNodes()
		s.createFakePods()
	}
	s.createServicesAndEndpoints()
	s.createInlineBackends()

	s.startForwards()
	s.waitForGatewaySnapshots()
}

func (s *XdsFleetSuite) TearDownSuite() {
	if s.testNamespace == "" {
		return
	}
	s.stopClients()
	for _, c := range s.conns {
		_ = c.Close()
	}
	if s.xdsPF != nil {
		s.xdsPF.Close()
		s.xdsPF.WaitForStop()
	}
	if s.metricsPF != nil {
		s.metricsPF.Close()
		s.metricsPF.WaitForStop()
	}
	if s.originalEnv != nil {
		if err := s.restoreEnv(); err != nil {
			s.T().Logf("failed to restore controller env (cluster left modified): %v", err)
		}
	}
	if fleetMemoryLimit != "" {
		// "0" removes the limit rather than setting it to zero.
		if err := s.setMemoryLimit("0"); err != nil {
			s.T().Logf("failed to remove controller memory cap (cluster left modified): %v", err)
		}
	}
	if s.out != nil {
		_ = s.out.Close()
	}
	// Fake proxy pods are bound to fake Nodes that no kubelet owns, so nothing
	// will ever confirm their graceful deletion: the namespace then hangs in
	// Terminating on "unexpected items still remain ... Resource=pods" forever,
	// and the leftovers poison the next run. Force them out first.
	if fleetPodLocality {
		if err := s.testInstallation.Actions.Kubectl().RunCommand(s.ctx,
			"delete", "pods", "-n", s.testNamespace, "--all",
			"--force", "--grace-period=0", "--wait=false",
		); err != nil {
			s.T().Logf("force-deleting fake proxy pods reported: %v", err)
		}
	}
	if fleetPodLocality {
		// Nodes are cluster-scoped and outlive the namespace.
		for i := range fleetZones * 2 {
			node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: s.nodeName(i)}}
			if err := s.testInstallation.ClusterContext.Client.Delete(s.ctx, node); err != nil {
				s.T().Logf("fake node delete reported: %v", err)
			}
		}
	}
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: s.testNamespace}}
	if err := s.testInstallation.ClusterContext.Client.Delete(s.ctx, ns); err != nil {
		s.T().Logf("namespace delete reported: %v", err)
	}
}

// TestXdsFleet connects the fleet in waves, measuring after each, then prices
// the churn events at full fan-out.
func (s *XdsFleetSuite) TestXdsFleet() {
	perWave := fleetGateways / fleetWaves
	if perWave == 0 {
		perWave = fleetGateways
	}
	connected := 0
	for wave := 1; connected < fleetGateways; wave++ {
		target := min(connected+perWave, fleetGateways)
		start := time.Now()
		s.connectGateways(connected, target)
		connected = target
		// A wave is settled when the controller stops emitting new snapshot
		// transforms. If it never settles the build is not keeping up, which is
		// itself the measurement.
		settled := s.waitQuiet(time.Duration(fleetSettleMillis)*time.Millisecond, fleetWaveTimeout)
		// A stream that never received a response is not a connected client, and
		// a fleet of those measures nothing at all. Stop rather than report a
		// zero as if it were a result.
		if acks := s.totalAcks(); acks == 0 {
			s.Require().FailNow("no synthetic xDS stream received a response",
				"connected %d gateways (%d streams) but got 0 acks and %d receive errors; first error: %s",
				connected, connected*fleetStreamsPerGateway, s.totalRecvErrors(), s.firstStreamError())
		}
		sample := s.scrape()
		restarts := s.controllerRestarts()
		s.emit("xds_fleet_wave", map[string]any{
			"build": benchLabel, "validation": benchValidation,
			"wave": wave, "gateways": connected,
			// With pod locality on, replicas of one Gateway land on different
			// nodes and are therefore different unique clients, so the client
			// count is the stream count, not the Gateway count.
			"clients":             connected * s.clientsPerGateway(),
			"streams":             connected * fleetStreamsPerGateway,
			"services":            fleetServices,
			"inline_backends":     fleetInlineBackends,
			"settled":             settled,
			"wave_seconds":        time.Since(start).Seconds(),
			"cpu_seconds":         sample.CPUSeconds,
			"heap_inuse_mb":       sample.HeapInuse / 1e6,
			"rss_mb":              sample.RSS / 1e6,
			"alloc_total_mb":      sample.AllocBytes / 1e6,
			"goroutines":          sample.Goroutines,
			"xds_resources":       sample.Resources,
			"xds_transforms":      sample.Transforms,
			"controller_restarts": restarts,
			"stream_acks":         s.totalAcks(),
			"stream_errors":       s.totalRecvErrors(),
		})
		s.T().Logf("wave %d: clients=%d settled=%v heap=%.0fMB rss=%.0fMB cpu=%.1fs resources=%.0f restarts=%d",
			wave, connected, settled, sample.HeapInuse/1e6, sample.RSS/1e6, sample.CPUSeconds, sample.Resources, restarts)
		if restarts > 0 {
			s.T().Logf("controller restarted during wave %d; the build did not survive %d clients at this shape", wave, connected)
			s.emit("xds_fleet_verdict", map[string]any{
				"build": benchLabel, "survived_clients": connected - perWave,
				"died_at_clients": connected, "reason": "controller restarted (out of memory or crash)",
			})
			return
		}
	}

	s.runFleetPhase("EdsChurn", func(i int) { s.churnEndpointSlice(i) })
	s.runFleetPhase("BaseChurn", func(i int) { s.churnInlineBackend(i) })
	s.runFleetPhase("StreamReconnect", func(i int) { s.reconnectOneGateway(i) })
}

// clientsPerGateway is how many unique clients one Gateway's replicas produce.
// With pod locality on, replica identity includes the node hostname, so replicas
// spread across nodes are distinct clients; with it off they collapse into one.
func (s *XdsFleetSuite) clientsPerGateway() int {
	if fleetPodLocality {
		return fleetStreamsPerGateway
	}
	return 1
}

func (s *XdsFleetSuite) runFleetPhase(name string, mutate func(int)) {
	s.T().Logf("=== fleet phase %s at %d clients", name, fleetGateways)
	s.waitQuiet(time.Duration(fleetSettleMillis)*time.Millisecond, fleetWaveTimeout)
	before := s.scrape()
	startRestarts := s.controllerRestarts()
	start := time.Now()
	var latencies []float64
	for i := range fleetIterations {
		t0 := time.Now()
		tBefore := s.scrape().Transforms
		mutate(i)
		last, ok := s.waitConverged(tBefore)
		if !ok {
			s.T().Logf("phase %s iteration %d did not converge in %s", name, i, fleetIterTimeout)
			latencies = append(latencies, float64(fleetIterTimeout.Milliseconds()))
			continue
		}
		latencies = append(latencies, float64(last.Sub(t0).Milliseconds()))
	}
	wall := time.Since(start)
	after := s.scrape()
	n := float64(max(len(latencies), 1))
	sortFloats(latencies)
	s.emit("xds_fleet_result", map[string]any{
		"build": benchLabel, "validation": benchValidation, "phase": name,
		"clients": fleetGateways, "services": fleetServices, "inline_backends": fleetInlineBackends,
		"iterations":            len(latencies),
		"wall_seconds":          wall.Seconds(),
		"cpu_ms_per_change":     (after.CPUSeconds - before.CPUSeconds) * 1000 / n,
		"alloc_mb_per_change":   (after.AllocBytes - before.AllocBytes) / 1e6 / n,
		"transforms_per_change": (after.Transforms - before.Transforms) / n,
		"deferrals_per_change":  (after.Deferrals - before.Deferrals) / n,
		"latency_ms_median":     percentile(latencies, 0.5),
		"latency_ms_max":        percentile(latencies, 1),
		"heap_inuse_mb_after":   after.HeapInuse / 1e6,
		"rss_mb_after":          after.RSS / 1e6,
		"controller_restarts":   s.controllerRestarts() - startRestarts,
	})
}

// ---- fleet construction ----

func (s *XdsFleetSuite) createNamespace() {
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
		Name:   s.testNamespace,
		Labels: map[string]string{"loadtest": "true"},
	}}
	err := s.testInstallation.ClusterContext.Client.Create(s.ctx, ns)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		s.Require().NoError(err, "should create namespace")
	}
}

// createZeroReplicaGatewayParameters pins proxy Deployments to zero replicas.
// The Gateways must exist for the controller to build snapshots for their roles,
// but their pods must not, because the point is to exceed what pods allow.
func (s *XdsFleetSuite) createZeroReplicaGatewayParameters() {
	gp := &unstructured.Unstructured{Object: map[string]any{
		"spec": map[string]any{
			"kube": map[string]any{
				"deployment": map[string]any{"replicas": int64(0)},
			},
		},
	}}
	gp.SetGroupVersionKind(gatewayParametersGVK)
	gp.SetName("fleet-zero-replicas")
	gp.SetNamespace(s.testNamespace)
	s.Require().NoError(s.testInstallation.ClusterContext.Client.Create(s.ctx, gp),
		"should create GatewayParameters")
}

func (s *XdsFleetSuite) createGateways() {
	s.gateways = make([]string, fleetGateways)
	for i := range s.gateways {
		s.gateways[i] = fmt.Sprintf("fleet-gw-%d", i)
	}
	start := time.Now()
	s.parallelDo(len(s.gateways), func(i int) error {
		gw := &gwv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      s.gateways[i],
				Namespace: s.testNamespace,
				Labels:    map[string]string{"loadtest": "true"},
			},
			Spec: gwv1.GatewaySpec{
				GatewayClassName: "kgateway",
				Infrastructure: &gwv1.GatewayInfrastructure{
					ParametersRef: &gwv1.LocalParametersReference{
						Group: "gateway.kgateway.dev",
						Kind:  "GatewayParameters",
						Name:  "fleet-zero-replicas",
					},
				},
				Listeners: []gwv1.Listener{{
					Name:     "http",
					Protocol: gwv1.HTTPProtocolType,
					Port:     80,
				}},
			},
		}
		return s.createIgnoreExists(gw)
	})
	s.T().Logf("created %d Gateways in %s", fleetGateways, time.Since(start).Round(time.Second))
}

// createServicesAndEndpoints builds the Kubernetes Service fleet: EDS clusters
// with a couple of endpoints each, which is what a real cluster is mostly made
// of. Endpoint counts alternate so the average is 1.5.
func (s *XdsFleetSuite) createServicesAndEndpoints() {
	start := time.Now()
	s.parallelDo(fleetServices, func(i int) error {
		name := fmt.Sprintf("fleet-svc-%d", i)
		svc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: s.testNamespace,
				Labels:    map[string]string{"loadtest": "true"},
			},
			Spec: corev1.ServiceSpec{
				ClusterIP: corev1.ClusterIPNone,
				Ports: []corev1.ServicePort{{
					Name: "http", Port: 8080,
					TargetPort: intstr.FromInt(8080), Protocol: corev1.ProtocolTCP,
				}},
			},
		}
		if err := s.createIgnoreExists(svc); err != nil {
			return err
		}
		// 1.5 endpoints on average.
		count := 1
		if i%2 == 0 {
			count = 2
		}
		endpoints := make([]discoverykv1.Endpoint, 0, count)
		ready := true
		for e := range count {
			endpoints = append(endpoints, discoverykv1.Endpoint{
				Addresses:  []string{fmt.Sprintf("10.%d.%d.%d", 100+(i/60000), (i/250)%250, (i*2+e)%250+1)},
				Conditions: discoverykv1.EndpointConditions{Ready: &ready},
			})
		}
		port := int32(8080)
		proto := corev1.ProtocolTCP
		portName := "http"
		eps := &discoverykv1.EndpointSlice{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: s.testNamespace,
				Labels: map[string]string{
					discoverykv1.LabelServiceName: name,
					"loadtest":                    "true",
				},
			},
			AddressType: discoverykv1.AddressTypeIPv4,
			Endpoints:   endpoints,
			Ports: []discoverykv1.EndpointPort{{
				Name: &portName, Port: &port, Protocol: &proto,
			}},
		}
		return s.createIgnoreExists(eps)
	})
	s.T().Logf("created %d Services with EndpointSlices in %s", fleetServices, time.Since(start).Round(time.Second))
}

// createInlineBackends creates the static Backends whose endpoints live in the
// object, so an edit to one is a cluster-base change.
func (s *XdsFleetSuite) createInlineBackends() {
	for i := range fleetInlineBackends {
		obj := &unstructured.Unstructured{Object: map[string]any{
			"spec": map[string]any{
				"static": map[string]any{
					"hosts": []any{map[string]any{
						"host": fmt.Sprintf("10.240.0.%d", i%250+1),
						"port": int64(8080),
					}},
				},
			},
		}}
		obj.SetGroupVersionKind(backendGVK)
		obj.SetName(fmt.Sprintf("fleet-inline-%d", i))
		obj.SetNamespace(s.testNamespace)
		obj.SetLabels(map[string]string{"loadtest": "true"})
		s.Require().NoError(s.createIgnoreExists(obj), "should create inline Backend %d", i)
	}
	s.T().Logf("created %d inline-CLA Backends", fleetInlineBackends)
}

// podName is the fake Pod backing one Gateway replica's stream.
func (s *XdsFleetSuite) podName(gatewayIdx, replica int) string {
	return fmt.Sprintf("fleet-proxy-%d-%d", gatewayIdx, replica)
}

// nodeName is per-run. Nodes are cluster-scoped, so a fixed name silently
// reuses the previous run's node — keeping its old zone labels and quietly
// invalidating any experiment that varies zones.
func (s *XdsFleetSuite) nodeName(i int) string {
	return fmt.Sprintf("%s-node-%d", s.testNamespace, i)
}

// createFakeNodes creates the Nodes the fake proxy Pods are bound to. Their
// topology labels are the only source of a client's locality, and binding pods
// to a node that no kubelet owns is what keeps 1600 proxies free.
func (s *XdsFleetSuite) createFakeNodes() {
	nodes := fleetZones * 2
	for i := range nodes {
		zone := fmt.Sprintf("zone-%d", i%fleetZones)
		node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{
			Name: s.nodeName(i),
			Labels: map[string]string{
				"loadtest":                 "true",
				corev1.LabelTopologyRegion: "region-0",
				corev1.LabelTopologyZone:   zone,
				corev1.LabelHostname:       s.nodeName(i),
			},
		}}
		s.Require().NoError(s.createIgnoreExists(node), "should create fake node %d", i)
	}
	s.T().Logf("created %d fake nodes across %d zones", nodes, fleetZones)
}

// createFakePods creates one Pod per stream, bound to a fake node and labelled
// with its Gateway. The controller reads the pod to derive the client's labels
// and locality, so without these every stream is rejected as "pod not found".
// Replicas of one Gateway are placed on different nodes, as anti-affinity would,
// which is what makes them distinct clients.
func (s *XdsFleetSuite) createFakePods() {
	nodes := fleetZones * 2
	start := time.Now()
	total := fleetGateways * fleetStreamsPerGateway
	s.parallelDo(total, func(n int) error {
		gwIdx, replica := n/fleetStreamsPerGateway, n%fleetStreamsPerGateway
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      s.podName(gwIdx, replica),
				Namespace: s.testNamespace,
				Labels: map[string]string{
					"loadtest":                 "true",
					wellknown.GatewayNameLabel: s.gateways[gwIdx],
					"app.kubernetes.io/name":   s.gateways[gwIdx],
				},
			},
			Spec: corev1.PodSpec{
				// A node no kubelet owns: the object exists, nothing runs.
				NodeName: s.nodeName((gwIdx*fleetStreamsPerGateway + replica) % nodes),
				Containers: []corev1.Container{{
					Name:  "envoy",
					Image: "registry.k8s.io/pause:3.10",
				}},
			},
		}
		if err := s.createIgnoreExists(pod); err != nil {
			return err
		}
		// The controller only treats a pod as a usable client when it is Ready,
		// and nothing else will ever set that on a pod no kubelet owns.
		pod.Status = corev1.PodStatus{
			Phase:      corev1.PodRunning,
			PodIP:      fmt.Sprintf("10.60.%d.%d", (n/250)%250, n%250+1),
			Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
		}
		return s.testInstallation.ClusterContext.Client.Status().Update(s.ctx, pod)
	})
	s.T().Logf("created %d fake proxy pods across %d nodes in %s", total, nodes, time.Since(start).Round(time.Second))
}

func (s *XdsFleetSuite) createIgnoreExists(obj client.Object) error {
	err := s.testInstallation.ClusterContext.Client.Create(s.ctx, obj)
	if apierrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

func (s *XdsFleetSuite) parallelDo(n int, fn func(int) error) {
	workers := min(fleetCreateWorkers, n)
	if workers < 1 {
		workers = 1
	}
	idx := make(chan int, workers)
	var wg sync.WaitGroup
	var firstErr atomic.Pointer[error]
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range idx {
				if err := fn(i); err != nil {
					e := err
					firstErr.CompareAndSwap(nil, &e)
				}
			}
		}()
	}
	for i := range n {
		idx <- i
	}
	close(idx)
	wg.Wait()
	if e := firstErr.Load(); e != nil {
		s.Require().NoError(*e, "fleet creation failed")
	}
}

// waitForGatewaySnapshots waits until the controller has translated the
// Gateways. Until a role has a snapshot, a client for it gets nothing and
// measures nothing.
func (s *XdsFleetSuite) waitForGatewaySnapshots() {
	deadline := time.Now().Add(fleetWaveTimeout)
	for time.Now().Before(deadline) {
		var gws gwv1.GatewayList
		if err := s.testInstallation.ClusterContext.Client.List(s.ctx, &gws,
			client.InNamespace(s.testNamespace)); err == nil {
			programmed := 0
			for i := range gws.Items {
				for _, c := range gws.Items[i].Status.Conditions {
					if c.Type == string(gwv1.GatewayConditionProgrammed) && c.Status == metav1.ConditionTrue {
						programmed++
						break
					}
				}
			}
			if programmed >= fleetGateways {
				s.T().Logf("all %d Gateways programmed", programmed)
				return
			}
			s.T().Logf("waiting for Gateways to be programmed: %d/%d", programmed, fleetGateways)
		}
		time.Sleep(5 * time.Second)
	}
	s.T().Logf("not all Gateways became programmed within %s; continuing anyway", fleetWaveTimeout)
}

// ---- synthetic xDS clients ----

func (s *XdsFleetSuite) startForwards() {
	mpf, err := s.testInstallation.Actions.Kubectl().StartPortForward(s.ctx,
		portforward.WithDeployment(s.controllerDeployment, s.installNamespace),
		portforward.WithRemotePort(benchMetricsPort))
	s.Require().NoError(err, "should port-forward controller metrics")
	s.metricsPF = mpf
	s.metricsURL = "http://" + mpf.Address() + "/metrics"

	xpf, err := s.testInstallation.Actions.Kubectl().StartPortForward(s.ctx,
		portforward.WithDeployment(s.controllerDeployment, s.installNamespace),
		portforward.WithRemotePort(fleetXdsPort))
	s.Require().NoError(err, "should port-forward controller xDS")
	s.xdsPF = xpf
	s.xdsAddr = xpf.Address()

	sample := s.scrape()
	s.Require().Positive(sample.CPUSeconds, "controller metrics should be reachable at %s", s.metricsURL)
}

// connectGateways opens streams for gateways [from, to).
func (s *XdsFleetSuite) connectGateways(from, to int) {
	for i := from; i < to; i++ {
		role := xds.OwnerNamespaceNameID(wellknown.GatewayApiProxyValue, s.testNamespace, s.gateways[i])
		for r := range fleetStreamsPerGateway {
			c, err := s.openStreamRetrying(role, fmt.Sprintf("%s.%s", s.podName(i, r), s.testNamespace))
			s.Require().NoError(err, "should open xDS stream for %s", role)
			s.clients = append(s.clients, c)
		}
	}
}

// openStreamRetrying retries a stream open, because the port-forward tunnel
// occasionally refuses a new connection under load and a single refusal should
// not end a run that takes an hour to reach this point.
func (s *XdsFleetSuite) openStreamRetrying(role, nodeID string) (*syntheticClient, error) {
	var err error
	for attempt := range 4 {
		var c *syntheticClient
		c, err = s.openStream(role, nodeID)
		if err == nil {
			return c, nil
		}
		// Drop the connection that failed so the next attempt dials a new one.
		if n := len(s.conns); n > 0 {
			_ = s.conns[n-1].Close()
			s.conns = s.conns[:n-1]
		}
		time.Sleep(time.Duration(attempt+1) * 500 * time.Millisecond)
	}
	return nil, err
}

// openStream dials (reusing a connection until it is full) and starts one ADS
// stream that subscribes to the wildcard resources a proxy subscribes to.
func (s *XdsFleetSuite) openStream(role, nodeID string) (*syntheticClient, error) {
	streamsPerConn := max(fleetStreamsPerConn, 1)
	if len(s.conns) == 0 || len(s.clients)%streamsPerConn == 0 {
		conn, err := grpc.NewClient(s.xdsAddr,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(256*1024*1024)))
		if err != nil {
			return nil, err
		}
		s.conns = append(s.conns, conn)
	}
	conn := s.conns[len(s.conns)-1]

	ctx, cancel := context.WithCancel(s.ctx)
	stream, err := discoveryv3.NewAggregatedDiscoveryServiceClient(conn).StreamAggregatedResources(ctx)
	if err != nil {
		cancel()
		return nil, err
	}
	c := &syntheticClient{
		role: role, nodeID: nodeID, stream: stream, cancel: cancel,
		done: make(chan struct{}),
	}
	node := &envoycorev3.Node{
		Id: nodeID,
		Metadata: &structpb.Struct{Fields: map[string]*structpb.Value{
			xds.RoleKey: structpb.NewStringValue(role),
		}},
		UserAgentName: "envoy",
	}
	for _, typeURL := range fleetSubscriptions {
		if err := stream.Send(&discoveryv3.DiscoveryRequest{
			Node: node, TypeUrl: typeURL,
		}); err != nil {
			cancel()
			return nil, err
		}
	}
	go c.pump(node)
	return c, nil
}

// pump acknowledges every response, which is what makes the client look caught
// up to the server and keeps the push loop running.
func (c *syntheticClient) pump(node *envoycorev3.Node) {
	defer close(c.done)
	for {
		resp, err := c.stream.Recv()
		if err != nil {
			if err != io.EOF {
				c.recvErrs.Add(1)
				msg := err.Error()
				c.firstErr.CompareAndSwap(nil, &msg)
			}
			return
		}
		if err := c.stream.Send(&discoveryv3.DiscoveryRequest{
			Node:          node,
			TypeUrl:       resp.GetTypeUrl(),
			VersionInfo:   resp.GetVersionInfo(),
			ResponseNonce: resp.GetNonce(),
		}); err != nil {
			return
		}
		c.acks.Add(1)
	}
}

func (s *XdsFleetSuite) stopClients() {
	for _, c := range s.clients {
		c.cancel()
	}
	for _, c := range s.clients {
		select {
		case <-c.done:
		case <-time.After(2 * time.Second):
		}
	}
	s.clients = nil
}

func (s *XdsFleetSuite) totalAcks() int64 {
	var n int64
	for _, c := range s.clients {
		n += c.acks.Load()
	}
	return n
}

// firstStreamError returns the first receive error any stream saw, which is the
// only useful thing to print when the fleet fails to connect.
func (s *XdsFleetSuite) firstStreamError() string {
	for _, c := range s.clients {
		if e := c.firstErr.Load(); e != nil {
			return *e
		}
	}
	return "none recorded"
}

func (s *XdsFleetSuite) totalRecvErrors() int64 {
	var n int64
	for _, c := range s.clients {
		n += c.recvErrs.Load()
	}
	return n
}

// ---- mutations ----

func (s *XdsFleetSuite) churnEndpointSlice(i int) {
	idx := i % fleetServices
	ip := fmt.Sprintf("10.99.%d.%d", (i/250)%250, i%250+1)
	patch := fmt.Sprintf(`[{"op":"replace","path":"/endpoints/0/addresses/0","value":%q}]`, ip)
	eps := &discoverykv1.EndpointSlice{ObjectMeta: metav1.ObjectMeta{
		Name: fmt.Sprintf("fleet-svc-%d", idx), Namespace: s.testNamespace,
	}}
	s.Require().NoError(s.testInstallation.ClusterContext.Client.Patch(s.ctx, eps,
		client.RawPatch(types.JSONPatchType, []byte(patch))), "should patch EndpointSlice")
}

func (s *XdsFleetSuite) churnInlineBackend(i int) {
	idx := i % fleetInlineBackends
	host := fmt.Sprintf("10.241.%d.%d", (i/250)%250, i%250+1)
	patch := fmt.Sprintf(`{"spec":{"static":{"hosts":[{"host":%q,"port":8080}]}}}`, host)
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(backendGVK)
	obj.SetName(fmt.Sprintf("fleet-inline-%d", idx))
	obj.SetNamespace(s.testNamespace)
	s.Require().NoError(s.testInstallation.ClusterContext.Client.Patch(s.ctx, obj,
		client.RawPatch(types.MergePatchType, []byte(patch))), "should patch inline Backend")
}

// reconnectOneGateway drops and reopens one Gateway's streams, which is what a
// proxy restart looks like to the control plane, without waiting for a pod.
func (s *XdsFleetSuite) reconnectOneGateway(i int) {
	gwIdx := i % fleetGateways
	role := xds.OwnerNamespaceNameID(wellknown.GatewayApiProxyValue, s.testNamespace, s.gateways[gwIdx])
	kept := make([]*syntheticClient, 0, len(s.clients))
	for _, c := range s.clients {
		if c.role == role {
			c.cancel()
			<-c.done
			continue
		}
		kept = append(kept, c)
	}
	s.clients = kept
	for r := range fleetStreamsPerGateway {
		c, err := s.openStream(role, fmt.Sprintf("%s.%s", s.podName(gwIdx, r), s.testNamespace))
		s.Require().NoError(err, "should reopen stream for %s", role)
		s.clients = append(s.clients, c)
	}
}

// ---- controller observation ----

func (s *XdsFleetSuite) scrape() controllerSample {
	sample, err := readControllerSample(s.metricsURL)
	s.Require().NoError(err, "should scrape %s", s.metricsURL)
	return sample
}

// controllerRestarts sums restart counts across controller pods. A restart at
// this scale is almost always the kernel reclaiming an out-of-memory process.
func (s *XdsFleetSuite) controllerRestarts() int32 {
	var pods corev1.PodList
	if err := s.testInstallation.ClusterContext.Client.List(s.ctx, &pods,
		client.InNamespace(s.installNamespace)); err != nil {
		return 0
	}
	var total int32
	for i := range pods.Items {
		if !hasPrefixIn(pods.Items[i].Name, s.controllerDeployment) {
			continue
		}
		for _, cs := range pods.Items[i].Status.ContainerStatuses {
			total += cs.RestartCount
		}
	}
	return total
}

func (s *XdsFleetSuite) waitConverged(before float64) (time.Time, bool) {
	settle := time.Duration(fleetSettleMillis) * time.Millisecond
	deadline := time.Now().Add(fleetIterTimeout)
	last := time.Time{}
	seen := before
	for time.Now().Before(deadline) {
		time.Sleep(250 * time.Millisecond)
		cur := s.scrape().Transforms
		if cur > seen {
			seen = cur
			last = time.Now()
			continue
		}
		if !last.IsZero() && time.Since(last) >= settle {
			return last, true
		}
	}
	if !last.IsZero() {
		return last, true
	}
	return time.Time{}, false
}

func (s *XdsFleetSuite) waitQuiet(quiet, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	last := s.scrape().Transforms
	stableSince := time.Now()
	for time.Now().Before(deadline) {
		time.Sleep(500 * time.Millisecond)
		cur := s.scrape().Transforms
		if cur != last {
			last = cur
			stableSince = time.Now()
			continue
		}
		if time.Since(stableSince) >= quiet {
			return true
		}
	}
	return false
}

func (s *XdsFleetSuite) emit(prefix string, v map[string]any) {
	line := prefix + " " + mustJSON(v)
	s.T().Log(line)
	if s.out != nil {
		if _, err := s.out.WriteString(line + "\n"); err != nil {
			s.T().Logf("failed to write KGW_BENCH_OUT: %v", err)
		}
	}
}

// ---- controller deployment plumbing ----

func (s *XdsFleetSuite) resolveController() (string, string, error) {
	var deployments appsv1.DeploymentList
	if err := s.testInstallation.ClusterContext.Client.List(s.ctx, &deployments,
		client.InNamespace(s.installNamespace)); err != nil {
		return "", "", err
	}
	for _, d := range deployments.Items {
		if !hasPrefixIn(d.GetName(), "kgateway") || len(d.Spec.Template.Spec.Containers) == 0 {
			continue
		}
		container := d.Spec.Template.Spec.Containers[0].Name
		for _, c := range d.Spec.Template.Spec.Containers {
			if hasPrefixIn(c.Name, "kgateway") {
				container = c.Name
				break
			}
		}
		return d.GetName(), container, nil
	}
	return "", "", fmt.Errorf("no kgateway controller deployment in %s", s.installNamespace)
}

func (s *XdsFleetSuite) snapshotEnv(names []string) error {
	var deployment appsv1.Deployment
	if err := s.testInstallation.ClusterContext.Client.Get(s.ctx,
		client.ObjectKey{Namespace: s.installNamespace, Name: s.controllerDeployment}, &deployment); err != nil {
		return err
	}
	var container *corev1.Container
	for i := range deployment.Spec.Template.Spec.Containers {
		if deployment.Spec.Template.Spec.Containers[i].Name == s.controllerContainer {
			container = &deployment.Spec.Template.Spec.Containers[i]
			break
		}
	}
	if container == nil {
		return fmt.Errorf("container %s not found", s.controllerContainer)
	}
	s.originalEnv = make(map[string]*corev1.EnvVar, len(names))
	for _, name := range names {
		s.originalEnv[name] = nil
		for i := range container.Env {
			if container.Env[i].Name == name {
				v := container.Env[i]
				s.originalEnv[name] = &v
				break
			}
		}
	}
	return nil
}

func (s *XdsFleetSuite) restoreEnv() error {
	exprs := make([]string, 0, len(s.originalEnv))
	for name, orig := range s.originalEnv {
		if orig == nil || orig.ValueFrom != nil {
			exprs = append(exprs, name+"-")
			continue
		}
		exprs = append(exprs, name+"="+orig.Value)
	}
	if len(exprs) == 0 {
		return nil
	}
	return s.setEnv(exprs...)
}

// setMemoryLimit sets (or with "0" clears) the controller container's memory
// limit and waits for the new generation to roll out.
func (s *XdsFleetSuite) setMemoryLimit(limit string) error {
	if err := s.testInstallation.Actions.Kubectl().RunCommand(s.ctx,
		"set", "resources", "-n", s.installNamespace,
		"deployment/"+s.controllerDeployment,
		"--containers="+s.controllerContainer,
		"--limits=memory="+limit,
	); err != nil {
		return err
	}
	return s.testInstallation.Actions.Kubectl().DeploymentRolloutStatus(s.ctx,
		s.controllerDeployment, "-n", s.installNamespace, "--timeout=300s")
}

func (s *XdsFleetSuite) setEnv(envExprs ...string) error {
	args := append([]string{
		"set", "env", "-n", s.installNamespace,
		"deployment/" + s.controllerDeployment,
		"--containers=" + s.controllerContainer,
	}, envExprs...)
	if err := s.testInstallation.Actions.Kubectl().RunCommand(s.ctx, args...); err != nil {
		return err
	}
	return s.testInstallation.Actions.Kubectl().DeploymentRolloutStatus(s.ctx,
		s.controllerDeployment, "-n", s.installNamespace, "--timeout=300s")
}

// fleetSubscriptions are the wildcard resource types a proxy subscribes to.
var fleetSubscriptions = []string{
	"type.googleapis.com/" + string((&envoyclusterv3.Cluster{}).ProtoReflect().Descriptor().FullName()),
	"type.googleapis.com/" + string((&envoylistenerv3.Listener{}).ProtoReflect().Descriptor().FullName()),
	"type.googleapis.com/" + string((&envoyendpointv3.ClusterLoadAssignment{}).ProtoReflect().Descriptor().FullName()),
}

var gatewayParametersGVK = backendGVK.GroupVersion().WithKind("GatewayParameters")

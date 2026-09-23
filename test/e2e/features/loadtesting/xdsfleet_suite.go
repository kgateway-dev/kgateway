//go:build e2e

package loadtesting

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoylistenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	discoveryv3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	resourcev3 "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
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
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils/portforward"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	testdefaults "github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
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
	originalResources    *corev1.ResourceRequirements

	testNamespace   string
	gateways        []string
	controllerImage string

	metricsPF  portforward.PortForwarder
	metricsURL string
	xdsPFs     []portforward.PortForwarder
	xdsAddrs   []string

	clients           []*syntheticClient
	conns             []*grpc.ClientConn
	activeConn        *grpc.ClientConn
	activeConnStreams int

	out                     *os.File
	observing               bool
	controllerFailureReason string
	restartBaseline         int32
	survivedGateways        int
	attemptedGateways       int
}

// Fleet shape. The defaults describe the production shape this was built to
// answer: thousands of Kubernetes Services with a couple of endpoints each, a
// handful of inline-CLA backends, and hundreds of Gateways.
var (
	fleetServices       = benchEnvInt("KGW_FLEET_SERVICES", 6000)
	fleetGateways       = benchEnvInt("KGW_FLEET_GATEWAYS", 800)
	fleetInlineBackends = benchEnvInt("KGW_FLEET_INLINE_BACKENDS", 20)
	// fleetEndpointsPerService fixes how many endpoints every Service gets.
	// Zero keeps the default shape, which alternates one and two for an average
	// of 1.5 - the shape a fleet of mostly-small Services has. Set it to model a
	// fleet whose Services are uniformly larger.
	fleetEndpointsPerService = benchEnvInt("KGW_FLEET_ENDPOINTS_PER_SERVICE", 0)
	// fleetFatServices gives the first N Services fleetFatServiceEndpoints
	// endpoints each, on top of whatever the rest get. A fleet's cost is not
	// only its Service count: one Service with hundreds of endpoints produces a
	// single large assignment that every client subscribing to it must be sent,
	// and that is a different kind of load from the same endpoints spread thin.
	fleetFatServices         = benchEnvInt("KGW_FLEET_FAT_SERVICES", 0)
	fleetFatServiceEndpoints = benchEnvInt("KGW_FLEET_FAT_SERVICE_ENDPOINTS", 0)
	// fleetEndpointPods backs every endpoint with a real Pod bound to a fake
	// node, and points the EndpointSlice entry at it.
	//
	// This is what gives an endpoint a locality. The control plane resolves an
	// endpoint's zone by following the slice's targetRef to a Pod and reading
	// its node's topology labels; an endpoint with no targetRef gets the empty
	// locality, so every assignment collapses to a single locality no matter how
	// many zones the fleet has. That makes per-client endpoint prioritisation
	// trivial, which is precisely the work per-client EDS exists to do - so
	// without this the benchmark cannot see the cost it is meant to measure.
	// Costs one Pod per endpoint.
	fleetEndpointPods = benchEnvString("KGW_FLEET_ENDPOINT_PODS", "false") == "true"
	// fleetRoutesPerGateway attaches N HTTPRoutes to every Gateway, each naming
	// fleetBackendsPerRoute of the fleet's Services.
	//
	// Zero - the default - creates none, which is the shape every earlier run
	// used: the control plane emits a cluster for every backend in scope
	// regardless of whether anything routes to it, so routes change nothing it
	// sends. That stops being true under referenced-only cluster discovery,
	// where the emitted set is computed by walking the generated routes and
	// listeners. A fleet with no routes references nothing, so scoping would
	// collapse every proxy's CDS to approximately empty and report a saving
	// that is really just a control plane with nothing to say. Routes are what
	// make that feature measurable rather than trivially "winning".
	fleetRoutesPerGateway = benchEnvInt("KGW_FLEET_ROUTES_PER_GATEWAY", 0)
	// fleetBackendsPerRoute is how many Services each route names. The product
	// with fleetRoutesPerGateway is how many of the fleet's Services a single
	// proxy can reach, and therefore how many clusters scoping should leave it.
	fleetBackendsPerRoute = benchEnvInt("KGW_FLEET_BACKENDS_PER_ROUTE", 1)
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
	// fleetXdsForwards spreads the connections over several port-forward
	// tunnels instead of one. One tunnel is a single process relaying every
	// byte of every stream, and it becomes the limit before the control plane
	// does: past roughly a thousand streams it starts refusing new connections,
	// which reads as a ceiling that belongs to the harness rather than to the
	// thing being measured. Raise it for ladders that go into the thousands.
	fleetXdsForwards = benchEnvInt("KGW_FLEET_XDS_FORWARDS", 1)
	// fleetTrickleMs pauses between individual stream opens, so each client
	// arrives in its own control-plane recomputation instead of in a burst.
	//
	// This is a different workload, not a slower one. Opening streams as fast as
	// the harness can means many clients land in one batch, and anything that
	// shares work or memory across the clients present in a single pass looks
	// good under that arrival pattern whether or not it would help a real fleet.
	// Real proxies connect one at a time - a rolling restart trickles them in over
	// minutes - so a sharing optimization that only works within a batch is worth
	// nothing in production while measuring well here. Set this to tell those two
	// cases apart; a value above the control plane's event-coalescing window is
	// enough, and 250ms is comfortably above it.
	//
	// Costs wall-clock proportional to the client count, so trade it against the
	// fleet size rather than running the full ladder with it.
	fleetTrickleMs     = benchEnvInt("KGW_FLEET_TRICKLE_MS", 0)
	fleetIterations    = benchEnvInt("KGW_FLEET_ITERATIONS", 5)
	fleetCreateWorkers = benchEnvInt("KGW_FLEET_CREATE_WORKERS", 24)
	fleetSettleMillis  = benchEnvInt("KGW_FLEET_SETTLE_MS", 3000)
	fleetWaveTimeout   = time.Duration(benchEnvInt("KGW_FLEET_WAVE_TIMEOUT_SECONDS", 900)) * time.Second
	fleetIterTimeout   = time.Duration(benchEnvInt("KGW_FLEET_ITERATION_TIMEOUT_SECONDS", 600)) * time.Second
	fleetXdsPort       = benchEnvInt("KGW_FLEET_XDS_PORT", 9977)
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
	for kv := range strings.SplitSeq(fleetExtraEnv, ",") {
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
	role             string
	nodeID           string
	localClusterName string
	endpointNames    []string
	stream           discoveryv3.AggregatedDiscoveryService_StreamAggregatedResourcesClient
	cancel           context.CancelFunc
	acks             atomic.Int64
	recvErrs         atomic.Int64
	firstErr         atomic.Pointer[string]
	done             chan struct{}
}

func (s *XdsFleetSuite) SetupSuite() {
	if os.Getenv("KGW_ENABLE_XDS_FLEET") != "true" {
		s.T().Skip("XdsFleet mutates the controller deployment; set KGW_ENABLE_XDS_FLEET=true (or use `make run-xds-fleet-bench`) to run it")
	}
	s.installNamespace = s.testInstallation.Metadata.InstallNamespace
	s.testNamespace = fmt.Sprintf("kgw-fleet-%d", time.Now().UnixNano())

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
		"KGW_VALIDATION_MODE", "DISABLE_POD_LOCALITY_XDS", "KGW_XDS_AUTH", "KGW_XDS_TLS", "KGW_XDS_IDENTITY_INCLUDE_NODE",
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
		"KGW_XDS_TLS=false",
	}
	if fleetIdentityIncludeNode != "" {
		envs = append(envs, "KGW_XDS_IDENTITY_INCLUDE_NODE="+fleetIdentityIncludeNode)
	}
	envs = append(envs, extraEnv...)
	s.Require().NoError(s.setEnv(envs...))

	s.T().Logf("XdsFleet: build=%q validation=%s services=%d gateways=%d inline=%d streams/gw=%d waves=%d trickleMs=%d extraEnv=%v",
		benchLabel, benchValidation, fleetServices, fleetGateways, fleetInlineBackends, fleetStreamsPerGateway, fleetWaves, fleetTrickleMs, extraEnv)

	if fleetMemoryLimit != "" {
		s.T().Logf("capping controller memory at %s for the run", fleetMemoryLimit)
		s.Require().NoError(s.setMemoryLimit(fleetMemoryLimit), "should cap controller memory")
	}

	// Record the image actually running, and refuse to measure without it.
	//
	// Every figure this suite emits describes one control-plane build, and
	// nothing else in the run states which. A runner that deploys by tag can
	// fail to deploy - a failed `helm upgrade` leaves the PREVIOUS image running
	// and exits without touching the fleet - and the benchmark will then happily
	// measure the wrong build under the new build's label. That has happened:
	// four arms of a comparison were collected against a stale image before
	// anyone noticed. Stamping the image into every record makes a mislabeled
	// run detectable afterwards rather than silently publishable.
	img, err := s.readControllerImage()
	s.Require().NoError(err, "should read the controller image being measured")
	s.controllerImage = img
	s.T().Logf("measuring controller image %s", img)

	s.createNamespace()
	s.createZeroReplicaGatewayParameters()
	s.createGateways()
	if fleetPodLocality || fleetEndpointPods {
		s.createFakeNodes()
	}
	if fleetPodLocality {
		s.createFakePods()
	}
	if fleetEndpointPods {
		s.createEndpointPods()
	}
	s.createServicesAndEndpoints()
	s.createInlineBackends()
	s.createRoutes()

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
	for _, pf := range s.xdsPFs {
		pf.Close()
		pf.WaitForStop()
	}
	if s.metricsPF != nil {
		s.metricsPF.Close()
		s.metricsPF.WaitForStop()
	}
	if s.originalEnv != nil {
		if err := s.restoreEnv(); err != nil {
			s.T().Errorf("failed to restore controller env (cluster left modified): %v", err)
		}
	}
	if s.originalResources != nil {
		if err := s.restoreResources(); err != nil {
			s.T().Errorf("failed to restore controller resources (cluster left modified): %v", err)
		}
	}
	if s.out != nil {
		_ = s.out.Close()
	}
	// Fake proxy pods are bound to fake Nodes that no kubelet owns, so nothing
	// will ever confirm their graceful deletion: the namespace then hangs in
	// Terminating on "unexpected items still remain ... Resource=pods" forever,
	// and the leftovers poison the next run. Force them out first.
	if fleetPodLocality || fleetEndpointPods {
		if err := s.testInstallation.Actions.Kubectl().RunCommand(s.ctx,
			"delete", "pods", "-n", s.testNamespace, "--all",
			"--force", "--grace-period=0", "--wait=false",
		); err != nil {
			s.T().Logf("force-deleting fake proxy pods reported: %v", err)
		}
	}
	if fleetPodLocality || fleetEndpointPods {
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
	s.restartBaseline = s.controllerRestarts()
	s.observing = true
	perWave := (fleetGateways + fleetWaves - 1) / fleetWaves
	connected := 0
	var lastAcks int64
	for wave := 1; connected < fleetGateways; wave++ {
		target := min(connected+perWave, fleetGateways)
		start := time.Now()
		newClients := (target - connected) * s.clientsPerGateway()
		firstNewStream := len(s.clients)
		s.attemptedGateways = target
		if err := s.connectGateways(connected, target); err != nil {
			reason := s.controllerFailure()
			if reason == "" {
				reason = fmt.Sprintf("could not open wave streams: %v", err)
			}
			s.emitFailure(reason)
			return
		}
		connected = target
		// Wait for the clients this wave added to actually be served before
		// measuring anything.
		//
		// Settling on the transform counter alone is not enough, and the
		// difference is not a detail: a controller that has fallen behind is
		// quiet in exactly the same way as one that has finished. Measured in
		// that state it reports a fleet it is not serving, and its heap is the
		// heap of a backlog rather than of N steady-state clients. Runs that
		// drifted into that state produced heap figures 2.5x apart from runs of
		// the same binary that did not, which made builds look different when
		// only their luck differed.
		//
		// So the gate is the clients' own acknowledgements: every new client
		// should receive at least one response. Failing to reach that inside the
		// wave timeout is not a measurement to record and move past - it is the
		// point where this build stopped keeping up, so the ladder ends there.
		servedAll := s.waitServed(s.clients[firstNewStream:], fleetWaveTimeout)
		settled := servedAll && s.waitQuiet(time.Duration(fleetSettleMillis)*time.Millisecond, fleetWaveTimeout)
		// Scrapes may fail precisely when the controller reaches its capacity.
		// Preserve the failure verdict before attempting any fatal assertions.
		if reason := s.controllerFailure(); reason != "" {
			s.emitFailure(reason)
			return
		}
		sample, sampleErr := readControllerSample(s.metricsURL)
		restarts := s.controllerRestarts()
		if reason := s.controllerFailure(); reason != "" {
			s.emitFailure(reason)
			return
		}
		if sampleErr != nil {
			s.emitFailure(fmt.Sprintf("controller metrics unavailable: %v", sampleErr))
			return
		}
		// A wave that connects clients and produces no new responses is not a
		// measurement. Those clients are attached and receiving nothing, so the
		// control plane is not building their snapshots, and every figure below
		// describes a fleet that is not being served. Record the delta and say
		// so loudly: read as a heap number, an unserved wave looks like a win.
		acks := s.totalAcks()
		ackDelta := acks - lastAcks
		lastAcks = acks
		if !servedAll {
			s.T().Logf("wave %d: only %d of the %d streams added this wave were served within %s; "+
				"the build stopped keeping up here, so the ladder ends at %d clients",
				wave, servedStreams(s.clients[firstNewStream:]), len(s.clients)-firstNewStream, fleetWaveTimeout, connected*s.clientsPerGateway())
		}
		s.emit("xds_fleet_wave", map[string]any{
			"build": benchLabel, "validation": benchValidation,
			"wave": wave, "gateways": connected,
			// Repeated node/locality identities share a client even when
			// they use separate streams.
			"clients":             connected * s.clientsPerGateway(),
			"streams":             connected * fleetStreamsPerGateway,
			"services":            fleetServices,
			"endpoints":           totalFleetEndpoints(),
			"fat_services":        fleetFatServices,
			"fat_service_eps":     fleetFatServiceEndpoints,
			"endpoint_pods":       fleetEndpointPods,
			"routes_per_gateway":  fleetRoutesPerGateway,
			"backends_per_route":  fleetBackendsPerRoute,
			"trickle_ms":          fleetTrickleMs,
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
			"deferred_clients":    sample.DeferredClients,
			"controller_restarts": restarts,
			"stream_acks":         acks,
			"stream_acks_delta":   ackDelta,
			"served":              servedAll,
			"new_clients":         newClients,
			"stream_errors":       s.totalRecvErrors(),
		})
		s.T().Logf("wave %d: gateways=%d clients=%d settled=%v heap=%.0fMB rss=%.0fMB cpu=%.1fs resources=%.0f restarts=%d",
			wave, connected, connected*s.clientsPerGateway(), settled, sample.HeapInuse/1e6, sample.RSS/1e6, sample.CPUSeconds, sample.Resources, restarts)
		if !servedAll {
			s.emitFailure("could not serve the streams added in this wave within the wave timeout")
			return
		}
		if !settled {
			s.emitFailure("controller did not complete the quiet window within the wave timeout")
			return
		}
		s.survivedGateways = connected
	}

	s.runFleetPhase("EdsChurn", func(i int) { s.churnEndpointSlice(i) })
	s.runFleetPhase("BaseChurn", func(i int) { s.churnInlineBackend(i) })
	s.runFleetPhase("StreamReconnect", func(i int) { s.reconnectOneGateway(i) })
}

// clientsPerGateway is how many unique clients one Gateway's replicas produce.
// With pod locality on, replicas share identities when their nodes (or zones
// when node identity is disabled) repeat; with it off they collapse into one.
func (s *XdsFleetSuite) clientsPerGateway() int {
	if fleetPodLocality {
		if strings.EqualFold(fleetIdentityIncludeNode, "false") {
			return min(fleetStreamsPerGateway, fleetZones)
		}
		return min(fleetStreamsPerGateway, fleetZones*2)
	}
	return 1
}

func (s *XdsFleetSuite) runFleetPhase(name string, mutate func(int)) {
	s.T().Logf("=== fleet phase %s at %d clients", name, fleetGateways*s.clientsPerGateway())
	s.Require().True(s.waitQuiet(time.Duration(fleetSettleMillis)*time.Millisecond, fleetWaveTimeout),
		"controller must go quiet before phase %s", name)
	before := s.scrape()
	startRestarts := s.controllerRestarts()
	start := time.Now()
	var latencies []float64
	timedOut := 0
	for i := range fleetIterations {
		t0 := time.Now()
		tBefore := s.scrape().Transforms
		mutate(i)
		last, ok := s.waitConverged(tBefore)
		if !ok {
			timedOut++
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
		"clients": fleetGateways * s.clientsPerGateway(), "services": fleetServices, "inline_backends": fleetInlineBackends,
		"trickle_ms":            fleetTrickleMs,
		"iterations":            len(latencies),
		"timed_out_iterations":  timedOut,
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
	s.Assert().Zero(timedOut, "phase %s must converge on every iteration", name)
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

// routeBackendsFor picks which Services gateway i routes to. Gateways take
// overlapping but different slices, walking the Service list by a stride, so the
// union across the fleet covers it while no single proxy reaches all of it -
// which is the asymmetry referenced-only discovery exists to exploit. A fleet
// where every gateway reaches every Service would have nothing to scope.
func routeBackendsFor(gateway, route int) []int {
	out := make([]int, 0, fleetBackendsPerRoute)
	for b := range fleetBackendsPerRoute {
		idx := (gateway*fleetRoutesPerGateway*fleetBackendsPerRoute + route*fleetBackendsPerRoute + b) % fleetServices
		out = append(out, idx)
	}
	return out
}

// distinctRoutedServices is how many of the fleet's Services the whole fleet
// routes to, reported so a scoped run can be read against what was reachable.
func distinctRoutedServices() int {
	if fleetRoutesPerGateway == 0 {
		return 0
	}
	seen := map[int]struct{}{}
	for g := range fleetGateways {
		for r := range fleetRoutesPerGateway {
			for _, b := range routeBackendsFor(g, r) {
				seen[b] = struct{}{}
			}
		}
	}
	return len(seen)
}

// createRoutes attaches HTTPRoutes to every Gateway. See fleetRoutesPerGateway
// for why a fleet without them cannot measure referenced-only discovery.
func (s *XdsFleetSuite) createRoutes() {
	if fleetRoutesPerGateway == 0 {
		return
	}
	start := time.Now()
	total := fleetGateways * fleetRoutesPerGateway
	port := gwv1.PortNumber(8080)
	s.parallelDo(total, func(n int) error {
		gwIdx, routeIdx := n/fleetRoutesPerGateway, n%fleetRoutesPerGateway
		refs := make([]gwv1.HTTPBackendRef, 0, fleetBackendsPerRoute)
		for _, b := range routeBackendsFor(gwIdx, routeIdx) {
			refs = append(refs, gwv1.HTTPBackendRef{
				BackendRef: gwv1.BackendRef{
					BackendObjectReference: gwv1.BackendObjectReference{
						Name: gwv1.ObjectName(fmt.Sprintf("fleet-svc-%d", b)),
						Port: &port,
					},
				},
			})
		}
		prefix := gwv1.PathMatchPathPrefix
		path := fmt.Sprintf("/r%d", routeIdx)
		route := &gwv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("fleet-rt-%d-%d", gwIdx, routeIdx),
				Namespace: s.testNamespace,
				Labels:    map[string]string{"loadtest": "true"},
			},
			Spec: gwv1.HTTPRouteSpec{
				CommonRouteSpec: gwv1.CommonRouteSpec{
					ParentRefs: []gwv1.ParentReference{{
						Name: gwv1.ObjectName(s.gateways[gwIdx]),
					}},
				},
				Rules: []gwv1.HTTPRouteRule{{
					Matches: []gwv1.HTTPRouteMatch{{
						Path: &gwv1.HTTPPathMatch{Type: &prefix, Value: &path},
					}},
					BackendRefs: refs,
				}},
			},
		}
		return s.createIgnoreExists(route)
	})
	s.T().Logf("created %d HTTPRoutes (%d per Gateway, %d backends each; %d distinct Services routed) in %s",
		total, fleetRoutesPerGateway, fleetBackendsPerRoute, distinctRoutedServices(),
		time.Since(start).Round(time.Second))
}

// createServicesAndEndpoints builds the Kubernetes Service fleet: EDS clusters
// with a couple of endpoints each, which is what a real cluster is mostly made
// of. Endpoint counts alternate so the average is 1.5.
// endpointsForService returns how many endpoints Service i gets: the fat
// Services first, then whatever the rest of the fleet is configured for.
func endpointsForService(i int) int {
	if i < fleetFatServices && fleetFatServiceEndpoints > 0 {
		return fleetFatServiceEndpoints
	}
	if fleetEndpointsPerService > 0 {
		return fleetEndpointsPerService
	}
	// Default shape: alternate one and two, averaging 1.5.
	if i%2 == 0 {
		return 2
	}
	return 1
}

// fleetEndpointStride is the address block reserved per Service. Addresses have
// to be distinct fleet-wide, because two endpoints sharing one is two Services
// sharing a backend as far as translation is concerned, which would quietly
// change what is being measured. Reserving a fixed block per Service keeps the
// address a pure function of (service, endpoint) so nothing has to be counted.
func fleetEndpointStride() int {
	return max(fleetFatServiceEndpoints, fleetEndpointsPerService, 2)
}

// endpointAddress maps (service, endpoint) to a unique 10.0.0.0/8 address.
func endpointAddress(service, endpoint int) string {
	id := service*fleetEndpointStride() + endpoint
	return fmt.Sprintf("10.%d.%d.%d", 100+id/62500, (id/250)%250, id%250+1)
}

// totalFleetEndpoints is what the fleet's Services add up to, reported so a run
// can be compared against another by endpoint count and not only Service count.
func totalFleetEndpoints() int {
	total := 0
	for i := range fleetServices {
		total += endpointsForService(i)
	}
	return total
}

func endpointPodName(service, endpoint int) string {
	return fmt.Sprintf("fleet-ep-%d-%d", service, endpoint)
}

// createEndpointPods gives every endpoint a Pod on a fake node, which is the
// only way the control plane can assign it a zone. Pods are spread over the
// fake nodes so one Service's endpoints land in several zones, the way a
// spread-scheduled Deployment's would - that is what makes an assignment
// multi-locality and gives per-client prioritisation something to do.
func (s *XdsFleetSuite) createEndpointPods() {
	nodes := fleetZones * 2
	start := time.Now()
	type ref struct{ svc, ep, seq int }
	var refs []ref
	seq := 0
	for i := range fleetServices {
		for e := range endpointsForService(i) {
			refs = append(refs, ref{i, e, seq})
			seq++
		}
	}
	s.parallelDo(len(refs), func(n int) error {
		r := refs[n]
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      endpointPodName(r.svc, r.ep),
				Namespace: s.testNamespace,
				Labels:    map[string]string{"loadtest": "true", "app": fmt.Sprintf("fleet-svc-%d", r.svc)},
			},
			Spec: corev1.PodSpec{
				// Offset by the endpoint index so one Service's endpoints land
				// on different nodes rather than all on the same one.
				NodeName:   s.nodeName((r.svc + r.ep) % nodes),
				Containers: []corev1.Container{{Name: "app", Image: "registry.k8s.io/pause:3.10"}},
			},
		}
		if err := s.createIgnoreExists(pod); err != nil {
			return err
		}
		pod.Status = corev1.PodStatus{
			Phase:      corev1.PodRunning,
			PodIP:      endpointAddress(r.svc, r.ep),
			Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
		}
		return s.testInstallation.ClusterContext.Client.Status().Update(s.ctx, pod)
	})
	s.T().Logf("created %d endpoint pods across %d nodes in %s", len(refs), nodes, time.Since(start).Round(time.Second))
}

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
		count := endpointsForService(i)
		endpoints := make([]discoverykv1.Endpoint, 0, count)
		ready := true
		for e := range count {
			ep := discoverykv1.Endpoint{
				Addresses:  []string{endpointAddress(i, e)},
				Conditions: discoverykv1.EndpointConditions{Ready: &ready},
			}
			if fleetEndpointPods {
				ep.TargetRef = &corev1.ObjectReference{
					Kind: "Pod", Name: endpointPodName(i, e), Namespace: s.testNamespace,
				}
			}
			endpoints = append(endpoints, ep)
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
	workers := max(min(fleetCreateWorkers, n), 1)
	idx := make(chan int, workers)
	var wg sync.WaitGroup
	var firstErr atomic.Pointer[error]
	for range workers {
		wg.Go(func() {
			for i := range idx {
				if err := fn(i); err != nil {
					e := err
					firstErr.CompareAndSwap(nil, &e)
				}
			}
		})
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

	for range max(fleetXdsForwards, 1) {
		xpf, err := s.testInstallation.Actions.Kubectl().StartPortForward(s.ctx,
			portforward.WithDeployment(s.controllerDeployment, s.installNamespace),
			portforward.WithRemotePort(fleetXdsPort))
		s.Require().NoError(err, "should port-forward controller xDS")
		s.xdsPFs = append(s.xdsPFs, xpf)
		s.xdsAddrs = append(s.xdsAddrs, xpf.Address())
	}
	s.T().Logf("opened %d xDS port-forward tunnels", len(s.xdsAddrs))

	sample := s.scrape()
	s.Require().Positive(sample.CPUSeconds, "controller metrics should be reachable at %s", s.metricsURL)
}

// connectGateways opens streams for gateways [from, to), pausing between them
// when fleetTrickleMs is set. See that knob for why the pause changes what is
// being measured rather than just how long it takes.
func (s *XdsFleetSuite) connectGateways(from, to int) error {
	for i := from; i < to; i++ {
		role := xds.OwnerNamespaceNameID(wellknown.GatewayApiProxyValue, s.testNamespace, s.gateways[i])
		for r := range fleetStreamsPerGateway {
			if fleetTrickleMs > 0 && !(i == from && r == 0) {
				time.Sleep(time.Duration(fleetTrickleMs) * time.Millisecond)
			}
			c, err := s.openStreamRetrying(role, fmt.Sprintf("%s.%s", s.podName(i, r), s.testNamespace))
			if err != nil {
				return fmt.Errorf("open xDS stream for %s: %w", role, err)
			}
			s.clients = append(s.clients, c)
		}
	}
	return nil
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
		if reason := s.controllerFailure(); reason != "" {
			return nil, fmt.Errorf("%s", reason)
		}
		// Retain the old connection for its established streams and teardown.
		// Only stop assigning new streams to it.
		s.activeConn = nil
		time.Sleep(time.Duration(attempt+1) * 500 * time.Millisecond)
	}
	return nil, err
}

// openStream dials (reusing a connection until it is full) and starts one ADS
// stream that subscribes to the wildcard resources a proxy subscribes to.
func (s *XdsFleetSuite) openStream(role, nodeID string) (*syntheticClient, error) {
	streamsPerConn := max(fleetStreamsPerConn, 1)
	if s.activeConn == nil || s.activeConnStreams >= streamsPerConn {
		// Spread connections over the tunnels so no single relay carries the
		// whole fleet.
		addr := s.xdsAddrs[len(s.conns)%len(s.xdsAddrs)]
		conn, err := grpc.NewClient(addr,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(256*1024*1024)))
		if err != nil {
			return nil, err
		}
		s.conns = append(s.conns, conn)
		s.activeConn, s.activeConnStreams = conn, 0
	}
	conn := s.activeConn

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
	if fleetPodLocality {
		c.localClusterName, _, _ = (ir.UniquelyConnectedClient{Role: role}).LocalClusterInfo()
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
	s.activeConnStreams++
	return c, nil
}

// pump acknowledges every response, which is what makes the client look caught
// up to the server and keeps the push loop running.
func (c *syntheticClient) pump(node *envoycorev3.Node) {
	defer close(c.done)
	for {
		resp, err := c.stream.Recv()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				c.recvErrs.Add(1)
				msg := err.Error()
				c.firstErr.CompareAndSwap(nil, &msg)
			}
			return
		}
		var resourceNames []string
		if resp.GetTypeUrl() == resourcev3.EndpointType {
			resourceNames = c.endpointNames
		}
		if err := c.stream.Send(&discoveryv3.DiscoveryRequest{
			ResourceNames: resourceNames,
			Node:          node,
			TypeUrl:       resp.GetTypeUrl(),
			VersionInfo:   resp.GetVersionInfo(),
			ResponseNonce: resp.GetNonce(),
		}); err != nil {
			return
		}
		if resp.GetTypeUrl() == resourcev3.ClusterType {
			names, err := fleetEndpointNames(resp, c.localClusterName)
			if err != nil {
				c.recvErrs.Add(1)
				msg := err.Error()
				c.firstErr.Store(&msg)
				return
			}
			c.endpointNames = names
			if err := c.stream.Send(&discoveryv3.DiscoveryRequest{Node: node, TypeUrl: resourcev3.EndpointType, ResourceNames: names}); err != nil {
				return
			}
		}
		c.acks.Add(1)
	}
}

// fleetEndpointNames includes every EDS service advertised by CDS and the
// bootstrap local cluster, preserving the ADS subscription on subsequent ACKs.
func fleetEndpointNames(response *discoveryv3.DiscoveryResponse, localCluster string) ([]string, error) {
	names := []string{}
	if localCluster != "" {
		names = append(names, localCluster)
	}
	for _, resource := range response.GetResources() {
		var cluster envoyclusterv3.Cluster
		if err := resource.UnmarshalTo(&cluster); err != nil {
			return nil, err
		}
		if cluster.GetType() != envoyclusterv3.Cluster_EDS {
			continue
		}
		name := cluster.GetEdsClusterConfig().GetServiceName()
		if name == "" {
			name = cluster.GetName()
		}
		names = append(names, name)
	}
	return names, nil
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
	firstReopened := len(s.clients)
	for r := range fleetStreamsPerGateway {
		c, err := s.openStream(role, fmt.Sprintf("%s.%s", s.podName(gwIdx, r), s.testNamespace))
		s.Require().NoError(err, "should reopen stream for %s", role)
		s.clients = append(s.clients, c)
	}
	s.Require().True(s.waitServed(s.clients[firstReopened:], fleetIterTimeout), "every reopened stream must be served")
}

// ---- controller observation ----

func (s *XdsFleetSuite) scrape() controllerSample {
	sample, err := readControllerSample(s.metricsURL)
	if s.observing {
		reason := s.controllerFailure()
		if reason == "" && err != nil {
			reason = fmt.Sprintf("controller metrics unavailable: %v", err)
		}
		if reason != "" {
			s.emitFailure(reason)
			s.Require().FailNow("fleet measurement aborted", reason)
		}
	}
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
		if s.controllerFailure() != "" {
			return time.Time{}, false
		}
		sample, err := readControllerSample(s.metricsURL)
		if err != nil {
			last = time.Time{}
			continue
		}
		cur := sample.Transforms
		if cur > seen {
			seen = cur
			last = time.Now()
			continue
		}
		if !last.IsZero() && time.Since(last) >= settle {
			return last, true
		}
	}
	return time.Time{}, false
}

// servedStreams counts live streams that have acknowledged at least one response.
// ACKs for other resource types or other streams cannot satisfy this gate.
func servedStreams(clients []*syntheticClient) int {
	served := 0
	for _, c := range clients {
		select {
		case <-c.done:
			continue
		default:
		}
		if c.acks.Load() > 0 {
			served++
		}
	}
	return served
}

// waitServed waits for every stream opened in this wave to receive a response.
func (s *XdsFleetSuite) waitServed(clients []*syntheticClient, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if s.controllerFailure() != "" {
			return false
		}
		if servedStreams(clients) == len(clients) {
			return true
		}
		time.Sleep(500 * time.Millisecond)
	}
	return servedStreams(clients) == len(clients)
}

func (s *XdsFleetSuite) waitQuiet(quiet, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	var last float64
	var stableSince time.Time
	for time.Now().Before(deadline) {
		if s.controllerFailure() != "" {
			return false
		}
		sample, err := readControllerSample(s.metricsURL)
		if err != nil {
			// Missing samples cannot establish a continuous quiet window.
			stableSince = time.Time{}
		} else {
			if stableSince.IsZero() || sample.Transforms != last {
				last = sample.Transforms
				stableSince = time.Now()
			}
			if time.Since(stableSince) >= quiet {
				return true
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return false
}

// controllerFailure is independent of the metrics port-forward, which can die
// with the pod. Ignore restarts that preceded this measurement run.
func (s *XdsFleetSuite) controllerFailure() string {
	if s.observing && s.controllerRestarts() > s.restartBaseline {
		s.controllerFailureReason = "controller restarted (out of memory or crash)"
	}
	return s.controllerFailureReason
}

func (s *XdsFleetSuite) emitFailure(reason string) {
	s.emit("xds_fleet_verdict", map[string]any{
		"build":              benchLabel,
		"survived_clients":   s.survivedGateways * s.clientsPerGateway(),
		"died_at_clients":    s.attemptedGateways * s.clientsPerGateway(),
		"survived_gateways":  s.survivedGateways,
		"died_at_gateways":   s.attemptedGateways,
		"reason":             reason,
		"first_stream_error": s.firstStreamError(),
	})
}

func (s *XdsFleetSuite) emit(prefix string, v map[string]any) {
	if s.controllerImage != "" {
		if _, present := v["controller_image"]; !present {
			v["controller_image"] = s.controllerImage
		}
	}
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
		client.InNamespace(s.installNamespace), client.MatchingLabels{testdefaults.WellKnownAppLabel: controllerAppName()}); err != nil {
		return "", "", err
	}
	for _, d := range deployments.Items {
		if len(d.Spec.Template.Spec.Containers) == 0 {
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
	if fleetMemoryLimit != "" {
		s.originalResources = container.Resources.DeepCopy()
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
	if len(s.originalEnv) == 0 {
		return nil
	}
	if err := updateBenchmarkContainer(s.ctx, s.testInstallation.ClusterContext.Client,
		s.installNamespace, s.controllerDeployment, s.controllerContainer, func(c *corev1.Container) {
			c.Env = restoredBenchmarkEnv(c.Env, s.originalEnv)
		}); err != nil {
		return err
	}
	return s.testInstallation.Actions.Kubectl().DeploymentRolloutStatus(s.ctx,
		s.controllerDeployment, "-n", s.installNamespace, "--timeout=300s")
}

// setMemoryLimit sets (or with "0" clears) the controller container's memory
// limit and waits for the new generation to roll out.
// readControllerImage returns the image of the container this suite measures.
func (s *XdsFleetSuite) readControllerImage() (string, error) {
	var dep appsv1.Deployment
	if err := s.testInstallation.ClusterContext.Client.Get(s.ctx,
		types.NamespacedName{Namespace: s.installNamespace, Name: s.controllerDeployment}, &dep); err != nil {
		return "", err
	}
	for _, c := range dep.Spec.Template.Spec.Containers {
		if c.Name == s.controllerContainer {
			return c.Image, nil
		}
	}
	return "", fmt.Errorf("container %q not found in deployment %q", s.controllerContainer, s.controllerDeployment)
}

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

// fleetSubscriptions are the initial wildcard subscriptions. EDS names are
// derived from CDS responses, plus the bootstrap local cluster.
var fleetSubscriptions = []string{
	"type.googleapis.com/" + string((&envoyclusterv3.Cluster{}).ProtoReflect().Descriptor().FullName()),
	"type.googleapis.com/" + string((&envoylistenerv3.Listener{}).ProtoReflect().Descriptor().FullName()),
	"type.googleapis.com/" + string((&envoyroutev3.RouteConfiguration{}).ProtoReflect().Descriptor().FullName()),
}

var gatewayParametersGVK = backendGVK.GroupVersion().WithKind("GatewayParameters")

// restoreResources replaces the full saved configuration, removing requests
// introduced by Kubernetes defaulting when the benchmark limit was applied.
func (s *XdsFleetSuite) restoreResources() error {
	if err := updateBenchmarkContainer(s.ctx, s.testInstallation.ClusterContext.Client,
		s.installNamespace, s.controllerDeployment, s.controllerContainer, func(c *corev1.Container) {
			c.Resources = *s.originalResources.DeepCopy()
		}); err != nil {
		return err
	}
	return s.testInstallation.Actions.Kubectl().DeploymentRolloutStatus(s.ctx,
		s.controllerDeployment, "-n", s.installNamespace, "--timeout=300s")
}

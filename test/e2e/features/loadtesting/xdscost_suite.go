//go:build e2e

package loadtesting

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stretchr/testify/suite"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils/portforward"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
)

// XdsCost is a repeatable control-plane cost benchmark for the per-client xDS
// pipeline. StrictChurn, its sibling in this package, asserts liveness under
// churn and answers "did the fleet stay served"; this suite answers "what did
// each kind of change cost the controller", which is the question that decides
// between competing per-client CDS topologies (#14602 + #14603 versus #14668).
//
// It measures the controller from the outside, by scraping its own /metrics
// before and after each change. The Go and process collectors are registered
// on the controller's registry (pkg/metrics), so process_cpu_seconds_total,
// go_memstats_alloc_bytes_total and go_memstats_heap_inuse_bytes are real
// measurements of the running binary rather than of a test harness.
//
// Three phases, matching the three events the topologies price differently:
//
//   - BaseChurn edits one static Backend's host. A static Backend translates to
//     a STATIC cluster whose ClusterLoadAssignment is inline, so its endpoints
//     are folded into the cluster's base version: editing the host is a base
//     change, and every connected client's CDS payload must be rebuilt. This is
//     the frequent event wherever endpoints live in the backend object
//     (ServiceEntry with inline endpoints, DNS and static Backends).
//   - EdsChurn rewrites one simulated Service's EndpointSlice. Those are EDS
//     clusters, so their endpoints flow through the separate per-client EDS
//     pipeline and never reach the cluster base. This phase is the control: it
//     should cost about the same under any CDS topology, and it shows how much
//     of an endpoint-churn bill is actually CDS.
//   - Reconnect deletes one gateway's Envoy pod. The replacement is a new xDS
//     client, which is the event the client-keyed topology makes cheap and the
//     sparse-delta topology makes expensive.
//
// Client fan-out is the number of Gateways, not the number of Envoy replicas.
// A UniquelyConnectedClient is keyed by role, namespace, labels and locality,
// so on a single-node cluster every replica of one Gateway collapses into one
// client. Scale KGW_BENCH_GATEWAYS to scale fan-out.
//
// The suite mutates the controller deployment (validation mode) and restores it
// on teardown, so like StrictChurn it is opt-in:
//
//	make run-xds-cost-bench
//
// Every phase emits one `xds_cost_result` JSON line, and the whole run emits an
// `xds_cost_summary` line, so two builds can be compared by diffing output.
// Set KGW_BENCH_OUT to also append those lines to a file.
type XdsCostSuite struct {
	LoadTestingSuite
	loadTestManager       *LoadTestManager
	installNamespace      string
	controllerDeployment  string
	controllerContainer   string
	originalControllerEnv map[string]*corev1.EnvVar

	gateways   []string
	metricsPF  portforward.PortForwarder
	metricsURL string

	results []phaseResult
	out     *os.File
	// churnGen makes every mutation write a value nothing has written before,
	// so no patch is silently a no-op.
	churnGen int
	// idle CPU and allocation rates with the fleet quiet, measured once before
	// the phases so background work can be netted out of per-change costs.
	idleCPUMillisPerSecond float64
	idleAllocMBPerSecond   float64
}

// Scale and pacing knobs. Defaults are sized for a laptop kind cluster: large
// enough that a per-client fan-out is hundreds of clusters per client, small
// enough to finish in a few minutes.
var (
	// benchGateways is the client fan-out. Each Gateway is its own xDS role and
	// therefore its own unique client.
	benchGateways = benchEnvInt("KGW_BENCH_GATEWAYS", 6)
	// benchStaticBackends are inline-CLA backends whose host edits are base
	// changes. Every Backend lands in every client's CDS payload whether or not
	// a route references it, so this is the per-client payload size.
	benchStaticBackends = benchEnvInt("KGW_BENCH_STATIC_BACKENDS", 150)
	// benchEdsRoutes sizes the simulated EDS fleet (fake Services and
	// EndpointSlices) that the EdsChurn phase rewrites.
	benchEdsRoutes = benchEnvInt("KGW_BENCH_EDS_ROUTES", 150)
	// benchIterations is how many changes each phase applies.
	benchIterations = benchEnvInt("KGW_BENCH_ITERATIONS", 12)
	// benchSettleMillis is the quiet window with no new xDS sync that ends an
	// iteration. Convergence latency is measured to the LAST sync, not to the
	// end of this window.
	benchSettleMillis = benchEnvInt("KGW_BENCH_SETTLE_MS", 750)
	// benchIterationTimeout bounds one change's convergence.
	benchIterationTimeout = time.Duration(benchEnvInt("KGW_BENCH_ITERATION_TIMEOUT_SECONDS", 120)) * time.Second
	// benchIdleSeconds is how long the idle baseline runs. The controller does
	// background work (status syncing, watch traffic) that lands in the same
	// counters as the measured changes, so every phase is reported alongside
	// what idling for the same wall time would have cost.
	benchIdleSeconds = benchEnvInt("KGW_BENCH_IDLE_SECONDS", 30)
	// benchMetricsPort is the controller's metrics port (chart default 9092).
	benchMetricsPort = benchEnvInt("KGW_BENCH_METRICS_PORT", 9092)
	// benchValidation selects the controller's validation mode for the run.
	// STANDARD is the product default; STRICT is the opt-in mode that pays an
	// envoy invocation per translated cluster.
	benchValidation = benchEnvString("KGW_BENCH_VALIDATION", "STANDARD")
	// benchLabel names the build under test in the emitted results.
	benchLabel = benchEnvString("KGW_BENCH_LABEL", "unlabeled")
)

// benchEnvInt and benchEnvString read the scale knobs. They are defined here
// rather than reusing StrictChurn's envScale so this file is self-contained and
// can be dropped into a branch that does not carry that suite, which is how two
// builds get measured by identical code.
func benchEnvInt(name string, def int) int {
	if v := os.Getenv(name); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

func benchEnvString(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

// controllerSample is one scrape of the controller's own metrics.
type controllerSample struct {
	At time.Time `json:"at"`
	// CPUSeconds is process_cpu_seconds_total: all threads, user plus system.
	CPUSeconds float64 `json:"cpu_seconds"`
	// AllocBytes is go_memstats_alloc_bytes_total, the cumulative allocation
	// total. Its delta over a phase is GC pressure.
	AllocBytes float64 `json:"alloc_bytes_total"`
	// HeapInuse and RSS are instantaneous.
	HeapInuse  float64 `json:"heap_inuse_bytes"`
	RSS        float64 `json:"rss_bytes"`
	Goroutines float64 `json:"goroutines"`
	// Syncs is kgateway_xds_snapshot_syncs_total summed over gateways. It only
	// advances for resource kinds the metrics layer start-times (Gateways,
	// routes and friends), so it stays flat for Backend and EndpointSlice
	// edits. Reported for completeness; it is NOT the convergence signal.
	Syncs float64 `json:"xds_syncs_total"`
	// Transforms is kgateway_xds_snapshot_transforms_total summed: one per
	// per-client snapshot transform that ran. This is both the fan-out measure
	// and the convergence signal, because it advances for any input that
	// reaches per-client assembly.
	Transforms float64 `json:"xds_transforms_total"`
	// Deferrals is kgateway_xds_snapshot_cluster_deferrals_total summed. It
	// exists only on the sparse-delta build, where a client's whole CDS is
	// withheld until the delta sets catch up; absent means zero.
	Deferrals float64 `json:"xds_cluster_deferrals_total"`
	// Resources is kgateway_xds_snapshot_resources summed over gateways and
	// resource kinds: a sanity check that the fleet is the size we think.
	Resources float64 `json:"xds_resources"`
	// DeferredClients is kgateway_xds_snapshot_deferred_clients summed: the
	// connected clients whose snapshot is currently withheld because their
	// per-client inputs are not ready. It names the condition that otherwise
	// only shows up as a wave whose acknowledgements did not advance. Absent on
	// builds without the gauge, which reads as zero.
	DeferredClients float64 `json:"xds_deferred_clients"`
}

// phaseResult is one phase's measurement, emitted as JSON.
type phaseResult struct {
	Build      string `json:"build"`
	Phase      string `json:"phase"`
	Validation string `json:"validation"`
	Iterations int    `json:"iterations"`
	// Fleet shape.
	Gateways       int `json:"gateways"`
	StaticBackends int `json:"static_backends"`
	EdsRoutes      int `json:"eds_routes"`

	WallSeconds float64 `json:"wall_seconds"`
	// Per-change costs: the phase delta divided by the iteration count.
	CPUMillisPerChange  float64 `json:"cpu_ms_per_change"`
	AllocMBPerChange    float64 `json:"alloc_mb_per_change"`
	SyncsPerChange      float64 `json:"syncs_per_change"`
	TransformsPerChange float64 `json:"transforms_per_change"`
	DeferralsPerChange  float64 `json:"deferrals_per_change"`
	// Convergence latency to the last xDS sync for the change, in milliseconds.
	LatencyMinMillis    float64 `json:"latency_ms_min"`
	LatencyMedianMillis float64 `json:"latency_ms_median"`
	LatencyMaxMillis    float64 `json:"latency_ms_max"`
	// Idle-equivalent cost: what the controller would have spent doing nothing
	// for this phase's wall time, per change. Subtract it from the per-change
	// figures to get the cost attributable to the changes themselves.
	IdleCPUMillisPerChange float64 `json:"idle_cpu_ms_per_change"`
	IdleAllocMBPerChange   float64 `json:"idle_alloc_mb_per_change"`
	// Steady state after the phase.
	HeapInuseMB float64 `json:"heap_inuse_mb_after"`
	RSSMB       float64 `json:"rss_mb_after"`
	Goroutines  float64 `json:"goroutines_after"`
	Resources   float64 `json:"xds_resources_after"`

	Note string `json:"note,omitempty"`
}

var _ e2e.NewSuiteFunc = NewXdsCostSuite

func NewXdsCostSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &XdsCostSuite{
		LoadTestingSuite: LoadTestingSuite{
			Suite:            suite.Suite{},
			ctx:              ctx,
			testInstallation: testInst,
		},
	}
}

var backendGVK = schema.GroupVersionKind{
	Group:   "gateway.kgateway.dev",
	Version: "v1alpha1",
	Kind:    "Backend",
}

func benchGatewayNames(n int) []string {
	names := make([]string, n)
	for i := range names {
		names[i] = fmt.Sprintf("bench-gw-%d", i+1)
	}
	return names
}

func (s *XdsCostSuite) SetupSuite() {
	// Hard opt-in, independent of any -run regex: this suite mutates the
	// controller deployment, so a broad invocation must not pick it up.
	if os.Getenv("KGW_ENABLE_XDS_COST") != "true" {
		s.T().Skip("XdsCost mutates the controller deployment; set KGW_ENABLE_XDS_COST=true (or use `make run-xds-cost-bench`) to run it")
	}

	s.installNamespace = s.testInstallation.Metadata.InstallNamespace
	s.loadTestManager = NewLoadTestManager(s.ctx, s.testInstallation,
		fmt.Sprintf("kgateway-xdscost-%d", time.Now().UnixNano()))
	s.gateways = benchGatewayNames(benchGateways)

	if path := os.Getenv("KGW_BENCH_OUT"); path != "" {
		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		s.Require().NoError(err, "should open KGW_BENCH_OUT")
		s.out = f
	}

	name, container, err := s.resolveController()
	s.Require().NoError(err, "should find the kgateway controller deployment in %s", s.installNamespace)
	s.controllerDeployment, s.controllerContainer = name, container

	// Pin the validation mode for the run so a comparison is not silently
	// measuring two different modes, and restore it on teardown.
	s.Require().NoError(s.snapshotControllerEnv([]string{"KGW_VALIDATION_MODE"}))
	s.T().Logf("XdsCost: build=%q validation=%s gateways=%d staticBackends=%d edsRoutes=%d iterations=%d",
		benchLabel, benchValidation, benchGateways, benchStaticBackends, benchEdsRoutes, benchIterations)
	s.Require().NoError(s.setControllerEnv("KGW_VALIDATION_MODE=" + strings.ToUpper(benchValidation)))

	// Fleet: a simulated EDS fleet, then the gateways, then the inline-CLA
	// Backends whose edits are base changes.
	s.Require().NoError(s.loadTestManager.SetupTestInfrastructure(), "should set up test namespace")
	s.Require().NoError(s.loadTestManager.SetupSimulation(benchEdsRoutes, "xds-cost"), "should set up EDS simulation")
	// The simulator sizes the fleet itself from the route count, so log what it
	// actually built: the EdsChurn phase rotates over that, not over
	// KGW_BENCH_EDS_ROUTES.
	simCfg := s.loadTestManager.simulator.config
	s.T().Logf("XdsCost: EDS simulation %s has %d services (%d nodes x %d per node) in %s",
		simCfg.SimulationName, simCfg.FakeNodeCount*simCfg.ServicesPerNode,
		simCfg.FakeNodeCount, simCfg.ServicesPerNode, simCfg.Namespace)
	s.Require().NoError(s.loadTestManager.CreateGateways(s.gateways), "should create gateways")
	s.Require().NoError(s.loadTestManager.WaitForGatewayReadiness(5*time.Minute), "gateways should become ready")
	s.createStaticBackends(benchStaticBackends)

	// Every gateway's Envoy must be up and streaming before any measurement:
	// an unconnected client is not part of the fan-out.
	for _, gw := range s.gateways {
		s.testInstallation.AssertionsT(s.T()).EventuallyPodsRunning(s.ctx,
			s.loadTestManager.testNamespace,
			metav1.ListOptions{LabelSelector: "gateway.networking.k8s.io/gateway-name=" + gw},
			2*time.Minute)
	}

	s.startMetricsForward()
	// Let the fleet reach steady state, so phase deltas measure the change
	// rather than the tail of setup.
	s.waitQuiet(4*time.Second, 3*time.Minute)
	warm := s.scrape()
	s.T().Logf("XdsCost: warm state resources=%.0f heap=%.1fMB rss=%.1fMB goroutines=%.0f cpu=%.1fs",
		warm.Resources, warm.HeapInuse/1e6, warm.RSS/1e6, warm.Goroutines, warm.CPUSeconds)
	s.Require().Positive(warm.Transforms, "controller should have run per-client transforms before measuring; "+
		"a zero here means the xds_snapshot metric vectors have no label values, i.e. no Gateway is connected")
}

func (s *XdsCostSuite) TearDownSuite() {
	if s.loadTestManager == nil {
		return
	}
	if s.metricsPF != nil {
		s.metricsPF.Close()
		s.metricsPF.WaitForStop()
	}
	if s.originalControllerEnv != nil {
		if err := s.restoreControllerEnv(); err != nil {
			s.T().Logf("failed to restore controller env (cluster left modified): %v", err)
		}
	}
	if s.out != nil {
		_ = s.out.Close()
	}
	// Backends live in the test namespace, which the manager's cleanup removes.
	if err := s.loadTestManager.CleanupAll(); err != nil {
		s.T().Logf("cleanup reported: %v", err)
	}
}

// measureIdle prices the controller doing nothing, so the phases below can be
// read net of background work. It is not a phase: nothing is mutated.
func (s *XdsCostSuite) measureIdle() {
	window := time.Duration(benchIdleSeconds) * time.Second
	s.waitQuiet(2*time.Second, 2*time.Minute)
	before := s.scrape()
	time.Sleep(window)
	after := s.scrape()
	secs := after.At.Sub(before.At).Seconds()
	s.idleCPUMillisPerSecond = (after.CPUSeconds - before.CPUSeconds) * 1000 / secs
	s.idleAllocMBPerSecond = (after.AllocBytes - before.AllocBytes) / 1e6 / secs
	s.emit("xds_cost_idle", map[string]any{
		"build":                benchLabel,
		"validation":           strings.ToUpper(benchValidation),
		"window_seconds":       secs,
		"cpu_ms_per_second":    s.idleCPUMillisPerSecond,
		"alloc_mb_per_second":  s.idleAllocMBPerSecond,
		"transforms_in_window": after.Transforms - before.Transforms,
		"heap_inuse_mb":        after.HeapInuse / 1e6,
		"rss_mb":               after.RSS / 1e6,
		"xds_resources":        after.Resources,
	})
}

// TestXdsCost runs the three phases in order, cheapest side effects first.
func (s *XdsCostSuite) TestXdsCost() {
	s.measureIdle()

	s.runPhase("EdsChurn",
		"EndpointSlice rewrite on an EDS Service: endpoints bypass the cluster base entirely, so this is the control",
		func(i int) { s.churnEndpointSlice(i) })

	s.runPhase("BaseChurn",
		"static Backend host edit: an inline-CLA base change, so every client's CDS payload is rebuilt",
		func(i int) { s.churnStaticBackend(i) })

	s.runPhase("Reconnect",
		"one gateway's Envoy pod deleted: the replacement is a new xDS client",
		func(i int) { s.reconnectGateway(i) })

	s.report()
}

// runPhase applies iterations of mutate, measuring each change's convergence
// latency and the phase's total controller cost.
func (s *XdsCostSuite) runPhase(name, note string, mutate func(int)) {
	s.T().Logf("=== phase %s: %s", name, note)
	// Start from quiet so the first iteration is not measuring the previous
	// phase's tail.
	s.waitQuiet(2*time.Second, 2*time.Minute)

	before := s.scrape()
	start := time.Now()
	latencies := make([]float64, 0, benchIterations)

	for i := range benchIterations {
		transformsBefore := s.scrape().Transforms
		t0 := time.Now()
		mutate(i)
		last, ok := s.waitConverged(transformsBefore, t0)
		if !ok {
			s.T().Logf("phase %s iteration %d did not converge within %s; recording the timeout", name, i, benchIterationTimeout)
			latencies = append(latencies, float64(benchIterationTimeout.Milliseconds()))
			continue
		}
		latencies = append(latencies, float64(last.Sub(t0).Milliseconds()))
	}

	wall := time.Since(start)
	after := s.scrape()
	n := float64(len(latencies))
	if n == 0 {
		n = 1
	}
	sort.Float64s(latencies)

	res := phaseResult{
		Build:                  benchLabel,
		Phase:                  name,
		Validation:             strings.ToUpper(benchValidation),
		Iterations:             benchIterations,
		Gateways:               benchGateways,
		StaticBackends:         benchStaticBackends,
		EdsRoutes:              benchEdsRoutes,
		WallSeconds:            wall.Seconds(),
		CPUMillisPerChange:     (after.CPUSeconds - before.CPUSeconds) * 1000 / n,
		AllocMBPerChange:       (after.AllocBytes - before.AllocBytes) / 1e6 / n,
		SyncsPerChange:         (after.Syncs - before.Syncs) / n,
		TransformsPerChange:    (after.Transforms - before.Transforms) / n,
		DeferralsPerChange:     (after.Deferrals - before.Deferrals) / n,
		IdleCPUMillisPerChange: s.idleCPUMillisPerSecond * wall.Seconds() / n,
		IdleAllocMBPerChange:   s.idleAllocMBPerSecond * wall.Seconds() / n,
		LatencyMinMillis:       percentile(latencies, 0),
		LatencyMedianMillis:    percentile(latencies, 0.5),
		LatencyMaxMillis:       percentile(latencies, 1),
		HeapInuseMB:            after.HeapInuse / 1e6,
		RSSMB:                  after.RSS / 1e6,
		Goroutines:             after.Goroutines,
		Resources:              after.Resources,
		Note:                   note,
	}
	s.results = append(s.results, res)
	s.emit("xds_cost_result", res)
}

// waitConverged polls the controller's per-client transform counter until it has
// advanced past before and then stayed put for the settle window. It returns the
// time of the last observed advance, which is when the fleet stopped rebuilding
// snapshots for this change.
func (s *XdsCostSuite) waitConverged(before float64, t0 time.Time) (time.Time, bool) {
	settle := time.Duration(benchSettleMillis) * time.Millisecond
	deadline := time.Now().Add(benchIterationTimeout)
	last := time.Time{}
	seen := before
	for time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
		cur := s.scrape().Transforms
		if cur > seen {
			seen = cur
			last = time.Now()
			continue
		}
		if !last.IsZero() && time.Since(last) >= settle {
			return last, true
		}
		// Nothing yet: keep waiting for the first push.
		_ = t0
	}
	if !last.IsZero() {
		return last, true
	}
	return time.Time{}, false
}

// waitQuiet blocks until the transform counter has been unchanged for quiet.
func (s *XdsCostSuite) waitQuiet(quiet, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	last := s.scrape().Transforms
	stableSince := time.Now()
	for time.Now().Before(deadline) {
		time.Sleep(250 * time.Millisecond)
		cur := s.scrape().Transforms
		if cur != last {
			last = cur
			stableSince = time.Now()
			continue
		}
		if time.Since(stableSince) >= quiet {
			return
		}
	}
	s.T().Logf("fleet did not go quiet within %s; continuing", timeout)
}

// churnStaticBackend edits one static Backend's host, rotating through the set.
// The host is an address, so the cluster's inline ClusterLoadAssignment changes,
// which changes the base cluster version.
//
// The address comes from a monotonic per-run counter in a different /16 than
// createStaticBackends uses. Deriving it from the iteration index instead would
// hand backend i back the address it was created with, and a patch that writes
// the value already there produces no event at all: the first version of this
// benchmark measured that mistake as a 120 second non-convergence.
func (s *XdsCostSuite) churnStaticBackend(i int) {
	idx := i % benchStaticBackends
	s.churnGen++
	host := fmt.Sprintf("10.245.%d.%d", (s.churnGen/250)%200, s.churnGen%250+1)
	patch := fmt.Sprintf(`{"spec":{"static":{"hosts":[{"host":%q,"port":8080}]}}}`, host)
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(backendGVK)
	obj.SetName(fmt.Sprintf("bench-static-%d", idx))
	obj.SetNamespace(s.loadTestManager.testNamespace)
	s.Require().NoError(s.testInstallation.ClusterContext.Client.Patch(s.ctx, obj,
		client.RawPatch(types.MergePatchType, []byte(patch))), "should patch static Backend %d", idx)
}

// churnEndpointSlice rewrites one simulated Service's EndpointSlice, the same
// mutation StrictChurn's background churner makes.
func (s *XdsCostSuite) churnEndpointSlice(i int) {
	cfg := s.loadTestManager.simulator.config
	total := cfg.FakeNodeCount * cfg.ServicesPerNode
	s.Require().Positive(total, "simulation must have services to churn")
	idx := i % total
	s.churnGen++
	ip := fmt.Sprintf("10.246.%d.%d", (s.churnGen/250)%200, s.churnGen%250+1)
	patch := fmt.Sprintf(`[{"op":"replace","path":"/endpoints/0/addresses/0","value":%q}]`, ip)
	eps := &discoveryv1.EndpointSlice{ObjectMeta: metav1.ObjectMeta{
		Name: fmt.Sprintf("sim-service-%d", idx), Namespace: cfg.Namespace,
	}}
	s.Require().NoError(s.testInstallation.ClusterContext.Client.Patch(s.ctx, eps,
		client.RawPatch(types.JSONPatchType, []byte(patch))), "should patch EndpointSlice %d", idx)
}

// reconnectGateway deletes one gateway's Envoy pods, rotating through gateways.
// The replacement pod opens a fresh xDS stream, which is a new client.
func (s *XdsCostSuite) reconnectGateway(i int) {
	gw := s.gateways[i%len(s.gateways)]
	ns := s.loadTestManager.testNamespace
	s.Require().NoError(s.testInstallation.Actions.Kubectl().RunCommand(s.ctx,
		"delete", "pod", "-n", ns, "-l", "gateway.networking.k8s.io/gateway-name="+gw, "--wait=false"),
		"should delete envoy pod for %s", gw)
	// The pod must be back and Running or the next iteration measures a
	// half-connected fleet.
	s.testInstallation.AssertionsT(s.T()).EventuallyPodsRunning(s.ctx, ns,
		metav1.ListOptions{LabelSelector: "gateway.networking.k8s.io/gateway-name=" + gw}, 3*time.Minute)
}

// createStaticBackends creates n inline-CLA Backends in the test namespace.
func (s *XdsCostSuite) createStaticBackends(n int) {
	for i := range n {
		obj := &unstructured.Unstructured{Object: map[string]any{
			"spec": map[string]any{
				"static": map[string]any{
					"hosts": []any{map[string]any{
						"host": fmt.Sprintf("10.244.0.%d", i%250+1),
						"port": int64(8080),
					}},
				},
			},
		}}
		obj.SetGroupVersionKind(backendGVK)
		obj.SetName(fmt.Sprintf("bench-static-%d", i))
		obj.SetNamespace(s.loadTestManager.testNamespace)
		obj.SetLabels(map[string]string{"loadtest": "true"})
		s.Require().NoError(s.testInstallation.ClusterContext.Client.Create(s.ctx, obj),
			"should create static Backend %d", i)
	}
	s.T().Logf("created %d static Backends in %s", n, s.loadTestManager.testNamespace)
}

// startMetricsForward opens a port-forward to the controller's metrics port and
// leaves it open for the run.
func (s *XdsCostSuite) startMetricsForward() {
	pf, err := s.testInstallation.Actions.Kubectl().StartPortForward(s.ctx,
		portforward.WithDeployment(s.controllerDeployment, s.installNamespace),
		portforward.WithRemotePort(benchMetricsPort),
	)
	s.Require().NoError(err, "should port-forward the controller metrics port")
	s.metricsPF = pf
	s.metricsURL = "http://" + pf.Address() + "/metrics"

	// Fail loudly and early if the endpoint is not what we think it is: a
	// silently empty scrape would turn every measurement into a zero.
	sample := s.scrape()
	s.Require().Positive(sample.CPUSeconds, "controller metrics should expose process_cpu_seconds_total at %s", s.metricsURL)
}

// scrape reads the controller's metrics once and sums the families we track.
func (s *XdsCostSuite) scrape() controllerSample {
	sample, err := readControllerSample(s.metricsURL)
	s.Require().NoError(err, "should scrape %s", s.metricsURL)
	return sample
}

// readControllerSample scrapes one sample from a controller metrics endpoint.
// Shared with the fleet-scale suite in xdsfleet_suite.go.
func readControllerSample(metricsURL string) (controllerSample, error) {
	resp, err := http.Get(metricsURL)
	if err != nil {
		return controllerSample{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return controllerSample{}, err
	}

	sums := sumFamilies(string(body), []string{
		"process_cpu_seconds_total",
		"process_resident_memory_bytes",
		"go_memstats_alloc_bytes_total",
		"go_memstats_heap_inuse_bytes",
		"go_goroutines",
		"kgateway_xds_snapshot_syncs_total",
		"kgateway_xds_snapshot_transforms_total",
		"kgateway_xds_snapshot_cluster_deferrals_total",
		"kgateway_xds_snapshot_resources",
		"kgateway_xds_snapshot_deferred_clients",
	})
	return controllerSample{
		At:              time.Now(),
		CPUSeconds:      sums["process_cpu_seconds_total"],
		AllocBytes:      sums["go_memstats_alloc_bytes_total"],
		HeapInuse:       sums["go_memstats_heap_inuse_bytes"],
		RSS:             sums["process_resident_memory_bytes"],
		Goroutines:      sums["go_goroutines"],
		Syncs:           sums["kgateway_xds_snapshot_syncs_total"],
		Transforms:      sums["kgateway_xds_snapshot_transforms_total"],
		Deferrals:       sums["kgateway_xds_snapshot_cluster_deferrals_total"],
		Resources:       sums["kgateway_xds_snapshot_resources"],
		DeferredClients: sums["kgateway_xds_snapshot_deferred_clients"],
	}, nil
}

// mustJSON marshals a value for one emitted result line.
func mustJSON(v any) string {
	blob, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf(`{"marshal_error":%q}`, err.Error())
	}
	return string(blob)
}

// sortFloats sorts in place; the fleet suite uses it before percentile.
func sortFloats(v []float64) { sort.Float64s(v) }

// hasPrefixIn reports whether name contains sub, used for controller pod and
// container name matching where the release name is not known exactly.
func hasPrefixIn(name, sub string) bool { return strings.Contains(name, sub) }

// sumFamilies sums every series of each requested metric name in a Prometheus
// text exposition. A name with no series sums to zero, which is what we want
// for build-specific metrics like the deferral counter.
func sumFamilies(body string, names []string) map[string]float64 {
	want := make(map[string]bool, len(names))
	out := make(map[string]float64, len(names))
	for _, n := range names {
		want[n] = true
		out[n] = 0
	}
	for _, line := range strings.Split(body, "\n") {
		if line == "" || line[0] == '#' {
			continue
		}
		sp := strings.LastIndexByte(line, ' ')
		if sp <= 0 {
			continue
		}
		name := line[:sp]
		if brace := strings.IndexByte(name, '{'); brace >= 0 {
			name = name[:brace]
		}
		if !want[name] {
			continue
		}
		v, err := strconv.ParseFloat(strings.TrimSpace(line[sp+1:]), 64)
		if err != nil {
			continue
		}
		out[name] += v
	}
	return out
}

func percentile(sorted []float64, q float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(q * float64(len(sorted)-1))
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func (s *XdsCostSuite) emit(prefix string, v any) {
	blob, err := json.Marshal(v)
	s.Require().NoError(err)
	line := prefix + " " + string(blob)
	s.T().Log(line)
	if s.out != nil {
		if _, err := s.out.WriteString(line + "\n"); err != nil {
			s.T().Logf("failed to write KGW_BENCH_OUT: %v", err)
		}
	}
}

func (s *XdsCostSuite) report() {
	s.T().Logf("=== XdsCost summary: build=%s validation=%s gateways=%d staticBackends=%d edsRoutes=%d",
		benchLabel, strings.ToUpper(benchValidation), benchGateways, benchStaticBackends, benchEdsRoutes)
	s.T().Logf("idle baseline: %.1f cpu-ms/s, %.1f alloc-MB/s", s.idleCPUMillisPerSecond, s.idleAllocMBPerSecond)
	s.T().Logf("%-11s %10s %10s %12s %12s %12s %11s %10s",
		"phase", "cpu-ms/ch", "net-cpu", "alloc-MB/ch", "net-alloc", "transf/ch", "median-ms", "heap-MB")
	for _, r := range s.results {
		s.T().Logf("%-11s %10.1f %10.1f %12.1f %12.1f %12.1f %11.0f %10.1f",
			r.Phase, r.CPUMillisPerChange, r.CPUMillisPerChange-r.IdleCPUMillisPerChange,
			r.AllocMBPerChange, r.AllocMBPerChange-r.IdleAllocMBPerChange,
			r.TransformsPerChange, r.LatencyMedianMillis, r.HeapInuseMB)
	}
	s.emit("xds_cost_summary", map[string]any{
		"idle_cpu_ms_per_second":   s.idleCPUMillisPerSecond,
		"idle_alloc_mb_per_second": s.idleAllocMBPerSecond,
		"build":                    benchLabel,
		"validation":               strings.ToUpper(benchValidation),
		"gateways":                 benchGateways,
		"static_backends":          benchStaticBackends,
		"eds_routes":               benchEdsRoutes,
		"iterations":               benchIterations,
		"phases":                   s.results,
	})
}

// resolveController finds the controller deployment and its container name.
func (s *XdsCostSuite) resolveController() (string, string, error) {
	var deployments appsv1.DeploymentList
	if err := s.testInstallation.ClusterContext.Client.List(s.ctx, &deployments,
		client.InNamespace(s.installNamespace)); err != nil {
		return "", "", err
	}
	for _, d := range deployments.Items {
		name := d.GetName()
		if !strings.Contains(name, "kgateway") {
			continue
		}
		if len(d.Spec.Template.Spec.Containers) == 0 {
			continue
		}
		container := d.Spec.Template.Spec.Containers[0].Name
		for _, c := range d.Spec.Template.Spec.Containers {
			if strings.Contains(c.Name, "kgateway") {
				container = c.Name
				break
			}
		}
		return name, container, nil
	}
	return "", "", fmt.Errorf("no kgateway controller deployment found in %s", s.installNamespace)
}

// snapshotControllerEnv records the current values of the named env vars so
// teardown can put the deployment back the way it was.
func (s *XdsCostSuite) snapshotControllerEnv(names []string) error {
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
		return fmt.Errorf("container %s not found on deployment %s", s.controllerContainer, s.controllerDeployment)
	}
	s.originalControllerEnv = make(map[string]*corev1.EnvVar, len(names))
	for _, name := range names {
		s.originalControllerEnv[name] = nil
		for i := range container.Env {
			if container.Env[i].Name == name {
				v := container.Env[i]
				s.originalControllerEnv[name] = &v
				break
			}
		}
	}
	return nil
}

func (s *XdsCostSuite) restoreControllerEnv() error {
	exprs := make([]string, 0, len(s.originalControllerEnv))
	for name, orig := range s.originalControllerEnv {
		if orig == nil || orig.ValueFrom != nil {
			// Unset it: `kubectl set env NAME-` removes the variable.
			exprs = append(exprs, name+"-")
			continue
		}
		exprs = append(exprs, name+"="+orig.Value)
	}
	if len(exprs) == 0 {
		return nil
	}
	sort.Strings(exprs)
	return s.setControllerEnv(exprs...)
}

func (s *XdsCostSuite) setControllerEnv(envExprs ...string) error {
	args := append([]string{
		"set", "env", "-n", s.installNamespace,
		"deployment/" + s.controllerDeployment,
		"--containers=" + s.controllerContainer,
	}, envExprs...)
	if err := s.testInstallation.Actions.Kubectl().RunCommand(s.ctx, args...); err != nil {
		return err
	}
	return s.testInstallation.Actions.Kubectl().DeploymentRolloutStatus(s.ctx,
		s.controllerDeployment, "-n", s.installNamespace, "--timeout=180s")
}

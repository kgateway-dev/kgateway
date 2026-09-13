// Package statusscale measures the control plane's steady-state memory footprint
// and CPU cost at route scale, in-process, against envtest.
//
// It exists to A/B two builds of the control plane (e.g. main vs a branch). The
// harness intentionally depends only on APIs that are stable across branches so
// the exact same file can be dropped into two worktrees and produce comparable
// numbers. See README.md for the procedure.
package statusscale_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	istiokube "istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/apiclient"
	"github.com/kgateway-dev/kgateway/v2/test/envtestassets"
	"github.com/kgateway-dev/kgateway/v2/test/envtestutil"
)

const (
	// controllerName is the controller that owns the status entries we count.
	controllerName = "kgateway.dev/kgateway"
	// fixtureNamespace holds every Gateway, HTTPRoute and Service we create.
	fixtureNamespace = "gwtest"
	// missingBackend is a Service name that intentionally does not exist, used to
	// force a real status transition during the status-churn phase.
	missingBackend = "svc-does-not-exist"
)

// bootstrapYAML is applied before the control plane starts. selfManaged keeps the
// deployer out of the measurement: no Deployments, Services or ConfigMaps are
// created per Gateway.
const bootstrapYAML = `kind: GatewayClass
apiVersion: gateway.networking.k8s.io/v1
metadata:
  name: kgateway
spec:
  controllerName: kgateway.dev/kgateway
  parametersRef:
    group: gateway.kgateway.dev
    kind: GatewayParameters
    name: kgateway
    namespace: default
---
kind: GatewayParameters
apiVersion: gateway.kgateway.dev/v1alpha1
metadata:
  name: kgateway
spec:
  selfManaged: {}
---
kind: Namespace
apiVersion: v1
metadata:
  name: gwtest
---
# The install namespace exists so the bootstrap controller can create its OAuth2
# HMAC secret once instead of retrying on a backoff loop for the whole run.
kind: Namespace
apiVersion: v1
metadata:
  name: kgateway-system
`

type config struct {
	Label        string        `json:"label"`
	Commit       string        `json:"commit"`
	Gateways     int           `json:"gateways"`
	Routes       int           `json:"routes"`
	Services     int           `json:"services"`
	ChurnRounds  int           `json:"churnRounds"`
	ChurnRoutes  int           `json:"churnRoutes"`
	WriteLatency time.Duration `json:"writeLatency"`

	quiet    time.Duration
	timeout  time.Duration
	outDir   string
	parallel int
	// Idle detection: how many consecutive post-GC live-heap readings must agree,
	// how closely, and how long to wait between them.
	stableSamples   int
	stableTolerance float64
	stableInterval  time.Duration
}

func loadConfig(t *testing.T) config {
	c := config{
		Label:        envOr("SCALEPERF_LABEL", "unlabeled"),
		Gateways:     envInt(t, "SCALEPERF_GATEWAYS", 10),
		Routes:       envInt(t, "SCALEPERF_ROUTES", 1000),
		Services:     envInt(t, "SCALEPERF_SERVICES", 20),
		ChurnRounds:  envInt(t, "SCALEPERF_CHURN_ROUNDS", 3),
		ChurnRoutes:  envInt(t, "SCALEPERF_CHURN_ROUTES", 100),
		WriteLatency: envDuration(t, "SCALEPERF_WRITE_LATENCY", 0),
		quiet:        envDuration(t, "SCALEPERF_QUIET", 3*time.Second),
		timeout:      envDuration(t, "SCALEPERF_TIMEOUT", 5*time.Minute),
		outDir:       envOr("SCALEPERF_OUT", ""),
		parallel:     envInt(t, "SCALEPERF_PARALLEL", 8),

		stableSamples:   envInt(t, "SCALEPERF_STABLE_SAMPLES", 4),
		stableTolerance: float64(envInt(t, "SCALEPERF_STABLE_TOLERANCE_PCT", 3)) / 100,
		stableInterval:  envDuration(t, "SCALEPERF_STABLE_INTERVAL", 5*time.Second),
	}
	if c.ChurnRoutes > c.Routes {
		c.ChurnRoutes = c.Routes
	}
	if c.outDir == "" {
		c.outDir = t.TempDir()
	}
	if err := os.MkdirAll(c.outDir, 0o755); err != nil {
		t.Fatalf("failed to create out dir %s: %v", c.outDir, err)
	}
	c.Commit = gitCommit()
	return c
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(t *testing.T, key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		t.Fatalf("invalid %s=%q: %v", key, v, err)
	}
	return n
}

func envDuration(t *testing.T, key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		t.Fatalf("invalid %s=%q: %v", key, v, err)
	}
	return d
}

func gitCommit() string {
	out, err := exec.Command("git", "rev-parse", "--short", "HEAD").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

type memSnapshot struct {
	HeapAllocBytes  uint64 `json:"heapAllocBytes"`
	HeapInuseBytes  uint64 `json:"heapInuseBytes"`
	HeapObjects     uint64 `json:"heapObjects"`
	StackInuseBytes uint64 `json:"stackInuseBytes"`
	SysBytes        uint64 `json:"sysBytes"`
	NumGC           uint32 `json:"numGC"`
	Goroutines      int    `json:"goroutines"`
}

type phase struct {
	WallSeconds float64 `json:"wallSeconds"`
	CPUSeconds  float64 `json:"cpuSeconds"`
	// AllocBytes and Mallocs are cumulative allocation during the phase, not live
	// heap. They are the clearest signal of GC pressure and of recomputation that
	// allocates and is then thrown away.
	AllocBytes    uint64 `json:"allocBytes"`
	Mallocs       uint64 `json:"mallocs"`
	RouteStatuses int    `json:"routeStatusWrites"`
	GwStatuses    int    `json:"gatewayStatusWrites"`
	SpecWrites    int    `json:"specWrites"`
}

type result struct {
	Config     config      `json:"config"`
	GOMAXPROCS int         `json:"gomaxprocs"`
	GOGC       string      `json:"gogc"`
	GOOS       string      `json:"goos"`
	GOARCH     string      `json:"goarch"`
	Baseline   memSnapshot `json:"baselineHeap"`
	Steady     memSnapshot `json:"steadyHeap"`

	Create phase `json:"create"`
	Settle phase `json:"settle"`
	// Idle covers the work that continues after status writes stop: each write is an
	// informer event that can re-trigger translation, and that translation produces
	// no further writes, so a quiet watch does not mean the control plane is done.
	// Costs here are real but invisible to a watch-only convergence test.
	Idle         phase       `json:"idle"`
	ChurnNeutral phase       `json:"churnNeutral"`
	ChurnStatus  phase       `json:"churnStatus"`
	Load         loadMetrics `json:"load"`

	MaxRSSBytes uint64 `json:"maxRSSBytes"`
	// TotalAllocBytes is every byte allocated over the whole run.
	TotalAllocBytes uint64 `json:"totalAllocBytes"`
	HeapProfile     string `json:"heapProfile"`
	ResultFile      string `json:"resultFile"`
}

type loadMetrics struct {
	ConvergenceWallSeconds float64 `json:"convergenceWallSeconds"`
	WriteAttempts          int     `json:"writeAttempts"`
	SuccessfulWrites       int     `json:"successfulWrites"`
	Conflicts              int     `json:"conflicts"`
	WriteActiveSeconds     float64 `json:"writeActiveSeconds"`
	WriteQPS               float64 `json:"writeQPS"`
	RouteWritesPerRoute    float64 `json:"routeWritesPerRoute"`
	StalenessSamples       int     `json:"stalenessSamples"`
	P95StalenessSeconds    float64 `json:"p95StalenessSeconds"`
	MaxStalenessSeconds    float64 `json:"maxStalenessSeconds"`
}

type statusWriteEvent struct {
	start      time.Time
	finish     time.Time
	statusCode int
}

// statusWriteProbe injects latency immediately before Gateway API status requests
// reach the API server and records their outcomes. Installing it on the shared REST
// config covers every typed KRT writer without changing production code.
type statusWriteProbe struct {
	latency time.Duration
	mu      sync.Mutex
	events  []statusWriteEvent
}

func newStatusWriteProbe(latency time.Duration) *statusWriteProbe {
	return &statusWriteProbe{latency: latency}
}

func (p *statusWriteProbe) wrapTransport(previous func(http.RoundTripper) http.RoundTripper) func(http.RoundTripper) http.RoundTripper {
	return func(base http.RoundTripper) http.RoundTripper {
		if previous != nil {
			base = previous(base)
		}
		return roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			if !isGatewayAPIStatusWrite(req) {
				return base.RoundTrip(req)
			}

			started := time.Now()
			if p.latency > 0 {
				timer := time.NewTimer(p.latency)
				select {
				case <-req.Context().Done():
					if !timer.Stop() {
						<-timer.C
					}
					p.record(statusWriteEvent{start: started, finish: time.Now()})
					return nil, req.Context().Err()
				case <-timer.C:
				}
			}

			resp, err := base.RoundTrip(req)
			statusCode := 0
			if resp != nil {
				statusCode = resp.StatusCode
			}
			p.record(statusWriteEvent{start: started, finish: time.Now(), statusCode: statusCode})
			return resp, err
		})
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func isGatewayAPIStatusWrite(req *http.Request) bool {
	if req.Method != http.MethodPut && req.Method != http.MethodPatch {
		return false
	}
	path := req.URL.Path
	if !strings.Contains(path, "/apis/gateway.networking.k8s.io/") || !strings.HasSuffix(path, "/status") {
		return false
	}
	return strings.Contains(path, "/httproutes/") || strings.Contains(path, "/gateways/")
}

func (p *statusWriteProbe) record(event statusWriteEvent) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, event)
}

func (p *statusWriteProbe) snapshot() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.events)
}

func (p *statusWriteProbe) loadMetricsSince(start int, convergence time.Duration, staleness []time.Duration) loadMetrics {
	p.mu.Lock()
	events := slices.Clone(p.events[start:])
	p.mu.Unlock()

	m := loadMetrics{
		ConvergenceWallSeconds: convergence.Seconds(),
		WriteAttempts:          len(events),
		StalenessSamples:       len(staleness),
	}
	if len(events) > 0 {
		first, last := events[0].start, events[0].finish
		for _, event := range events {
			if event.start.Before(first) {
				first = event.start
			}
			if event.finish.After(last) {
				last = event.finish
			}
			if event.statusCode >= http.StatusOK && event.statusCode < http.StatusMultipleChoices {
				m.SuccessfulWrites++
			}
			if event.statusCode == http.StatusConflict {
				m.Conflicts++
			}
		}
		m.WriteActiveSeconds = last.Sub(first).Seconds()
		if m.WriteActiveSeconds > 0 {
			m.WriteQPS = float64(m.SuccessfulWrites) / m.WriteActiveSeconds
		}
	}
	if len(staleness) > 0 {
		slices.Sort(staleness)
		p95Index := (95*len(staleness)+99)/100 - 1
		m.P95StalenessSeconds = staleness[p95Index].Seconds()
		m.MaxStalenessSeconds = staleness[len(staleness)-1].Seconds()
	}
	return m
}

func TestScaleFootprint(t *testing.T) {
	if os.Getenv("SCALEPERF") == "" {
		t.Skip("set SCALEPERF=1 to run the control plane scale footprint measurement")
	}
	cfg := loadConfig(t)
	t.Logf("scale footprint: label=%s commit=%s gateways=%d routes=%d services=%d",
		cfg.Label, cfg.Commit, cfg.Gateways, cfg.Routes, cfg.Services)

	// The default 512KiB sampling rate is too coarse to attribute a few MiB of live
	// heap. Lower it for attribution runs only: sampling every allocation costs CPU,
	// so profiles taken this way pair with CPU numbers from a default-rate run.
	if rate := envInt(t, "SCALEPERF_MEMPROFILERATE", 0); rate > 0 {
		runtime.MemProfileRate = rate
		t.Logf("MemProfileRate=%d: heap attribution is finer, CPU numbers are not comparable to default-rate runs", rate)
	}

	st, err := envtestutil.BuildSettings()
	if err != nil {
		t.Fatalf("failed to build settings: %v", err)
	}
	// Log I/O is real CPU and its volume differs between builds; keep it out of the
	// measurement unless explicitly asked for.
	if os.Getenv("KGW_LOG_LEVEL") == "" {
		st.LogLevel = "error"
	}

	assetsDir, err := envtestassets.GetEnvTestAssetsDir()
	if err != nil {
		t.Fatalf("failed to get envtest assets dir: %v", err)
	}
	root := filepath.Join("..", "..", "..")
	testEnv := &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join(root, "pkg", "kgateway", "crds"),
			filepath.Join(root, "install", "helm", "kgateway-crds", "templates"),
		},
		ErrorIfCRDPathMissing:   true,
		BinaryAssetsDirectory:   assetsDir,
		ControlPlaneStopTimeout: time.Millisecond,
	}

	bootstrap := filepath.Join(t.TempDir(), "bootstrap.yaml")
	if err := os.WriteFile(bootstrap, []byte(bootstrapYAML), 0o600); err != nil {
		t.Fatalf("failed to write bootstrap yaml: %v", err)
	}

	writeProbe := newStatusWriteProbe(cfg.WriteLatency)
	envtestutil.RunController(
		t,
		st,
		testEnv,
		nil,
		[][]string{{"default", bootstrap}},
		nil,
		func(t *testing.T, ctx context.Context, _ *krt.DebugHandler, client istiokube.CLIClient, _ int) {
			measure(t, ctx, client, cfg, writeProbe)
		},
		func(cfg *rest.Config) (apiclient.Client, error) {
			cfg.WrapTransport = writeProbe.wrapTransport(cfg.WrapTransport)
			return apiclient.New(cfg)
		},
	)
}

func measure(t *testing.T, ctx context.Context, client istiokube.CLIClient, cfg config, writeProbe *statusWriteProbe) {
	res := result{
		Config:     cfg,
		GOMAXPROCS: runtime.GOMAXPROCS(0),
		GOGC:       envOr("GOGC", "default(100)"),
		GOOS:       runtime.GOOS,
		GOARCH:     runtime.GOARCH,
	}

	w := newWatcher(t, ctx, client)
	defer w.stop()

	// Let the control plane finish its initial sync before taking the baseline, so
	// the baseline reflects an idle-but-started process rather than a starting one.
	w.waitQuiet(t, cfg.quiet, cfg.timeout, "initial control plane sync")
	// Both heap snapshots are taken after two collections so they measure live data
	// and are comparable to each other.
	runtime.GC()
	runtime.GC()
	res.Baseline = snapshotHeap()
	t.Logf("baseline live heap: %s", humanBytes(res.Baseline.HeapAllocBytes))

	// Phase 1: create the fixture. CPU here is dominated by our own client work,
	// so it is reported but is the weakest of the CPU signals.
	loadStart := time.Now()
	writeStart := writeProbe.snapshot()
	createStart := w.mark()
	createFixture(t, ctx, client, cfg, w)
	res.Create = w.since(createStart)

	// Phase 2: settle. From "all objects exist" to "all routes carry our status and
	// nothing else is being written". This is control plane work almost exclusively.
	settleStart := w.mark()
	w.waitFor(t, func(s snapshot) bool { return s.routesWithOurStatus >= cfg.Routes },
		cfg.quiet, cfg.timeout, fmt.Sprintf("%d routes to reach status", cfg.Routes))
	res.Settle = w.since(settleStart)
	res.Load = writeProbe.loadMetricsSince(writeStart, time.Since(loadStart), w.staleness())
	res.Load.RouteWritesPerRoute = float64(res.Create.RouteStatuses+res.Settle.RouteStatuses) / float64(cfg.Routes)
	if res.Load.StalenessSamples != cfg.Routes {
		t.Fatalf("staleness tracking saw %d/%d first route statuses", res.Load.StalenessSamples, cfg.Routes)
	}

	// Then wait for the process to actually go idle. Measuring the heap at the end of
	// the settle phase above reports whatever translation happened to be in flight,
	// which is window-dependent and not a steady state.
	idleStart := w.mark()
	stopIdleCPUProfile := startIdleCPUProfile(t, cfg)
	waitHeapStable(t, cfg)
	stopIdleCPUProfile()
	res.Idle = w.since(idleStart)

	runtime.GC()
	runtime.GC()
	res.Steady = snapshotHeap()
	t.Logf("steady live heap: %s (%+d objects vs baseline)",
		humanBytes(res.Steady.HeapAllocBytes),
		int64(res.Steady.HeapObjects)-int64(res.Baseline.HeapObjects))

	res.HeapProfile = filepath.Join(cfg.outDir, fmt.Sprintf("heap-%s.pb.gz", cfg.Label))
	writeHeapProfile(t, res.HeapProfile)

	// Phase 3: hostname churn. Patching a route's hostnames re-runs translation
	// without changing what the status says, but observedGeneration in the parent
	// conditions still has to advance, so one write per patch is correct. What this
	// phase measures is the cost of that re-translation, and any writes beyond one
	// per patch — including writes to Gateways, whose status should not move at all.
	if cfg.ChurnRounds > 0 && cfg.ChurnRoutes > 0 {
		res.ChurnNeutral = churn(t, ctx, client, cfg, w, "neutral", neutralPatch, nil)
		// Phase 4: status-changing churn. Flipping the backendRef to a missing
		// Service and back forces a genuine ResolvedRefs transition each round, so
		// this measures the cost of writes that actually have to happen.
		res.ChurnStatus = churn(t, ctx, client, cfg, w, "status", statusPatch, statusSettled(cfg))
	}

	res.MaxRSSBytes = maxRSSBytes()
	var final runtime.MemStats
	runtime.ReadMemStats(&final)
	res.TotalAllocBytes = final.TotalAlloc
	res.ResultFile = filepath.Join(cfg.outDir, fmt.Sprintf("result-%s.json", cfg.Label))
	writeResult(t, res)
	report(t, res)
}

// createFixture creates Services, Gateways and HTTPRoutes. Routes are spread
// round-robin over gateways and services.
func createFixture(t *testing.T, ctx context.Context, client istiokube.CLIClient, cfg config, w *watcher) {
	kube := client.Kube()
	gwapi := client.GatewayAPI()

	for i := range cfg.Services {
		svc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: serviceName(i), Namespace: fixtureNamespace},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{{Name: "http", Port: 8080}},
				// No selector: nothing in envtest would populate EndpointSlices anyway,
				// and backend resolution for status only needs the Service to exist.
			},
		}
		if _, err := kube.CoreV1().Services(fixtureNamespace).Create(ctx, svc, metav1.CreateOptions{}); err != nil &&
			!apierrors.IsAlreadyExists(err) {
			t.Fatalf("failed to create service %s: %v", svc.Name, err)
		}
	}

	for i := range cfg.Gateways {
		gw := &gwv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: gatewayName(i), Namespace: fixtureNamespace},
			Spec: gwv1.GatewaySpec{
				GatewayClassName: "kgateway",
				Listeners: []gwv1.Listener{{
					Name:     "http",
					Protocol: gwv1.HTTPProtocolType,
					Port:     8080,
					AllowedRoutes: &gwv1.AllowedRoutes{
						Namespaces: &gwv1.RouteNamespaces{From: ptr(gwv1.NamespacesFromAll)},
					},
				}},
			},
		}
		if _, err := gwapi.GatewayV1().Gateways(fixtureNamespace).Create(ctx, gw, metav1.CreateOptions{}); err != nil &&
			!apierrors.IsAlreadyExists(err) {
			t.Fatalf("failed to create gateway %s: %v", gw.Name, err)
		}
	}

	forEachConcurrent(cfg.parallel, cfg.Routes, func(i int) {
		route := &gwv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{Name: routeName(i), Namespace: fixtureNamespace},
			Spec: gwv1.HTTPRouteSpec{
				CommonRouteSpec: gwv1.CommonRouteSpec{
					ParentRefs: []gwv1.ParentReference{{
						Name: gwv1.ObjectName(gatewayName(i % cfg.Gateways)),
					}},
				},
				Hostnames: []gwv1.Hostname{gwv1.Hostname(fmt.Sprintf("r%d.example.com", i))},
				Rules: []gwv1.HTTPRouteRule{{
					Matches: []gwv1.HTTPRouteMatch{{
						Path: &gwv1.HTTPPathMatch{
							Type:  ptr(gwv1.PathMatchPathPrefix),
							Value: ptr(fmt.Sprintf("/r%d", i)),
						},
					}},
					BackendRefs: []gwv1.HTTPBackendRef{{
						BackendRef: gwv1.BackendRef{
							BackendObjectReference: gwv1.BackendObjectReference{
								Name: gwv1.ObjectName(serviceName(i % cfg.Services)),
								Port: ptr(gwv1.PortNumber(8080)),
							},
						},
					}},
				}},
			},
		}
		w.expectRouteStatus(route.Name, time.Now())
		if _, err := gwapi.GatewayV1().HTTPRoutes(fixtureNamespace).Create(ctx, route, metav1.CreateOptions{}); err != nil &&
			!apierrors.IsAlreadyExists(err) {
			w.cancelRouteStatus(route.Name)
			t.Fatalf("failed to create route %s: %v", route.Name, err)
		}
	})
	t.Logf("created %d services, %d gateways, %d routes", cfg.Services, cfg.Gateways, cfg.Routes)
}

// patchFunc builds the merge patch applied to route i on churn round r.
type patchFunc func(cfg config, round, i int) string

// neutralPatch changes a hostname: translation re-runs and only observedGeneration
// changes in the resulting status.
func neutralPatch(_ config, round, i int) string {
	return fmt.Sprintf(`{"spec":{"hostnames":["r%d-c%d.example.com"]}}`, i, round)
}

// statusPatch alternates the backendRef between a real and a missing Service,
// which flips the ResolvedRefs condition on the route's parent status.
func statusPatch(cfg config, round, i int) string {
	name := serviceName(i % cfg.Services)
	if round%2 == 0 {
		name = missingBackend
	}
	return fmt.Sprintf(
		`{"spec":{"rules":[{"matches":[{"path":{"type":"PathPrefix","value":"/r%d"}}],"backendRefs":[{"name":%q,"port":8080}]}]}}`,
		i, name)
}

// statusSettled returns a predicate that waits for every churned route to report
// the ResolvedRefs value implied by the round's patch.
func statusSettled(cfg config) func(round int) func(snapshot) bool {
	return func(round int) func(snapshot) bool {
		want := "True"
		if round%2 == 0 {
			want = "False"
		}
		return func(s snapshot) bool {
			for i := range cfg.ChurnRoutes {
				if s.resolvedRefs[routeName(i)] != want {
					return false
				}
			}
			return true
		}
	}
}

func churn(
	t *testing.T,
	ctx context.Context,
	client istiokube.CLIClient,
	cfg config,
	w *watcher,
	kind string,
	patch patchFunc,
	settled func(round int) func(snapshot) bool,
) phase {
	routes := client.GatewayAPI().GatewayV1().HTTPRoutes(fixtureNamespace)
	start := w.mark()
	for round := range cfg.ChurnRounds {
		forEachConcurrent(cfg.parallel, cfg.ChurnRoutes, func(i int) {
			body := []byte(patch(cfg, round, i))
			if _, err := routes.Patch(ctx, routeName(i), types.MergePatchType, body, metav1.PatchOptions{}); err != nil {
				t.Fatalf("failed to patch route %s: %v", routeName(i), err)
			}
		})
		if settled != nil {
			w.waitFor(t, settled(round), cfg.quiet, cfg.timeout,
				fmt.Sprintf("%s churn round %d to settle", kind, round))
		} else {
			w.waitQuiet(t, cfg.quiet, cfg.timeout, fmt.Sprintf("%s churn round %d to go quiet", kind, round))
		}
	}
	p := w.since(start)
	t.Logf("%s churn: %d rounds x %d routes -> %d route status writes, %d gateway status writes, %.2fs cpu",
		kind, cfg.ChurnRounds, cfg.ChurnRoutes, p.RouteStatuses, p.GwStatuses, p.CPUSeconds)
	return p
}

// routeState is the last observed state of one HTTPRoute.
type routeState struct {
	generation   int64
	hasOurStatus bool
	resolvedRefs string
}

// snapshot is a consistent view of the watcher's counters.
type snapshot struct {
	routeStatusWrites   int
	gwStatusWrites      int
	specWrites          int
	routesWithOurStatus int
	resolvedRefs        map[string]string
	lastEvent           time.Time
}

// watcher counts status-only writes performed by the control plane. A MODIFIED
// event whose metadata.generation is unchanged from the previously observed
// version is a status (or metadata) write; a generation bump is one of our own
// spec writes.
type watcher struct {
	mu              sync.Mutex
	routes          map[string]routeState
	gateways        map[string]int64
	routeStatus     int
	gwStatus        int
	specWrites      int
	lastEvent       time.Time
	statusExpected  map[string]time.Time
	statusStaleness []time.Duration
	cancel          context.CancelFunc
	wg              sync.WaitGroup
}

func newWatcher(t *testing.T, ctx context.Context, client istiokube.CLIClient) *watcher {
	ctx, cancel := context.WithCancel(ctx)
	w := &watcher{
		routes:         map[string]routeState{},
		gateways:       map[string]int64{},
		statusExpected: map[string]time.Time{},
		lastEvent:      time.Now(),
		cancel:         cancel,
	}

	w.wg.Go(func() {
		watchLoop(ctx, t, "httproute", func(opts metav1.ListOptions) (watch.Interface, error) {
			return client.GatewayAPI().GatewayV1().HTTPRoutes(fixtureNamespace).Watch(ctx, opts)
		}, w.onRoute)
	})
	w.wg.Go(func() {
		watchLoop(ctx, t, "gateway", func(opts metav1.ListOptions) (watch.Interface, error) {
			return client.GatewayAPI().GatewayV1().Gateways(fixtureNamespace).Watch(ctx, opts)
		}, w.onGateway)
	})
	return w
}

func (w *watcher) stop() {
	w.cancel()
	w.wg.Wait()
}

func (w *watcher) expectRouteStatus(name string, since time.Time) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.statusExpected[name] = since
}

func (w *watcher) cancelRouteStatus(name string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.statusExpected, name)
}

func (w *watcher) staleness() []time.Duration {
	w.mu.Lock()
	defer w.mu.Unlock()
	return slices.Clone(w.statusStaleness)
}

// watchLoop keeps a watch open, re-establishing it if the server closes it.
func watchLoop(
	ctx context.Context,
	t *testing.T,
	name string,
	open func(metav1.ListOptions) (watch.Interface, error),
	handle func(watch.Event),
) {
	for ctx.Err() == nil {
		iface, err := open(metav1.ListOptions{})
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			t.Logf("watch %s: open failed, retrying: %v", name, err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(200 * time.Millisecond):
			}
			continue
		}
		for ev := range iface.ResultChan() {
			handle(ev)
		}
		iface.Stop()
	}
}

func (w *watcher) onRoute(ev watch.Event) {
	route, ok := ev.Object.(*gwv1.HTTPRoute)
	if !ok {
		return
	}
	hasOurs, resolved := ourStatus(route)

	w.mu.Lock()
	defer w.mu.Unlock()
	w.lastEvent = time.Now()

	prev, seen := w.routes[route.Name]
	switch {
	case ev.Type == watch.Deleted:
		delete(w.routes, route.Name)
		return
	case !seen:
		// first sighting: creation, not a write we attribute to either side
	case route.Generation != prev.generation:
		w.specWrites++
	default:
		w.routeStatus++
	}
	if hasOurs {
		if started, expected := w.statusExpected[route.Name]; expected {
			w.statusStaleness = append(w.statusStaleness, time.Since(started))
			delete(w.statusExpected, route.Name)
		}
	}
	w.routes[route.Name] = routeState{
		generation:   route.Generation,
		hasOurStatus: hasOurs,
		resolvedRefs: resolved,
	}
}

func (w *watcher) onGateway(ev watch.Event) {
	gw, ok := ev.Object.(*gwv1.Gateway)
	if !ok {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.lastEvent = time.Now()

	prev, seen := w.gateways[gw.Name]
	switch {
	case ev.Type == watch.Deleted:
		delete(w.gateways, gw.Name)
		return
	case !seen:
	case gw.Generation != prev:
		w.specWrites++
	default:
		w.gwStatus++
	}
	w.gateways[gw.Name] = gw.Generation
}

// ourStatus reports whether the route carries a parent status entry from our
// controller, and the value of that entry's ResolvedRefs condition.
func ourStatus(route *gwv1.HTTPRoute) (bool, string) {
	for _, p := range route.Status.Parents {
		if string(p.ControllerName) != controllerName {
			continue
		}
		resolved := ""
		for _, c := range p.Conditions {
			if c.Type == string(gwv1.RouteConditionResolvedRefs) {
				resolved = string(c.Status)
			}
		}
		return true, resolved
	}
	return false, ""
}

func (w *watcher) snapshot() snapshot {
	w.mu.Lock()
	defer w.mu.Unlock()
	s := snapshot{
		routeStatusWrites: w.routeStatus,
		gwStatusWrites:    w.gwStatus,
		specWrites:        w.specWrites,
		resolvedRefs:      make(map[string]string, len(w.routes)),
		lastEvent:         w.lastEvent,
	}
	for name, st := range w.routes {
		if st.hasOurStatus {
			s.routesWithOurStatus++
		}
		s.resolvedRefs[name] = st.resolvedRefs
	}
	return s
}

// marker records the counters and clocks at the start of a phase.
type marker struct {
	wall       time.Time
	cpu        float64
	totalAlloc uint64
	mallocs    uint64
	snap       snapshot
}

func (w *watcher) mark() marker {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return marker{
		wall:       time.Now(),
		cpu:        cpuSeconds(),
		totalAlloc: ms.TotalAlloc,
		mallocs:    ms.Mallocs,
		snap:       w.snapshot(),
	}
}

func (w *watcher) since(m marker) phase {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	now := w.snapshot()
	return phase{
		WallSeconds:   time.Since(m.wall).Seconds(),
		CPUSeconds:    cpuSeconds() - m.cpu,
		AllocBytes:    ms.TotalAlloc - m.totalAlloc,
		Mallocs:       ms.Mallocs - m.mallocs,
		RouteStatuses: now.routeStatusWrites - m.snap.routeStatusWrites,
		GwStatuses:    now.gwStatusWrites - m.snap.gwStatusWrites,
		SpecWrites:    now.specWrites - m.snap.specWrites,
	}
}

// waitFor blocks until pred holds and no watch event has been seen for quiet.
func (w *watcher) waitFor(t *testing.T, pred func(snapshot) bool, quiet, timeout time.Duration, what string) {
	deadline := time.Now().Add(timeout)
	for {
		s := w.snapshot()
		if pred(s) && time.Since(s.lastEvent) >= quiet {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out after %s waiting for %s (routes with status: %d, route status writes: %d)",
				timeout, what, s.routesWithOurStatus, s.routeStatusWrites)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// waitQuiet blocks until no watch event has been seen for quiet.
func (w *watcher) waitQuiet(t *testing.T, quiet, timeout time.Duration, what string) {
	w.waitFor(t, func(snapshot) bool { return true }, quiet, timeout, what)
}

// waitHeapStable blocks until live heap stops moving, which is the only reliable
// signal that the control plane has finished reacting. It forces a collection and
// samples HeapAlloc until several consecutive readings agree within a tolerance.
//
// A quiet watch is not sufficient: status writes stop while the translations those
// writes triggered are still running, and that work produces no watch events. With a
// short window the measured heap is dominated by whatever is in flight, so the same
// binary reports wildly different numbers depending on how long the harness waits.
func waitHeapStable(t *testing.T, cfg config) {
	samples := make([]uint64, 0, cfg.stableSamples)
	deadline := time.Now().Add(cfg.timeout)
	for {
		runtime.GC()
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		samples = append(samples, ms.HeapAlloc)
		if len(samples) > cfg.stableSamples {
			samples = samples[1:]
		}
		if len(samples) == cfg.stableSamples {
			lo, hi := samples[0], samples[0]
			for _, s := range samples {
				lo = min(lo, s)
				hi = max(hi, s)
			}
			if lo > 0 && float64(hi-lo)/float64(lo) <= cfg.stableTolerance {
				t.Logf("heap stable after %d samples: %s (spread %.1f%%)",
					cfg.stableSamples, humanBytes(hi), float64(hi-lo)/float64(lo)*100)
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out after %s waiting for live heap to stabilize (last samples: %v)", cfg.timeout, samples)
		}
		time.Sleep(cfg.stableInterval)
	}
}

func snapshotHeap() memSnapshot {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return memSnapshot{
		HeapAllocBytes:  ms.HeapAlloc,
		HeapInuseBytes:  ms.HeapInuse,
		HeapObjects:     ms.HeapObjects,
		StackInuseBytes: ms.StackInuse,
		SysBytes:        ms.Sys,
		NumGC:           ms.NumGC,
		Goroutines:      runtime.NumGoroutine(),
	}
}

func cpuSeconds() float64 {
	var ru syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err != nil {
		return 0
	}
	tv := func(t syscall.Timeval) float64 {
		return float64(t.Sec) + float64(t.Usec)/1e6
	}
	return tv(ru.Utime) + tv(ru.Stime)
}

// maxRSSBytes returns peak resident set size. ru_maxrss is bytes on darwin and
// kilobytes on linux.
func maxRSSBytes() uint64 {
	var ru syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err != nil {
		return 0
	}
	if runtime.GOOS == "darwin" {
		return uint64(ru.Maxrss)
	}
	return uint64(ru.Maxrss) * 1024
}

// startIdleCPUProfile profiles only the post-write idle phase, which is where most
// of the CPU goes at high route counts and which the whole-run heap profile cannot
// separate from load. Off by default: CPU profiling costs a few percent, so a run
// with it enabled is for attribution, not for comparison against runs without it.
func startIdleCPUProfile(t *testing.T, cfg config) func() {
	if os.Getenv("SCALEPERF_IDLE_CPUPROFILE") == "" {
		return func() {}
	}
	path := filepath.Join(cfg.outDir, fmt.Sprintf("cpuidle-%s.pprof", cfg.Label))
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("failed to create idle cpu profile %s: %v", path, err)
	}
	if err := pprof.StartCPUProfile(f); err != nil {
		f.Close()
		t.Fatalf("failed to start idle cpu profile: %v", err)
	}
	return func() {
		pprof.StopCPUProfile()
		f.Close()
		t.Logf("idle cpu profile: %s", path)
	}
}

func writeHeapProfile(t *testing.T, path string) {
	// FreeOSMemory forces a GC and returns free pages, so the profile reflects
	// live data rather than whatever the last GC cycle happened to leave behind.
	debug.FreeOSMemory()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("failed to create heap profile %s: %v", path, err)
	}
	defer f.Close()
	if err := pprof.Lookup("heap").WriteTo(f, 0); err != nil {
		t.Fatalf("failed to write heap profile: %v", err)
	}
	t.Logf("heap profile: %s", path)
}

func writeResult(t *testing.T, res result) {
	b, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal result: %v", err)
	}
	if err := os.WriteFile(res.ResultFile, b, 0o600); err != nil {
		t.Fatalf("failed to write result: %v", err)
	}
	compact, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("failed to marshal compact result: %v", err)
	}
	// A single grep-able line so a driver script can collect runs without parsing
	// the whole log.
	fmt.Printf("SCALEPERF_JSON %s\n", compact)
}

func report(t *testing.T, res result) {
	t.Logf("=== scale footprint (%s @ %s) ===", res.Config.Label, res.Config.Commit)
	t.Logf("  gateways=%d routes=%d services=%d gomaxprocs=%d",
		res.Config.Gateways, res.Config.Routes, res.Config.Services, res.GOMAXPROCS)
	t.Logf("  live heap:     baseline %s -> steady %s (delta %s)",
		humanBytes(res.Baseline.HeapAllocBytes), humanBytes(res.Steady.HeapAllocBytes),
		humanBytes(int64(res.Steady.HeapAllocBytes)-int64(res.Baseline.HeapAllocBytes)))
	t.Logf("  heap inuse:    %s (secondary allocator-span metric)", humanBytes(res.Steady.HeapInuseBytes))
	t.Logf("  heap objects:  %d", res.Steady.HeapObjects)
	t.Logf("  goroutines:    %d", res.Steady.Goroutines)
	t.Logf("  peak rss:      %s", humanBytes(res.MaxRSSBytes))
	t.Logf("  total alloc:   %s", humanBytes(res.TotalAllocBytes))
	t.Logf("  create:        %.2fs wall %.2fs cpu %s alloc",
		res.Create.WallSeconds, res.Create.CPUSeconds, humanBytes(res.Create.AllocBytes))
	t.Logf("  settle:        %.2fs wall incl quiet %.2fs cpu %s alloc (%d route status writes, %d gw status writes)",
		res.Settle.WallSeconds, res.Settle.CPUSeconds, humanBytes(res.Settle.AllocBytes),
		res.Settle.RouteStatuses, res.Settle.GwStatuses)
	t.Logf("  idle wait:     %.2fs wall incl stability wait %.2fs cpu %s alloc (work still running after writes stopped)",
		res.Idle.WallSeconds, res.Idle.CPUSeconds, humanBytes(res.Idle.AllocBytes))
	t.Logf("  load total:    %.2fs cpu %s alloc (create + settle; wall omitted)",
		res.Create.CPUSeconds+res.Settle.CPUSeconds,
		humanBytes(res.Create.AllocBytes+res.Settle.AllocBytes))
	t.Logf("  status load:   %.2fs wall, %.2f route writes/route, %.1f writes/s, %d conflicts",
		res.Load.ConvergenceWallSeconds, res.Load.RouteWritesPerRoute, res.Load.WriteQPS, res.Load.Conflicts)
	t.Logf("  staleness:     p95 %.2fs, max %.2fs (%d watch-observed first statuses)",
		res.Load.P95StalenessSeconds, res.Load.MaxStalenessSeconds, res.Load.StalenessSamples)
	t.Logf("  convergence:   %.2fs cpu %s alloc (create + settle + post-write idle)",
		res.Create.CPUSeconds+res.Settle.CPUSeconds+res.Idle.CPUSeconds,
		humanBytes(res.Create.AllocBytes+res.Settle.AllocBytes+res.Idle.AllocBytes))
	t.Logf("  churn neutral: %.2fs wall incl quiet %.2fs cpu %s alloc (%d route status writes, %d gw)",
		res.ChurnNeutral.WallSeconds, res.ChurnNeutral.CPUSeconds, humanBytes(res.ChurnNeutral.AllocBytes),
		res.ChurnNeutral.RouteStatuses, res.ChurnNeutral.GwStatuses)
	t.Logf("  churn status:  %.2fs wall incl quiet %.2fs cpu %s alloc (%d route status writes, %d gw)",
		res.ChurnStatus.WallSeconds, res.ChurnStatus.CPUSeconds, humanBytes(res.ChurnStatus.AllocBytes),
		res.ChurnStatus.RouteStatuses, res.ChurnStatus.GwStatuses)
	t.Logf("  result: %s", res.ResultFile)
}

func humanBytes[T int64 | uint64](n T) string {
	f := float64(n)
	neg := ""
	if f < 0 {
		neg, f = "-", -f
	}
	switch {
	case f >= 1<<30:
		return fmt.Sprintf("%s%.2fGiB", neg, f/(1<<30))
	case f >= 1<<20:
		return fmt.Sprintf("%s%.1fMiB", neg, f/(1<<20))
	case f >= 1<<10:
		return fmt.Sprintf("%s%.1fKiB", neg, f/(1<<10))
	default:
		return fmt.Sprintf("%s%.0fB", neg, f)
	}
}

func serviceName(i int) string { return fmt.Sprintf("svc-%d", i) }
func gatewayName(i int) string { return fmt.Sprintf("gw-%d", i) }
func routeName(i int) string   { return fmt.Sprintf("route-%05d", i) }

func ptr[T any](v T) *T { return &v }

func forEachConcurrent(parallel, n int, fn func(i int)) {
	if parallel < 1 {
		parallel = 1
	}
	sem := make(chan struct{}, parallel)
	var wg sync.WaitGroup
	for i := range n {
		sem <- struct{}{}
		wg.Go(func() {
			defer func() { <-sem }()
			fn(i)
		})
	}
	wg.Wait()
}

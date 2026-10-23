package proxy_syncer

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"runtime/metrics"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"text/template"
	"time"

	"github.com/Masterminds/sprig/v3"
	envoybootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	"istio.io/istio/pkg/kube/krt"
	istiolog "istio.io/istio/pkg/log"
	"istio.io/istio/pkg/slices"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/pkg/apiclient"
	apifake "github.com/kgateway-dev/kgateway/v2/pkg/apiclient/fake"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/extensions2/registry"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/proxy_syncer/sharedproto"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/xds"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	"github.com/kgateway-dev/kgateway/v2/pkg/schemes"
)

// BenchmarkSyncer benchmarks the full proxy syncer KRT graph (common collections, plugins,
// gateway translation, per-client snapshots and status contributions) against scenarios built
// from pilot-load style templates in testdata/bench.
//
// Ported from agentgateway's BenchmarkSyncer
// (https://github.com/agentgateway/agentgateway/pull/4017). The harness and scenarios are the
// same, except that:
//   - kgateway builds its collections from an apiclient, so objects are served by the fake client
//     and refreshed through its typed clientsets rather than through mutable collections.
//   - Apps add the EndpointSlice kgateway reads endpoints from.
//   - Translation debouncing is off; see newBenchSyncer.
//   - ListenerSet hostnames include the namespace, so every app's ListenerSet is accepted rather
//     than conflicting with the same app name in other namespaces.
//
// A scenario is a list of configs; each renders a template Replicas times in each of its
// Namespaces, naming instances <Name>-<index>. Once the scenario is synced, every config whose
// template is marked `#refresh=true` gets its own benchmark. Each iteration refreshes one
// instance, the way a pilot-load refresh does (.Rand flips and .RandNumber changes), then waits
// for the controller to go idle.
//
// Templates get .Name, .Namespace, .Index (unique across namespaces, e.g. for IPs), .Rand,
// .RandNumber, the config's Vars, and the sprig functions.
//
// Reported metrics:
//   - ns/op, B/op, allocs/op: the full cost of one refresh, until every collection is idle.
//   - xds-changes/op: per-client xDS snapshots changed per refresh. 0 means the refresh did no
//     real work.
//   - retained-bytes: heap held by the synced scenario, inputs included.
//
// Example:
//
//	go test ./pkg/kgateway/proxy_syncer/ -run '^$' -bench 'BenchmarkSyncer/httproutes/refresh=httproute' -benchmem
func BenchmarkSyncer(b *testing.B) {
	gateways, gatewayConfigs := gatewayConfigs()

	// Each app has a Service, a Pod, an EndpointSlice, and an HTTPRoute attached directly to
	// every gateway.
	b.Run("httproutes", func(b *testing.B) {
		const namespaces = 50
		const replicas = 100
		runScenario(b, append(slices.Clone(gatewayConfigs),
			benchConfig{Template: "app", Namespace: "mesh", Namespaces: namespaces, Replicas: replicas},
			benchConfig{
				Template: "httproute", Name: "app", Namespace: "mesh", Namespaces: namespaces, Replicas: replicas,
				Vars: map[string]any{"gateways": gateways},
			},
		))
	})

	// Each app has a Service, a Pod, an EndpointSlice, its own ListenerSet on every gateway, and
	// many HTTPRoutes attached to those ListenerSets. A TLS certificate is shared per namespace.
	b.Run("listenersets", func(b *testing.B) {
		const namespaces = 10
		const replicas = 100
		runScenario(b, append(slices.Clone(gatewayConfigs),
			benchConfig{Template: "tls-secret", Namespace: "mesh", Namespaces: namespaces, Vars: map[string]any{"Name": "ns-cert"}},
			benchConfig{Template: "app", Namespace: "mesh", Namespaces: namespaces, Replicas: replicas},
			benchConfig{
				Template: "route", Name: "app", Namespace: "mesh", Namespaces: namespaces, Replicas: replicas,
				Vars: map[string]any{"gateways": gateways, "routes": 16},
			},
			benchConfig{
				Template: "listenerset", Name: "app", Namespace: "mesh", Namespaces: namespaces, Replicas: replicas,
				Vars: map[string]any{"gateways": gateways},
			},
		))
	})
}

// gatewayConfigs returns the node pods run on, our Gateway plus 5 Gateways owned by another
// controller, and the "<namespace>/<name>" of each for routes to attach to.
func gatewayConfigs() ([]string, []benchConfig) {
	gateways := []string{"gateways/kgateway-0"}
	for i := range 5 {
		gateways = append(gateways, fmt.Sprintf("gateways/other-%d", i))
	}
	return gateways, []benchConfig{
		{Template: "node"},
		{Template: "gatewayclass", Vars: map[string]any{
			"className":  wellknown.DefaultGatewayClassName,
			"controller": wellknown.DefaultGatewayControllerName,
		}},
		{Template: "gatewayclass", Name: "other-gatewayclass", Vars: map[string]any{"className": "other", "controller": "example.com/other"}},
		{Template: "gateway", Name: "kgateway", Namespace: "gateways", Vars: map[string]any{"className": wellknown.DefaultGatewayClassName}},
		{Template: "tls-secret", Name: "kgateway", Namespace: "gateways"},
		{Template: "gateway", Name: "other", Namespace: "gateways", Replicas: 5, Vars: map[string]any{"className": "other"}},
	}
}

type benchConfig struct {
	// Template is the file name in testdata/bench, without extension.
	Template string
	// Name prefixes instance names (<Name>-<index>). Defaults to Template. Configs sharing a Name
	// render matching names, e.g. an HTTPRoute pointing at the Service of the same app.
	Name string
	// Namespace is set on rendered objects that do not set one. Leave empty for cluster scoped objects.
	Namespace string
	// Namespaces, if set, spreads instances over that many namespaces named <Namespace>-<n>.
	Namespaces int
	// Replicas is the number of instances per namespace. Defaults to 1.
	Replicas int
	// Vars are extra template inputs. A "Name" var replaces the generated instance name.
	Vars map[string]any
}

func runScenario(b *testing.B, configs []benchConfig) {
	templates := loadBenchTemplates(b)
	perTemplate := map[string]int{}
	for _, cfg := range configs {
		perTemplate[cfg.Template]++
	}
	for i, cfg := range configs {
		// Name benchmarks after the template, adding the config name when a template is used twice.
		refresh := cfg.Template
		if perTemplate[cfg.Template] > 1 {
			refresh += "-" + cfg.name()
		}
		if strings.Contains(templates[cfg.Template].Root.String(), "#refresh=true") {
			b.Run("refresh="+refresh, func(b *testing.B) {
				benchRefresh(b, templates, configs, i)
			})
		}
	}
}

func benchRefresh(b *testing.B, templates map[string]*template.Template, configs []benchConfig, refreshed int) {
	logging.Reset(slog.LevelWarn)
	for _, scope := range istiolog.Scopes() {
		scope.SetOutputLevel(istiolog.WarnLevel)
	}
	// TestMain arms the shared-proto mutation tripwire for this package. It re-hashes every
	// shared proto on each snapshot build, which production does not do.
	prevAssert := sharedproto.AssertImmutability
	sharedproto.AssertImmutability = false
	b.Cleanup(func() { sharedproto.AssertImmutability = prevAssert })
	ctx, cancel := context.WithCancel(context.Background())
	stop := ctx.Done()
	before := liveHeapBytes()

	namespaces := map[string]bool{}
	var initial [][][]renderedObject // [config][instance][object]
	var objs []client.Object
	for _, cfg := range configs {
		var instances [][]renderedObject
		for _, inst := range cfg.instances() {
			rendered := renderInstance(b, templates[cfg.Template], cfg, inst, false)
			instances = append(instances, rendered)
			for _, o := range rendered {
				objs = append(objs, o.obj)
			}
			if inst.namespace != "" {
				namespaces[inst.namespace] = true
			}
		}
		initial = append(initial, instances)
	}
	for ns := range namespaces {
		objs = append(objs, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}})
	}

	cli := apifake.NewClient(b, objs...)
	// Informers must be stopped before Shutdown, which waits for them to exit.
	b.Cleanup(func() {
		cancel()
		cli.Shutdown()
	})
	s := newBenchSyncer(b, ctx, cli)
	cli.RunAndWait(stop)
	cli.WaitForCacheSync("bench", stop, s.waitForSync...)
	waitIdle()
	retained := liveHeapBytes() - before

	// Pre-render the refreshed state of every instance, so the loop does no rendering or decoding.
	// Only objects that actually differ are re-applied, like a server side apply would.
	cfg := configs[refreshed]
	type refresh struct{ apply, revert []client.Object }
	var refreshes []refresh
	for i, inst := range cfg.instances() {
		base := initial[refreshed][i]
		var r refresh
		for j, o := range renderInstance(b, templates[cfg.Template], cfg, inst, true) {
			if o.yaml != base[j].yaml {
				r.apply = append(r.apply, o.obj)
				r.revert = append(r.revert, base[j].obj)
			}
		}
		if len(r.apply) == 0 {
			b.Fatalf("refreshing %s renders identical objects; the template must use .Rand or .RandNumber", cfg.Template)
		}
		refreshes = append(refreshes, r)
	}

	var xdsChanges atomic.Int64
	s.perclientSnapCollection.RegisterBatch(func(events []krt.Event[XdsSnapWrapper]) {
		xdsChanges.Add(int64(len(events)))
	}, false)

	// Refresh instances round-robin. Odd passes revert to the initial state, so every refresh is a
	// real change.
	var resourceVersion int
	for i := 0; b.Loop(); i++ {
		r := refreshes[i%len(refreshes)]
		objs := r.apply
		if (i/len(refreshes))%2 == 1 {
			objs = r.revert
		}
		// The fake clientset rebuilds a REST mapper on every update (about 2 MB), so leave the
		// writes out of the measurement. Each object's event is emitted as its update returns, so
		// little of the work it triggers happens while the timer is stopped.
		b.StopTimer()
		for _, o := range objs {
			// The fake tracker never bumps resourceVersion; stamp one so updates look real.
			resourceVersion++
			o.SetResourceVersion(strconv.Itoa(resourceVersion))
			if err := update(ctx, cli, o); err != nil {
				b.Fatalf("update %T %s/%s: %v", o, o.GetNamespace(), o.GetName(), err)
			}
		}
		b.StartTimer()
		waitIdle()
	}
	b.ReportMetric(float64(xdsChanges.Load())/float64(b.N), "xds-changes/op")
	b.ReportMetric(retained, "retained-bytes")
}

// newBenchSyncer wires the proxy syncer the way the controller does, with one connected Envoy
// for our Gateway so per-client snapshots are built.
func newBenchSyncer(b *testing.B, ctx context.Context, cli apiclient.Client) *ProxySyncer {
	settings, err := apisettings.BuildSettings()
	if err != nil {
		b.Fatal(err)
	}
	// waitIdle cannot see a pending timer, so it would return before a debounced change is
	// delivered. Measure the undebounced cost of each change instead.
	settings.TranslationDebounce = 0
	krtopts := krtutil.NewKrtOptions(ctx.Done(), nil)
	commoncol, err := collections.NewCommonCollections(ctx, krtopts, cli, wellknown.DefaultGatewayControllerName, *settings)
	if err != nil {
		b.Fatal(err)
	}
	v := noopValidator{}
	plugins := registry.MergePlugins(append(registry.Plugins(ctx, commoncol, *settings, v), krtcollections.NewBuiltinPlugin(ctx))...)
	commoncol.InitPlugins(ctx, plugins, *settings)

	role := xds.OwnerNamespaceNameID(wellknown.GatewayApiProxyValue, "gateways", "kgateway-0")
	uccs := krt.NewStaticCollection(nil, []ir.UniquelyConnectedClient{
		ir.NewUniquelyConnectedClient(role, "gateways", map[string]string{"app": "kgateway-0"}, ir.PodLocality{}),
	}, krtopts.ToOptions("UniqueClients")...)

	s := NewProxySyncer(ctx, wellknown.DefaultGatewayControllerName, cli, uccs, plugins, commoncol, nil, v)
	s.Init(ctx, krtopts)
	return s
}

type noopValidator struct{}

func (noopValidator) Validate(context.Context, *envoybootstrapv3.Bootstrap) error { return nil }

// update writes o through the typed clientset for its kind. The fake tracker stores the object
// verbatim, status included.
func update(ctx context.Context, cli apiclient.Client, o client.Object) error {
	opts := metav1.UpdateOptions{}
	ns := o.GetNamespace()
	var err error
	switch o := o.(type) {
	case *corev1.Service:
		_, err = cli.Kube().CoreV1().Services(ns).Update(ctx, o, opts)
	case *corev1.Pod:
		_, err = cli.Kube().CoreV1().Pods(ns).Update(ctx, o, opts)
	case *corev1.Secret:
		_, err = cli.Kube().CoreV1().Secrets(ns).Update(ctx, o, opts)
	case *corev1.ConfigMap:
		_, err = cli.Kube().CoreV1().ConfigMaps(ns).Update(ctx, o, opts)
	case *discoveryv1.EndpointSlice:
		_, err = cli.Kube().DiscoveryV1().EndpointSlices(ns).Update(ctx, o, opts)
	case *gwv1.Gateway:
		_, err = cli.GatewayAPI().GatewayV1().Gateways(ns).Update(ctx, o, opts)
	case *gwv1.ListenerSet:
		_, err = cli.GatewayAPI().GatewayV1().ListenerSets(ns).Update(ctx, o, opts)
	case *gwv1.HTTPRoute:
		_, err = cli.GatewayAPI().GatewayV1().HTTPRoutes(ns).Update(ctx, o, opts)
	case *gwv1.GRPCRoute:
		_, err = cli.GatewayAPI().GatewayV1().GRPCRoutes(ns).Update(ctx, o, opts)
	default:
		return fmt.Errorf("refreshing %T is not supported; add it to update", o)
	}
	return err
}

type renderedObject struct {
	yaml string
	obj  client.Object
}

func (cfg benchConfig) name() string {
	if cfg.Name != "" {
		return cfg.Name
	}
	if name, ok := cfg.Vars["Name"].(string); ok {
		return name
	}
	return cfg.Template
}

type instance struct {
	namespace string
	// index is the position within the namespace; global is unique across all namespaces.
	index, global int
}

func (cfg benchConfig) instances() []instance {
	var res []instance
	for n := range max(cfg.Namespaces, 1) {
		ns := cfg.Namespace
		if cfg.Namespaces > 0 {
			ns = fmt.Sprintf("%s-%d", cfg.Namespace, n)
		}
		for i := range max(cfg.Replicas, 1) {
			res = append(res, instance{namespace: ns, index: i, global: len(res)})
		}
	}
	return res
}

var benchDecoder = serializer.NewCodecFactory(schemes.DefaultScheme()).UniversalDeserializer()

func renderInstance(t testing.TB, tmpl *template.Template, cfg benchConfig, inst instance, refreshed bool) []renderedObject {
	data := maps.Clone(cfg.Vars)
	if data == nil {
		data = map[string]any{}
	}
	if _, ok := data["Name"]; !ok {
		data["Name"] = fmt.Sprintf("%s-%d", cfg.name(), inst.index)
	}
	data["Namespace"] = inst.namespace
	data["Index"] = inst.global
	data["Rand"] = refreshed
	// Deterministic stand-in for pilot-load's random number; always differs after a refresh.
	randNumber := 1 + inst.global*7919%5000
	if refreshed {
		randNumber += 5000
	}
	data["RandNumber"] = randNumber

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		t.Fatalf("render %s: %v", cfg.Template, err)
	}
	var res []renderedObject
	for doc := range strings.SplitSeq(buf.String(), "\n---\n") {
		if isEmptyYAML(doc) {
			continue
		}
		o, _, err := benchDecoder.Decode([]byte(doc), nil, nil)
		if err != nil {
			t.Fatalf("decode %s: %v\n%s", cfg.Template, err, doc)
		}
		obj := o.(client.Object)
		if obj.GetNamespace() == "" && inst.namespace != "" {
			obj.SetNamespace(inst.namespace)
		}
		// The API server merges stringData into data; do the same.
		if s, ok := obj.(*corev1.Secret); ok && len(s.StringData) > 0 {
			if s.Data == nil {
				s.Data = map[string][]byte{}
			}
			for k, v := range s.StringData {
				s.Data[k] = []byte(v)
			}
			s.StringData = nil
		}
		res = append(res, renderedObject{yaml: doc, obj: obj})
	}
	return res
}

func isEmptyYAML(doc string) bool {
	for line := range strings.SplitSeq(doc, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			return false
		}
	}
	return true
}

func loadBenchTemplates(t testing.TB) map[string]*template.Template {
	files, err := filepath.Glob("testdata/bench/*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	res := map[string]*template.Template{}
	for _, f := range files {
		by, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		name := strings.TrimSuffix(filepath.Base(f), ".yaml")
		tmpl, err := template.New(name).Funcs(sprig.TxtFuncMap()).Parse(string(by))
		if err != nil {
			t.Fatalf("parse %s: %v", f, err)
		}
		res[name] = tmpl
	}
	return res
}

// waitIdle blocks until no other goroutine is running or runnable, which (absent timers) means
// every krt queue has drained.
func waitIdle() {
	samples := [2]metrics.Sample{
		{Name: "/sched/goroutines/running:goroutines"},
		{Name: "/sched/goroutines/runnable:goroutines"},
	}
	// While other goroutines are busy, spin for up to 1ms then back off, so long recomputes do not
	// burn a core polling. Idleness is confirmed without sleeping: a sleep in an idle process takes far
	// longer than requested and would put a floor on every measurement.
	start := time.Now()
	backoff := time.Duration(0)
	for idle := 0; idle < 3; {
		runtime.Gosched()
		metrics.Read(samples[:])
		if samples[0].Value.Uint64() <= 1 && samples[1].Value.Uint64() == 0 { //nolint:gosec // G602: samples is a fixed-size array of two
			idle++
			continue
		}
		idle = 0
		if time.Since(start) > time.Millisecond {
			backoff = min(max(backoff*2, 10*time.Microsecond), time.Millisecond)
			time.Sleep(backoff)
		}
	}
}

// liveHeapBytes returns the heap in use after a full garbage collection.
func liveHeapBytes() float64 {
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return float64(m.HeapAlloc)
}

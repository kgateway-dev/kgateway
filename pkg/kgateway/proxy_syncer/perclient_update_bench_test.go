package proxy_syncer

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"google.golang.org/protobuf/types/known/durationpb"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/runtime/schema"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/translator/irtranslator"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	"github.com/kgateway-dev/kgateway/v2/pkg/validator"
)

// The update benchmarks price the cost the client-keyed design accepts: a change
// to one base reruns every client's walk over every backend, re-applying overlays
// and rebuilding inline CLAs for the pairs that vary. The connect benchmark cannot
// show this; it measures one client's walk. Two operations are measured, each
// with and without strict-mode validation, since validation is the one per-pair
// cost that is not a few function calls:
//
//   - BackendUpdate: one backend's translated output changes. Every client's
//     payload must change.
//   - DestinationRuleUpdate: an overlay input for one backend changes. Only the
//     clients whose overlay applies must change; the rest must not rerun at all.
//
// The fleet shape is deliberately unfavourable to the design: a quarter of the
// backends carry inline endpoints, half of those with a zone-preferring traffic
// distribution (so every client materializes its own CLA for them) and half
// without (so the CLA is built once on the base and shared), an eighth have a
// rule, and a quarter of the clients match rules.
const (
	updateBenchBackends          = 400
	updateBenchClients           = 12
	updateBenchInlineEvery       = 4
	updateBenchZonalInlineEvery  = 8
	updateBenchRuleEvery         = 8
	updateBenchOverlayClientEver = 4
	updateBenchValidationLatency = 200 * time.Microsecond
)

// benchRule stands in for a DestinationRule: an overlay input keyed by backend
// name whose Generation changes the overlay's output.
type benchRule struct {
	Backend    string
	Generation int64
}

func (r benchRule) ResourceName() string { return r.Backend }
func (r benchRule) Equals(o benchRule) bool {
	return r.Backend == o.Backend && r.Generation == o.Generation
}

func updateBenchBackendIndex(in ir.BackendObjectIR) int {
	i, _ := strconv.Atoi(strings.TrimPrefix(in.GetName(), "b"))
	return i
}

func updateBenchTranslator(rules krt.Collection[benchRule], v validator.Validator) *irtranslator.BackendTranslator {
	mode := apisettings.ValidationMode("")
	if v != nil {
		mode = apisettings.ValidationStrict
	}
	return &irtranslator.BackendTranslator{
		ContributedBackends: map[schema.GroupKind]ir.BackendInit{
			{Group: "", Kind: "Service"}: {
				InitEnvoyBackend: func(_ context.Context, in ir.BackendObjectIR, out *envoyclusterv3.Cluster) *ir.EndpointsForBackend {
					// The translated field the backend-update benchmark alternates.
					out.AltStatName = string(in.AppProtocol)
					if updateBenchBackendIndex(in)%updateBenchInlineEvery != 0 {
						out.ClusterDiscoveryType = &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_EDS}
						return nil
					}
					// Inline endpoints. With a zone-preferring distribution every
					// client builds its own CLA; without one the base carries it.
					out.ClusterDiscoveryType = &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_STRICT_DNS}
					eps := ir.NewEndpointsForBackend(in)
					if updateBenchBackendIndex(in)%updateBenchZonalInlineEvery == 0 {
						eps.TrafficDistribution = wellknown.TrafficDistributionPreferSameZone
					}
					for i := range 3 {
						eps.Add(ir.PodLocality{Region: "r1"}, ir.EndpointWithMd{
							LbEndpoint: lbEndpointPipe(fmt.Sprintf("%s-%d", in.GetName(), i)),
						})
					}
					return eps
				},
			},
		},
		ContributedPolicies: map[schema.GroupKind]sdk.PolicyPlugin{
			{Group: "bench", Kind: "Rule"}: {
				PerClientClusterOverlay: func(kctx krt.HandlerContext, _ context.Context, ucc ir.UniquelyConnectedClient, in ir.BackendObjectIR) *sdk.ClusterOverlay {
					// Cheap client gate first, as the real overlays do.
					if ucc.Labels["tier"] != "overlay" {
						return nil
					}
					rule := krt.FetchOne(kctx, rules, krt.FilterKey(in.GetName()))
					if rule == nil {
						return nil
					}
					generation := rule.Generation
					return &sdk.ClusterOverlay{Mutate: func(out *envoyclusterv3.Cluster) {
						out.OutlierDetection = &envoyclusterv3.OutlierDetection{
							Interval: durationpb.New(time.Duration(generation) * time.Second),
						}
					}}
				},
			},
		},
		Validator: v,
		Mode:      mode,
	}
}

type updateBenchFixture struct {
	clients       []ir.UniquelyConnectedClient
	overlayClient []ir.UniquelyConnectedClient
	finalBackends krt.StaticCollection[*ir.BackendObjectIR]
	rules         krt.StaticCollection[benchRule]
	clusters      PerClientEnvoyClusters
}

func newUpdateBenchFixture(b *testing.B, v validator.Validator) *updateBenchFixture {
	b.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	b.Cleanup(cancel)
	krtopts := krtutil.NewKrtOptions(ctx.Done(), nil)

	f := &updateBenchFixture{}
	for i := range updateBenchClients {
		labels := map[string]string{"app": "gw", "pod": fmt.Sprintf("p%d", i)}
		if i%updateBenchOverlayClientEver == 0 {
			labels["tier"] = "overlay"
		}
		c := ir.NewUniquelyConnectedClient(fmt.Sprintf("role-%d", i), "ns", labels, ir.PodLocality{Region: "r1", Zone: fmt.Sprintf("z%d", i%3)})
		f.clients = append(f.clients, c)
		if labels["tier"] == "overlay" {
			f.overlayClient = append(f.overlayClient, c)
		}
	}
	uccs := krt.NewStaticCollection(nil, f.clients, krtopts.ToOptions("UniqueClients")...)

	backends := make([]*ir.BackendObjectIR, 0, updateBenchBackends)
	var rules []benchRule
	for i := range updateBenchBackends {
		backends = append(backends, clustersTestBackendWithProtocol(fmt.Sprintf("b%d", i), "v0"))
		if i%updateBenchRuleEvery == 0 {
			rules = append(rules, benchRule{Backend: fmt.Sprintf("b%d", i), Generation: 1})
		}
	}
	f.finalBackends = krt.NewStaticCollection(nil, backends, krtopts.ToOptions("FinalBackends")...)
	f.rules = krt.NewStaticCollection(nil, rules, krtopts.ToOptions("Rules")...)
	f.clusters = NewPerClientEnvoyClusters(ctx, krtopts, updateBenchTranslator(f.rules, v), f.finalBackends, uccs)

	f.waitFor(b, f.clients, func(row *clustersWithErrors) bool {
		return row != nil && len(row.clusters.Items) == updateBenchBackends
	})
	return f
}

// hashes snapshots each client's stored payload version.
func (f *updateBenchFixture) hashes(clients []ir.UniquelyConnectedClient) map[string]uint64 {
	out := make(map[string]uint64, len(clients))
	for _, c := range clients {
		if row := f.clusters.perClient.GetKey(c.ResourceName()); row != nil {
			out[c.ResourceName()] = row.clustersHash
		}
	}
	return out
}

// waitFor polls until every listed client's stored row satisfies ok.
func (f *updateBenchFixture) waitFor(b *testing.B, clients []ir.UniquelyConnectedClient, ok func(*clustersWithErrors) bool) {
	b.Helper()
	deadline := time.Now().Add(time.Minute)
	for time.Now().Before(deadline) {
		done := true
		for _, c := range clients {
			if !ok(f.clusters.perClient.GetKey(c.ResourceName())) {
				done = false
				break
			}
		}
		if done {
			return
		}
		time.Sleep(200 * time.Microsecond)
	}
	b.Fatal("per-client rows did not converge in time")
}

// waitChanged polls until every listed client's payload version differs from before.
func (f *updateBenchFixture) waitChanged(b *testing.B, clients []ir.UniquelyConnectedClient, before map[string]uint64) {
	b.Helper()
	f.waitFor(b, clients, func(row *clustersWithErrors) bool {
		return row != nil && row.clustersHash != before[row.resourceName]
	})
}

func benchValidators() []struct {
	name string
	v    validator.Validator
} {
	return []struct {
		name string
		v    validator.Validator
	}{
		{name: "NoValidation", v: nil},
		{name: "StrictValidation", v: &benchLatencyValidator{latency: updateBenchValidationLatency}},
	}
}

// BenchmarkPerClientBackendUpdate changes one backend's translated output and
// waits for every client's stored payload to reflect it. The changed backend is
// a zone-preferring inline-endpoint one, so the update also rebuilds one CLA
// per client.
func BenchmarkPerClientBackendUpdate(b *testing.B) {
	for _, tc := range benchValidators() {
		b.Run(tc.name, func(b *testing.B) {
			f := newUpdateBenchFixture(b, tc.v)
			const target = "b0"
			b.ResetTimer()
			i := 0
			for b.Loop() {
				i++
				before := f.hashes(f.clients)
				f.finalBackends.UpdateObject(clustersTestBackendWithProtocol(target, fmt.Sprintf("v%d", i)))
				f.waitChanged(b, f.clients, before)
			}
		})
	}
}

// BenchmarkPerClientDestinationRuleUpdate changes the overlay input for one
// backend and waits for the matching clients' stored payloads to reflect it.
// Clients the overlay does not apply to are checked afterwards to be untouched:
// their transform depends on the rule collection only through the fetches the
// overlay made, and it made none for them.
func BenchmarkPerClientDestinationRuleUpdate(b *testing.B) {
	for _, tc := range benchValidators() {
		b.Run(tc.name, func(b *testing.B) {
			f := newUpdateBenchFixture(b, tc.v)
			const target = "b0"
			var others []ir.UniquelyConnectedClient
			for _, c := range f.clients {
				if c.Labels["tier"] != "overlay" {
					others = append(others, c)
				}
			}
			b.ResetTimer()
			var generation int64 = 1
			for b.Loop() {
				generation++
				before := f.hashes(f.clients)
				f.rules.UpdateObject(benchRule{Backend: target, Generation: generation})
				f.waitChanged(b, f.overlayClient, before)
				b.StopTimer()
				after := f.hashes(others)
				for _, c := range others {
					if after[c.ResourceName()] != before[c.ResourceName()] {
						b.Fatalf("client %s has no matching overlay but its payload changed", c.ResourceName())
					}
				}
				b.StartTimer()
			}
		})
	}
}

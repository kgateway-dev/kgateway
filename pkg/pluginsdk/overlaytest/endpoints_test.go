package overlaytest

import (
	"context"
	"strings"
	"testing"

	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/endpoints"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

var endpointsTestGK = schema.GroupKind{Group: "test", Kind: "EndpointPolicy"}

// zonePreferringPlugin acts on backends whose canonical hostname ends in the
// gated suffix, standing in for a plugin that keys off something other than its
// own attached policies. gate decides what its predicate claims.
func zonePreferringPlugin(gate func(ir.BackendObjectIR) bool, mutate bool) sdk.PolicyPlugin {
	return sdk.PolicyPlugin{
		Name: "zone-preferring",
		PerClientEditEndpoints: func(_ krt.HandlerContext, _ context.Context, _ ir.UniquelyConnectedClient, out endpoints.EndpointInputsEditor) uint64 {
			if !strings.HasSuffix(out.Hostname(), ".gated") {
				return 0
			}
			if mutate {
				out.SetTrafficDistribution(wellknown.TrafficDistributionPreferSameZone)
			}
			return 1
		},
		PerClientEndpointsMayApply: gate,
	}
}

func endpointsFixture(t testing.TB, name, hostname string) ir.BackendObjectIR {
	t.Helper()
	backend := ir.NewBackendObjectIR(ir.ObjectSource{
		Group: "", Kind: "Service", Namespace: "ns", Name: name,
	}, 80, "", "")
	backend.CanonicalHostname = hostname
	return backend
}

func runEndpoints(t *testing.T, plugin sdk.PolicyPlugin) []string {
	t.Helper()
	rec := &recordingTB{TB: t}
	AssertMayApplyCoversEndpointHook(rec, EndpointsCase{
		Plugin: plugin,
		Backends: []ir.BackendObjectIR{
			endpointsFixture(t, "gated", "svc.ns.svc.cluster.local.gated"),
			endpointsFixture(t, "plain", "svc.ns.svc.cluster.local"),
		},
		Clients: []ir.UniquelyConnectedClient{
			ir.NewUniquelyConnectedClient("role", "ns", nil, ir.PodLocality{Region: "r1", Zone: "z1"}),
		},
	})
	return rec.errors
}

func TestAssertMayApplyCoversEndpointHook(t *testing.T) {
	hostnameGate := func(backend ir.BackendObjectIR) bool {
		return strings.HasSuffix(backend.CanonicalHostname, ".gated")
	}

	t.Run("an honest predicate passes", func(t *testing.T) {
		if errs := runEndpoints(t, zonePreferringPlugin(hostnameGate, true)); len(errs) != 0 {
			t.Fatalf("expected no findings, got %v", errs)
		}
	})

	// The bug the checker exists for: the predicate rules out a backend the
	// hook would have acted on, so the framework builds one shared inline CLA
	// and every client silently loses the hook's contribution.
	t.Run("a predicate that rules out a backend the hook acts on is caught", func(t *testing.T) {
		invertedGate := func(backend ir.BackendObjectIR) bool {
			return !strings.HasSuffix(backend.CanonicalHostname, ".gated")
		}
		errs := runEndpoints(t, zonePreferringPlugin(invertedGate, true))
		if len(errs) == 0 {
			t.Fatal("expected the under-declared predicate to be reported")
		}
		if !strings.Contains(strings.Join(errs, "\n"), "the hook contributed") {
			t.Errorf("expected the contribution to be named, got %v", errs)
		}
	})

	// A hook that mutates the resolver and returns 0 is the quieter half of the
	// same bug: ResolveEndpointInputs keeps the mutation even though it skips
	// the hash, so the endpoints move while the contribution claims they did not.
	t.Run("a silent mutation is caught even when the hook returns zero", func(t *testing.T) {
		plugin := sdk.PolicyPlugin{
			Name: "silent-mutator",
			PerClientEditEndpoints: func(_ krt.HandlerContext, _ context.Context, _ ir.UniquelyConnectedClient, out endpoints.EndpointInputsEditor) uint64 {
				if strings.HasSuffix(out.Hostname(), ".gated") {
					return 0
				}
				out.SetTrafficDistribution(wellknown.TrafficDistributionPreferSameZone)
				return 0
			},
			PerClientEndpointsMayApply: hostnameGate,
		}
		errs := runEndpoints(t, plugin)
		if len(errs) == 0 {
			t.Fatal("expected the silent mutation to be reported")
		}
		if !strings.Contains(strings.Join(errs, "\n"), "changed the endpoints") {
			t.Errorf("expected the mutation to be named, got %v", errs)
		}
	})

	t.Run("a case that proves nothing is reported", func(t *testing.T) {
		always := func(ir.BackendObjectIR) bool { return true }
		errs := runEndpoints(t, zonePreferringPlugin(always, true))
		if len(errs) != 1 || !strings.Contains(errs[0], "admitted every backend") {
			t.Errorf("expected the vacuous case to be reported, got %v", errs)
		}

		never := func(ir.BackendObjectIR) bool { return false }
		errs = runEndpoints(t, zonePreferringPlugin(never, false))
		found := false
		for _, err := range errs {
			if strings.Contains(err, "ruled out every backend") {
				found = true
			}
		}
		if !found {
			t.Errorf("expected the non-discriminating predicate to be reported, got %v", errs)
		}
	})
}

// The generic helper AttachedPolicyEndpointsMayApply is what plugins reach for
// when their hook reads only their own attached policies, so its promise is
// worth pinning once here rather than in each plugin that uses it.
func TestAttachedPolicyEndpointsMayApply(t *testing.T) {
	withPolicy := endpointsFixture(t, "with-policy", "svc.ns.svc.cluster.local")
	withPolicy.AttachedPolicies = ir.AttachedPolicies{Policies: map[schema.GroupKind][]ir.PolicyAtt{
		endpointsTestGK: {{GroupKind: endpointsTestGK}},
	}}
	without := endpointsFixture(t, "without-policy", "svc.ns.svc.cluster.local")

	plugin := sdk.PolicyPlugin{
		Name: "attached-policy-reader",
		PerClientEditEndpoints: func(_ krt.HandlerContext, _ context.Context, _ ir.UniquelyConnectedClient, out endpoints.EndpointInputsEditor) uint64 {
			if len(out.PoliciesFor(endpointsTestGK)) == 0 {
				return 0
			}
			out.SetTrafficDistribution(wellknown.TrafficDistributionPreferSameZone)
			return 1
		},
		PerClientEndpointsMayApply: sdk.AttachedPolicyEndpointsMayApply(endpointsTestGK),
	}

	rec := &recordingTB{TB: t}
	AssertMayApplyCoversEndpointHook(rec, EndpointsCase{
		Plugin:   plugin,
		Backends: []ir.BackendObjectIR{withPolicy, without},
		Clients: []ir.UniquelyConnectedClient{
			ir.NewUniquelyConnectedClient("role", "ns", nil, ir.PodLocality{Region: "r1", Zone: "z1"}),
		},
	})
	if len(rec.errors) != 0 {
		t.Fatalf("AttachedPolicyEndpointsMayApply must cover a hook that reads only its own attached policies, got %v", rec.errors)
	}
}

package overlaytest

import (
	"context"
	"strconv"
	"testing"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	"google.golang.org/protobuf/proto"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/endpoints"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

// EndpointsCase is one plugin's PerClientEndpointsMayApply checked against the
// endpoint hook it gates.
type EndpointsCase struct {
	// Plugin is the registration under test. PerClientEndpointsMayApply must be
	// set, along with one of PerClientEditEndpoints or PerClientProcessEndpoints;
	// checking a predicate without the hook it gates is meaningless and fails.
	Plugin sdk.PolicyPlugin
	// Backends are the fixtures checked. At least one must be ruled out by the
	// predicate, because a backend it admits proves nothing, and at least one
	// must be admitted, so the fixtures are shown to tell the two apart.
	Backends []ir.BackendObjectIR
	// Inputs builds the endpoint inputs for a backend. Nil uses
	// [BackendEndpointInputs], which carries the backend's attached policies
	// and a pair of endpoints in different zones so that a hook's effect on
	// prioritization is observable. Supply one when the hook reads endpoints
	// this plugin's backends would really have.
	Inputs func(ir.BackendObjectIR) endpoints.EndpointsInputs
	// Clients are the clients the hook is evaluated for. At least one must be
	// given. A predicate is a promise about every client, so include the ones
	// whose shape the hook would otherwise act on.
	Clients []ir.UniquelyConnectedClient
	// Kctx is the handler context the predicate and hook fetch through. Nil is replaced by
	// krt.TestingDummyContext{}, which serves synced static collections.
	Kctx krt.HandlerContext
}

// AssertMayApplyCoversEndpointHook runs the case. For every backend the
// predicate rules out, it runs the hook anyway, for every client, and requires
// that the hook had nothing to contribute: it returned 0 and left the resolved
// inputs producing the same ClusterLoadAssignment.
//
// Both halves are needed. ResolveEndpointInputs skips a hook's hash when it
// returns 0 but still keeps whatever that hook wrote to the resolver, so a hook
// that mutates and returns 0 still changes the client's endpoints.
//
// Only that direction is asserted. A predicate that admits a backend its hook
// would not touch costs a per-client inline CLA where a shared one would have
// done, which is a performance matter, not a correctness one, and is left to
// review.
func AssertMayApplyCoversEndpointHook(t testing.TB, c EndpointsCase) {
	t.Helper()
	if c.Plugin.PerClientEndpointsMayApply == nil {
		t.Fatalf("overlaytest: plugin %q registers no PerClientEndpointsMayApply", c.Plugin.Name)
	}
	hook := endpointHook(c.Plugin)
	if hook == nil {
		t.Fatalf("overlaytest: plugin %q registers PerClientEndpointsMayApply without an endpoint hook for it to gate", c.Plugin.Name)
	}
	if len(c.Backends) == 0 {
		t.Fatalf("overlaytest: no backends to check")
	}
	if len(c.Clients) == 0 {
		t.Fatalf("overlaytest: no clients to evaluate")
	}
	kctx := c.Kctx
	if kctx == nil {
		kctx = krt.TestingDummyContext{}
	}
	inputsFor := c.Inputs
	if inputsFor == nil {
		inputsFor = BackendEndpointInputs
	}
	ctx := context.Background()

	var ruledOut, admitted int
	for _, backend := range c.Backends {
		if c.Plugin.PerClientEndpointsMayApply(kctx, backend) {
			admitted++
			continue
		}
		ruledOut++
		for _, ucc := range c.Clients {
			inputs := inputsFor(backend)
			before := endpoints.PrioritizeEndpoints(nil, ucc, inputs)

			resolver := endpoints.NewEndpointInputsResolver(inputs)
			contribution := hook(kctx, ctx, ucc, resolver)
			resolved := resolver.Inputs()
			after := endpoints.PrioritizeEndpoints(nil, ucc, resolved)

			if contribution != 0 {
				t.Errorf("overlaytest: PerClientEndpointsMayApply ruled out backend %s, but the hook contributed %d for client %s; the framework skips the hook for such a backend, so that contribution would be lost",
					backend.ResourceName(), contribution, ucc.ResourceName())
			}
			// The assignment is the observable, but it cannot see a change that
			// only matters once the backend has endpoints the fixture lacks, so
			// the inputs behind it are compared too.
			if !proto.Equal(before, after) || !inputs.EndpointsForBackend.Equals(resolved.EndpointsForBackend) {
				t.Errorf("overlaytest: PerClientEndpointsMayApply ruled out backend %s, but the hook changed the endpoints for client %s; a hook that returns 0 still keeps whatever it wrote to the resolver",
					backend.ResourceName(), ucc.ResourceName())
			}
		}
	}
	if ruledOut == 0 {
		t.Errorf("overlaytest: the predicate admitted every backend, so nothing was checked. Include a fixture it rules out")
	}
	if admitted == 0 {
		t.Errorf("overlaytest: the predicate ruled out every backend, so it is not shown to discriminate. Include a fixture it admits")
	}
}

// BackendEndpointInputs builds representative endpoint inputs for a backend:
// its attached policies, which is what EndpointInputsEditor.PoliciesFor reads,
// and two endpoints in different zones of one region, carrying the topology
// labels prioritization keys off.
//
// The endpoints are synthetic because the predicate only decides anything for a
// backend that has inline endpoints, and because a hook that rewrites or
// reorders them needs something to act on before the difference can be seen.
func BackendEndpointInputs(backend ir.BackendObjectIR) endpoints.EndpointsInputs {
	eps := ir.NewEndpointsForBackend(backend)
	eps.AttachedPolicies = backend.AttachedPolicies
	for i, locality := range []ir.PodLocality{
		{Region: "r1", Zone: "z1"},
		{Region: "r1", Zone: "z2"},
	} {
		eps.Add(locality, ir.EndpointWithMd{
			LbEndpoint: &envoyendpointv3.LbEndpoint{
				HostIdentifier: &envoyendpointv3.LbEndpoint_Endpoint{
					Endpoint: &envoyendpointv3.Endpoint{
						Address: &envoycorev3.Address{Address: &envoycorev3.Address_SocketAddress{
							SocketAddress: &envoycorev3.SocketAddress{
								Address:       "10.0.0." + strconv.Itoa(i+1),
								PortSpecifier: &envoycorev3.SocketAddress_PortValue{PortValue: 8080},
							},
						}},
					},
				},
			},
			EndpointMd: ir.EndpointMetadata{Labels: map[string]string{
				corev1.LabelTopologyRegion: locality.Region,
				corev1.LabelTopologyZone:   locality.Zone,
			}},
		})
	}
	return endpoints.EndpointsInputs{EndpointsForBackend: *eps}
}

// endpointHook normalizes a plugin's endpoint hook onto the resolver, the way
// OrderedEndpointPlugins does, so a legacy plugin is checked through the same
// isolation boundary the framework gives it.
func endpointHook(plugin sdk.PolicyPlugin) func(krt.HandlerContext, context.Context, ir.UniquelyConnectedClient, *endpoints.EndpointInputsResolver) uint64 {
	switch {
	case plugin.PerClientEditEndpoints != nil:
		edit := plugin.PerClientEditEndpoints
		return func(kctx krt.HandlerContext, ctx context.Context, ucc ir.UniquelyConnectedClient, out *endpoints.EndpointInputsResolver) uint64 {
			return edit(kctx, ctx, ucc, out)
		}
	case plugin.PerClientProcessEndpoints != nil: //nolint:staticcheck // the legacy hook is gated by the same predicate
		legacy := plugin.PerClientProcessEndpoints //nolint:staticcheck // isolated below exactly as the framework isolates it
		return func(kctx krt.HandlerContext, ctx context.Context, ucc ir.UniquelyConnectedClient, out *endpoints.EndpointInputsResolver) uint64 {
			return legacy(kctx, ctx, ucc, out.LegacyMutableInputs())
		}
	default:
		return nil
	}
}

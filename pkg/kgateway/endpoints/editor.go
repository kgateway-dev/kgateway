package endpoints

import (
	"maps"
	"slices"

	"google.golang.org/protobuf/proto"
	"istio.io/api/networking/v1alpha3"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

// EndpointInputsEditor is the mutation surface exposed to per-client endpoint
// plugins. The shared endpoint inputs are only available through read accessors;
// plugins express output changes through setters or by installing a replacement
// endpoint set built with [EndpointSetBuilder].
//
// This prevents plugins from accidentally mutating KRT-owned maps, slices, or
// protobufs shared by other clients. Plugins that still use the legacy raw-input
// hook are isolated by a defensive deep copy in the translation framework.
type EndpointInputsEditor interface {
	BackendLabels() map[string]string
	Hostname() string
	Port() uint32
	PoliciesFor(schema.GroupKind) []PolicyView

	SetPriorityInfo(*PriorityInfo)
	SetTrafficDistribution(wellknown.TrafficDistribution)

	ForEachEndpoint(func(ir.PodLocality, EndpointView) bool)
	NewEndpointSet() *EndpointSetBuilder
	ReplaceEndpoints(*EndpointSetBuilder)
}

// EndpointInputsResolver owns one client's working endpoint inputs. Translation
// code retains the concrete resolver to retrieve the result; plugins receive only
// the restricted [EndpointInputsEditor] interface.
type EndpointInputsResolver struct {
	inputs                 EndpointsInputs
	legacyIsolated         bool
	endpointHashesReusable bool
}

// NewEndpointInputsResolver creates a per-client working view over shared
// endpoint inputs. The shared nested state remains immutable until a plugin
// explicitly builds a replacement or requests the legacy mutable view.
func NewEndpointInputsResolver(inputs EndpointsInputs) *EndpointInputsResolver {
	return &EndpointInputsResolver{inputs: inputs, endpointHashesReusable: true}
}

// Inputs returns the resolved per-client inputs after all plugins have run.
func (e *EndpointInputsResolver) Inputs() EndpointsInputs {
	return e.inputs
}

// LegacyMutableInputs returns a transitively isolated input graph for the legacy
// raw mutation hook. The copy is made at most once per client even when multiple
// legacy plugins run.
func (e *EndpointInputsResolver) LegacyMutableInputs() *EndpointsInputs {
	if !e.legacyIsolated {
		e.inputs = cloneEndpointsInputs(e.inputs)
		e.legacyIsolated = true
	}
	// A legacy plugin can mutate endpoint protos or metadata without going
	// through EndpointsForBackend.Add, so their cached contribution hashes can
	// no longer be trusted by a later editor plugin.
	e.endpointHashesReusable = false
	return &e.inputs
}

func (e *EndpointInputsResolver) BackendLabels() map[string]string {
	return maps.Clone(e.inputs.EndpointsForBackend.BackendLabels)
}

func (e *EndpointInputsResolver) Hostname() string {
	return e.inputs.EndpointsForBackend.Hostname
}

func (e *EndpointInputsResolver) Port() uint32 {
	return e.inputs.EndpointsForBackend.Port
}

// PoliciesFor returns read-only views of the policies of one kind attached to
// this backend. A view cannot reach the attachment's mutable metadata, so no
// copy of it is made: this runs per client per backend, and the callers only
// read.
func (e *EndpointInputsResolver) PoliciesFor(groupKind schema.GroupKind) []PolicyView {
	attachments := e.inputs.EndpointsForBackend.AttachedPolicies.Policies[groupKind]
	if len(attachments) == 0 {
		return nil
	}
	views := make([]PolicyView, len(attachments))
	for i, attachment := range attachments {
		views[i] = PolicyView{attachment: attachment}
	}
	return views
}

func (e *EndpointInputsResolver) SetPriorityInfo(priorityInfo *PriorityInfo) {
	// A plugin owns the value it passes in and may reuse or mutate it after this
	// call. Snapshot the transitive graph so later writes cannot mutate resolved
	// inputs behind the editor boundary.
	e.inputs.PriorityInfo = clonePriorityInfo(priorityInfo)
}

func (e *EndpointInputsResolver) SetTrafficDistribution(distribution wellknown.TrafficDistribution) {
	e.inputs.EndpointsForBackend.TrafficDistribution = distribution
}

// ForEachEndpoint visits the immutable source endpoint set. Returning false
// stops iteration. EndpointView exposes scalar reads and an explicit Clone method
// for plugins that need to modify an endpoint.
func (e *EndpointInputsResolver) ForEachEndpoint(fn func(ir.PodLocality, EndpointView) bool) {
	for locality, localityEndpoints := range e.inputs.EndpointsForBackend.LbEps {
		for _, endpoint := range localityEndpoints {
			if !fn(locality, EndpointView{
				locality:     locality,
				endpoint:     endpoint,
				hashReusable: e.endpointHashesReusable,
			}) {
				return
			}
		}
	}
}

func (e *EndpointInputsResolver) NewEndpointSet() *EndpointSetBuilder {
	return &EndpointSetBuilder{state: &endpointSetBuilderState{
		endpoints: e.inputs.EndpointsForBackend.EmptyCopy(),
		owner:     e,
	}}
}

func (e *EndpointInputsResolver) ReplaceEndpoints(replacement *EndpointSetBuilder) {
	if replacement == nil {
		panic("endpoint replacement builder is nil")
	}
	state := replacement.mutableState()
	if state.owner != e {
		panic("endpoint replacement builder belongs to a different resolver")
	}

	// Consume the shared state rather than only this particular builder value:
	// copying an EndpointSetBuilder before installation must not create another
	// live handle to the installed map.
	installed := state.endpoints
	state.endpoints = ir.EndpointsForBackend{}
	state.owner = nil
	state.consumed = true

	// Take only what the builder owns — the endpoint set and its content hash.
	// A builder is seeded from the inputs as they stood when it was created, so
	// installing its whole EndpointsForBackend would roll back every setter
	// called while the replacement was being populated. SetTrafficDistribution
	// between NewEndpointSet and ReplaceEndpoints is exactly that shape, and it
	// would fail silently: the resolved hash faithfully versions the reverted
	// state.
	e.inputs.EndpointsForBackend.AdoptEndpointsFrom(&installed)
	e.endpointHashesReusable = true
}

// PolicyView is an immutable view of one policy attachment. It exposes what
// endpoint plugins actually need — the policy IR, whether the attachment failed
// IR construction, and the identity a plugin folds into its version hash —
// while keeping PolicyRef, Errors, and MergeOrigins out of reach. That is what
// lets [EndpointInputsResolver.PoliciesFor] hand out attachments without
// deep-copying metadata that no caller writes.
type PolicyView struct {
	attachment ir.PolicyAtt
}

// PolicyIR returns the attached policy's IR, which is immutable KRT state and
// therefore safe to share across clients.
func (p PolicyView) PolicyIR() ir.PolicyIR {
	return p.attachment.PolicyIr
}

// HasErrors reports whether this attachment failed IR construction, in which
// case its PolicyIR must not be applied.
func (p PolicyView) HasErrors() bool {
	return len(p.attachment.Errors) > 0
}

// RefString identifies the attached policy object. Plugins hash it to version
// their contribution; see [EndpointEditorPlugin] on why that hash is
// load-bearing.
func (p PolicyView) RefString() string {
	return ir.PolicyRefString(p.attachment.PolicyRef)
}

// Generation is the observed generation of the attached policy object.
func (p PolicyView) Generation() int64 {
	return p.attachment.Generation
}

// EndpointView is an immutable view of one endpoint from shared input state.
// Clone must be called before changing any endpoint proto or metadata label.
type EndpointView struct {
	locality     ir.PodLocality
	endpoint     ir.EndpointWithMd
	hashReusable bool
}

func (e EndpointView) Label(name string) string {
	return e.endpoint.EndpointMd.Labels[name]
}

// SocketAddress returns the socket address and whether the endpoint uses a
// socket address. An empty address with ok=true is distinct from a non-socket
// endpoint.
func (e EndpointView) SocketAddress() (address string, ok bool) {
	socketAddress := e.endpoint.GetEndpoint().GetAddress().GetSocketAddress()
	if socketAddress == nil {
		return "", false
	}
	return socketAddress.GetAddress(), true
}

func (e EndpointView) LoadBalancingWeight() uint32 {
	return e.endpoint.GetLoadBalancingWeight().GetValue()
}

// Clone returns a mutable, transitively isolated endpoint value the caller
// owns. Prefer [EndpointSetBuilder.AddModified] when the clone is only being
// made in order to contribute a changed endpoint: it clones once where this
// plus Add clones twice.
func (e EndpointView) Clone() ir.EndpointWithMd {
	return e.endpoint.Clone()
}

// EndpointSetBuilder accumulates a replacement endpoint set. Unchanged
// endpoints are structurally shared with the immutable source and keep their
// precomputed contribution hash; changed and synthesized ones are isolated by
// the builder itself, so nothing a plugin holds stays reachable from the
// resolved set.
//
// A builder belongs to the resolver that created it and is consumed by
// ReplaceEndpoints. Any later use panics, including through a copied builder
// value. Only the endpoint set and its hash are installed: the identity fields
// it is seeded with are inert.
//
// Those panics are deliberate and uncontained. A nil, foreign, or consumed
// builder is a deterministic bug that a plugin's own tests hit on the first
// run, and silently dropping the contribution instead would ship a client
// whose endpoints nobody asked for.
type EndpointSetBuilder struct {
	state *endpointSetBuilderState
}

type endpointSetBuilderState struct {
	endpoints ir.EndpointsForBackend
	owner     *EndpointInputsResolver
	consumed  bool
}

func (b *EndpointSetBuilder) mutableState() *endpointSetBuilderState {
	if b == nil || b.state == nil {
		panic("endpoint replacement builder is uninitialized")
	}
	if b.state.consumed {
		panic("endpoint replacement builder has already been consumed")
	}
	return b.state
}

// AddUnchanged structurally shares an immutable endpoint and reuses its
// precomputed contribution hash. Moving it to another locality, or using a view
// exposed after a legacy mutable hook, safely falls back to a fresh hash.
//
// This is the one path that installs an endpoint without copying it, and it can
// be: a view only ever names source state the plugin has no way to write to.
// Add and AddModified take values the plugin does own, so they copy.
func (b *EndpointSetBuilder) AddUnchanged(locality ir.PodLocality, endpoint EndpointView) {
	state := b.mutableState()
	if locality != endpoint.locality || !endpoint.hashReusable {
		// Locality participates in the endpoint contribution hash, so moving an
		// otherwise unchanged endpoint still requires a fresh hash. A legacy
		// plugin may also have mutated the endpoint behind the view, invalidating
		// the cached contribution.
		state.endpoints.Add(locality, endpoint.endpoint)
		return
	}
	state.endpoints.ReuseEndpoint(locality, endpoint.endpoint)
}

// AddModified contributes a changed version of an existing endpoint. The
// builder clones base, hands the clone to modify, and hashes the result once
// modify returns.
//
// This is the shape to reach for. The plugin never names the value it is
// contributing, so it cannot accidentally retain it and mutate it after the
// hash is taken or into a later client's pass, and the endpoint is cloned once
// where [EndpointView.Clone] followed by Add clones twice. A plugin that
// deliberately captures the pointer out of modify escapes that, which no Go
// signature can prevent; use Add if you want the copy made unconditionally.
func (b *EndpointSetBuilder) AddModified(locality ir.PodLocality, base EndpointView, modify func(*ir.EndpointWithMd)) {
	state := b.mutableState()
	if modify == nil {
		panic("endpoint modifier is nil")
	}
	endpoint := base.endpoint.Clone()
	modify(&endpoint)
	state.endpoints.Add(locality, endpoint)
}

// Add contributes an endpoint the plugin synthesized itself. The value is deep
// copied on the way in, so the caller keeps ownership of what it passed and may
// go on mutating or reusing it. Without that copy a plugin that builds one
// endpoint and adds it for every client would put a single proto pointer into
// every client's resolved set, and a later write would reach back into clients
// already resolved.
//
// Use [EndpointSetBuilder.AddModified] for endpoints derived from an existing
// one; it avoids the second clone.
func (b *EndpointSetBuilder) Add(locality ir.PodLocality, endpoint ir.EndpointWithMd) {
	b.mutableState().endpoints.Add(locality, endpoint.Clone())
}

func cloneEndpointsInputs(in EndpointsInputs) EndpointsInputs {
	out := in
	out.EndpointsForBackend = cloneEndpointsForBackend(in.EndpointsForBackend)
	out.PriorityInfo = clonePriorityInfo(in.PriorityInfo)
	return out
}

func cloneEndpointsForBackend(in ir.EndpointsForBackend) ir.EndpointsForBackend {
	out := in
	out.BackendLabels = maps.Clone(in.BackendLabels)
	out.AttachedPolicies = cloneAttachedPolicies(in.AttachedPolicies)
	out.LbEps = make(ir.LocalityLbMap, len(in.LbEps))
	for locality, localityEndpoints := range in.LbEps {
		cloned := make([]ir.EndpointWithMd, len(localityEndpoints))
		for i, endpoint := range localityEndpoints {
			cloned[i] = endpoint.Clone()
		}
		out.LbEps[locality] = cloned
	}
	return out
}

func cloneAttachedPolicies(in ir.AttachedPolicies) ir.AttachedPolicies {
	if in.Policies == nil {
		return ir.AttachedPolicies{}
	}
	out := ir.AttachedPolicies{Policies: make(map[schema.GroupKind][]ir.PolicyAtt, len(in.Policies))}
	for groupKind, attachments := range in.Policies {
		out.Policies[groupKind] = clonePolicyAttachments(attachments)
	}
	return out
}

func clonePolicyAttachments(in []ir.PolicyAtt) []ir.PolicyAtt {
	out := slices.Clone(in)
	for i := range out {
		if out[i].PolicyRef != nil {
			policyRef := *out[i].PolicyRef
			out[i].PolicyRef = &policyRef
		}
		out[i].Errors = slices.Clone(out[i].Errors)
		if out[i].MergeOrigins != nil {
			out[i].MergeOrigins = make(ir.MergeOrigins, len(out[i].MergeOrigins))
			for field, origins := range in[i].MergeOrigins {
				out[i].MergeOrigins[field] = origins.Clone()
			}
		}
	}
	return out
}

func clonePriorityInfo(in *PriorityInfo) *PriorityInfo {
	if in == nil {
		return nil
	}
	out := *in
	if in.FailoverPriority != nil {
		prioritizer := *in.FailoverPriority
		prioritizer.priorityLabels = slices.Clone(in.FailoverPriority.priorityLabels)
		prioritizer.priorityLabelOverrides = maps.Clone(in.FailoverPriority.priorityLabelOverrides)
		out.FailoverPriority = &prioritizer
	}
	out.Failover = make([]*v1alpha3.LocalityLoadBalancerSetting_Failover, len(in.Failover))
	for i, failover := range in.Failover {
		if failover != nil {
			out.Failover[i] = proto.Clone(failover).(*v1alpha3.LocalityLoadBalancerSetting_Failover)
		}
	}
	return &out
}

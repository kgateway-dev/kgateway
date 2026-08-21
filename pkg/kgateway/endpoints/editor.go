package endpoints

import (
	"maps"
	"slices"

	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
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
	inputs         EndpointsInputs
	legacyIsolated bool
}

// NewEndpointInputsResolver creates a per-client working view over shared
// endpoint inputs. The shared nested state remains immutable until a plugin
// explicitly builds a replacement or requests the legacy mutable view.
func NewEndpointInputsResolver(inputs EndpointsInputs) *EndpointInputsResolver {
	return &EndpointInputsResolver{inputs: inputs}
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
	e.inputs.PriorityInfo = priorityInfo
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
			if !fn(locality, EndpointView{endpoint: endpoint}) {
				return
			}
		}
	}
}

func (e *EndpointInputsResolver) NewEndpointSet() *EndpointSetBuilder {
	return &EndpointSetBuilder{endpoints: e.inputs.EndpointsForBackend.EmptyCopy()}
}

func (e *EndpointInputsResolver) ReplaceEndpoints(replacement *EndpointSetBuilder) {
	if replacement == nil {
		return
	}
	// The builder started from EmptyCopy, which reseeds the equality hash from
	// backend identity, so it carries no trace of anything the row's owner
	// folded in after construction — newFinalBackendEndpoints folds the
	// attached-policy hash, which changes what the endpoints translate into
	// without changing the endpoints. Fold the source row's hash back in: the
	// resolved hash keys CLA interning and the KRT equality of the resolved
	// row, so dropping it would serve one policy state's load assignment for
	// another's. This over-discriminates (the result now varies with the source
	// endpoints too, costing a missed dedup at worst) which is the safe
	// direction.
	replacement.endpoints.FoldVersion(e.inputs.EndpointsForBackend.LbEpsEqualityHash)
	e.inputs.EndpointsForBackend = replacement.endpoints
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
	endpoint ir.EndpointWithMd
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

// Clone returns a mutable, transitively isolated endpoint value.
func (e EndpointView) Clone() ir.EndpointWithMd {
	out := e.endpoint
	if e.endpoint.LbEndpoint != nil {
		out.LbEndpoint = proto.Clone(e.endpoint.LbEndpoint).(*envoyendpointv3.LbEndpoint)
	}
	out.EndpointMd.Labels = maps.Clone(e.endpoint.EndpointMd.Labels)
	return out
}

// EndpointSetBuilder constructs a replacement EndpointsForBackend while
// preserving its identity and precomputed hash semantics. Unchanged endpoints
// may be structurally shared; modified endpoints must be added after Clone.
type EndpointSetBuilder struct {
	endpoints ir.EndpointsForBackend
}

func (b *EndpointSetBuilder) AddUnchanged(locality ir.PodLocality, endpoint EndpointView) {
	b.endpoints.Add(locality, endpoint.endpoint)
}

func (b *EndpointSetBuilder) Add(locality ir.PodLocality, endpoint ir.EndpointWithMd) {
	b.endpoints.Add(locality, endpoint)
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
			cloned[i] = EndpointView{endpoint: endpoint}.Clone()
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

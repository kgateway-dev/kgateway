package pluginsdk

import (
	"context"
	"encoding/json"
	"errors"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/krt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/endpoints"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/statussync"
)

// ErrNotFound is returned when a requested resource is not found
var ErrNotFound = errors.New("not found")

type (
	EndpointsInputs      = endpoints.EndpointsInputs
	EndpointInputsEditor = endpoints.EndpointInputsEditor
	EndpointSetBuilder   = endpoints.EndpointSetBuilder
	EndpointView         = endpoints.EndpointView
	PolicyView           = endpoints.PolicyView
	ProcessBackend       func(ctx context.Context, pol ir.PolicyIR, in ir.BackendObjectIR, out *envoyclusterv3.Cluster)
	// EndpointEditorPlugin edits per-client endpoint inputs through a
	// copy-on-write API. Read-only source state is exposed through accessors;
	// endpoint rewrites build a replacement set and clone only modified protos.
	// The returned hash must capture effects not already represented by the
	// replacement endpoint set's LbEpsEqualityHash or the framework's
	// load-balancing-context hash.
	EndpointEditorPlugin func(
		kctx krt.HandlerContext,
		ctx context.Context,
		ucc ir.UniquelyConnectedClient,
		out EndpointInputsEditor,
	) uint64
	// EndpointPlugin is the legacy mutable endpoint hook. Its returned hash must
	// capture every per-client effect not reflected by the resolved endpoint or
	// load-balancing-context hashes because those hashes key CLA interning.
	// Deprecated: use EndpointEditorPlugin. The framework deep-copies all
	// mutable nested state before invoking this hook.
	EndpointPlugin func(
		kctx krt.HandlerContext,
		ctx context.Context,
		ucc ir.UniquelyConnectedClient,
		out *EndpointsInputs,
	) uint64
)

// ClusterOverlay carries per-client cluster mutations. Returning nil from a
// PerClientClusterOverlay means the client/backend pair needs no mutation.
// Mutate receives a fresh clone and must not retain it after returning. When
// every applicable overlay leaves the clone equal to the shared base, the pair
// keeps using the base, so Mutate may leave the cluster unchanged when the
// decision depends on the cluster itself. Prefer returning nil whenever the
// decision can be made without the cluster: that skips the clone and the
// comparison.
type ClusterOverlay struct {
	Mutate func(out *envoyclusterv3.Cluster)
}

// PerClientClusterOverlay decides whether a client/backend pair needs a
// mutation on top of the shared base cluster, and returns it if so. Returning
// nil is the common case and keeps the pair on the shared base with nothing
// allocated for it.
//
// Applicable overlays mutate the same clone in (Group, Kind) order for stable
// proto content. This ordering does not define policy precedence. Each overlay
// must write only its owned fields and must not depend on another overlay's
// mutations. The framework does not enforce field ownership.
// So do not write an overlay that depends on running before or after another,
// or that expects to observe another's mutation. Confine each overlay to the
// fields it owns. The overlays satisfy that today -- destrule writes
// outlier detection, locality LB config and TCP keepalive; waypoint rewrites
// the discovery type and load assignment -- but the framework does not enforce
// it. Waypoint owns the discovery-type transition and clears any inherited
// locality mode when replacing backend endpoints with a service VIP; that
// redirect cannot use the backend endpoints' locality weights.
//
// Fetches through kctx register KRT dependencies. All fields read from in must
// be covered by the accompanying OverlayInputsHash; otherwise KRT can retain
// a base row whose backend inputs are stale.
type PerClientClusterOverlay func(
	kctx krt.HandlerContext,
	ctx context.Context,
	ucc ir.UniquelyConnectedClient,
	in ir.BackendObjectIR,
) *ClusterOverlay

// OverlayInputsHash hashes every BackendObjectIR field whose change can affect
// the accompanying per-client cluster hook. KRT retains equal base rows, so
// omitting an input can leave clients using stale backend state.
//
// Inputs fetched through krt.Fetch are tracked separately. Extra hash inputs
// are safe but cause unnecessary client recomputation.
//
// Register it beside PerClientClusterOverlay or PerClientProcessBackend. Hooks
// without this declaration fall back to comparing backend IR and backing-object
// version, rerunning clients on every object write. Hooks may apply globally,
// so the framework cannot infer their applicability from policy attachments.
// Declaring inputs avoids that fanout without migrating the mutation hook.
// Use pkg/pluginsdk/overlaytest to check the declaration against the hook.
type OverlayInputsHash func(in ir.BackendObjectIR) uint64

// ProcessBaseCluster is a client-independent cluster mutation that runs for
// every backend, whether or not a policy is attached to it. It runs during
// base translation, after every ProcessBackend hook (so it sees the transport
// sockets and other fields those policies set, and may wrap them) and before
// any PerClientClusterOverlay. Hooks from different plugins run in
// (Group, Kind) order.
//
// Use it instead of a PerClientClusterOverlay that would return the same
// mutation for every client: the result lands once on the shared base rather
// than on a clone per client. The base is re-translated whenever the backend
// IR changes, so no input declaration is needed for fields read from in.
// Fetches through kctx register dependencies of the base translation.
type ProcessBaseCluster func(
	kctx krt.HandlerContext,
	ctx context.Context,
	in ir.BackendObjectIR,
	out *envoyclusterv3.Cluster,
)

// PerClientProcessBackend is the legacy eager cluster mutation hook.
// Deprecated: use PerClientClusterOverlay. Legacy hooks are treated as
// applicable to every client because they cannot report a no-op cheaply.
type PerClientProcessBackend func(
	kctx krt.HandlerContext,
	ctx context.Context,
	ucc ir.UniquelyConnectedClient,
	in ir.BackendObjectIR,
	out *envoyclusterv3.Cluster,
)

// PolicyStatusInputs is provided to a PolicyPlugin's RegisterPolicyStatus hook. The plugin
// registers its raw collection, keyed report reducer, and just-in-time writer.
type PolicyStatusInputs = statussync.RegistrationInputs

// AttachedPolicyEndpointsMayApply is a PerClientEndpointsMayApply for plugins
// whose endpoint hook reads only policies of gk attached to the backend (via
// EndpointInputsEditor.PoliciesFor): with none attached, the hook has nothing
// to act on for any client.
func AttachedPolicyEndpointsMayApply(gk schema.GroupKind) func(krt.HandlerContext, ir.BackendObjectIR) bool {
	return func(_ krt.HandlerContext, backend ir.BackendObjectIR) bool {
		return len(backend.AttachedPolicies.Policies[gk]) > 0
	}
}

type PolicyPlugin struct {
	Name                      string
	NewGatewayTranslationPass func(tctx ir.GwTranslationCtx, reporter reporter.Reporter) ir.ProxyTranslationPass

	// Backend processing for envoy proxy
	ProcessBackend ProcessBackend
	// ProcessBaseCluster runs for every backend after all ProcessBackend
	// hooks; see ProcessBaseCluster.
	ProcessBaseCluster      ProcessBaseCluster
	PerClientClusterOverlay PerClientClusterOverlay
	// OverlayInputsHash declares the backend fields the per-client hook reads.
	// Required for efficient change detection with either hook; absent a
	// declaration, consumers compare backend IR and backing-object version.
	OverlayInputsHash OverlayInputsHash
	// Deprecated: use PerClientClusterOverlay.
	PerClientProcessBackend PerClientProcessBackend
	PerClientEditEndpoints  EndpointEditorPlugin
	// Deprecated: use PerClientEditEndpoints.
	PerClientProcessEndpoints EndpointPlugin
	// PerClientEndpointsMayApply reports whether the endpoint hook can contribute
	// to this backend for any client. False skips the hook: it must return zero
	// and leave inputs unchanged for every client. Nil assumes it may apply.
	//
	// If all hooks rule the backend out and prioritization is client-independent,
	// the framework builds its inline CLA on the shared base. Hooks that only
	// read their own attached policies can use AttachedPolicyEndpointsMayApply.
	// Fetch through the supplied base-translation HandlerContext to register
	// dependencies that invalidate this decision.
	PerClientEndpointsMayApply func(kctx krt.HandlerContext, backend ir.BackendObjectIR) bool

	Policies       krt.Collection[ir.PolicyWrapper]
	GlobalPolicies func(krt.HandlerContext) ir.PolicyIR
	// PoliciesFetch can optionally be set if the plugin needs a custom mechanism for fetching the policy IR,
	// rather than the default behavior of fetching by name from the aggregated policy KRT collection
	PoliciesFetch func(n, ns string) ir.PolicyIR
	MergePolicies func(pols []ir.PolicyAtt) ir.PolicyAtt

	// RegisterPolicyStatus, when set, is called once by the proxy syncer after the report
	// collections are built. The plugin derives its per-object desired-status collection
	// from the provided report collection and registers it, along with a status writer
	// for its GVK. Plugins that do not report status may leave this unset.
	RegisterPolicyStatus func(inputs PolicyStatusInputs)
}

type BackendPlugin struct {
	ir.BackendInit
	AliasKinds []schema.GroupKind
	// RawBackends is the informer-backed source for status reconciliation. It is shared
	// with the translated Backends collection so status does not create another wrapper.
	// Backend plugins for the Backend GVK must provide it; otherwise resource-driven Backend
	// status reconciliation is disabled and the proxy syncer logs an error during setup.
	RawBackends krt.Collection[*kgateway.Backend]
	Backends    krt.Collection[ir.BackendObjectIR]
	Endpoints   krt.Collection[ir.EndpointsForBackend]
	// ExtraConditions, when set, contributes additional status conditions to the
	// Backend resource beyond the Accepted condition (e.g. the EC2 EndpointsDiscovered
	// condition produced by runtime endpoint discovery). May be nil.
	ExtraConditions krt.Collection[ir.BackendObjectStatus]
}

type KGwTranslator interface {
	// This function is called by the reconciler when a K8s Gateway resource is created or updated.
	// It returns an instance of the kgateway Proxy resource, that should configure a target kgateway Proxy workload.
	// A null return value indicates the K8s Gateway resource failed to translate into a kgateway Proxy. The error will be reported on the provided reporter.
	Translate(kctx krt.HandlerContext,
		ctx context.Context,
		gateway *ir.Gateway,
		reporter reporter.Reporter) *ir.GatewayIR
}
type (
	GwTranslatorFactory func(gw *gwv1.Gateway) KGwTranslator
	ContributesPolicies map[schema.GroupKind]PolicyPlugin
)

type Plugin struct {
	ContributesPolicies     ContributesPolicies
	ContributesBackends     map[schema.GroupKind]BackendPlugin
	ContributesGwTranslator GwTranslatorFactory
	// ContributesLeaderAction is a lifecycle hook called after all collections are synced
	// allowing Plugins to register handlers against collections, e.g. for status reporting
	// This is executed only on a leader pod.
	ContributesLeaderAction map[schema.GroupKind]func()
	// extra has sync beyond primary resources in the collections above
	ExtraHasSynced func() bool
}

type (
	AncestorReports map[ir.ObjectSource][]error
	PolicyReport    map[ir.AttachedPolicyRef]AncestorReports
)

// marshal json for krt debugging
func (p PolicyReport) MarshalJSON() ([]byte, error) {
	m := map[string]map[string][]error{}
	for key, pol := range p {
		objErrMap := map[string][]error{}
		for objKey, errs := range pol {
			objErrMap[objKey.ResourceName()] = errs
		}
		m[key.ID()] = objErrMap
	}
	return json.Marshal(m)
}

func (p Plugin) HasSynced() bool {
	for _, up := range p.ContributesBackends {
		if up.Backends != nil && !up.Backends.HasSynced() {
			return false
		}
		if up.Endpoints != nil && !up.Endpoints.HasSynced() {
			return false
		}
		if up.ExtraConditions != nil && !up.ExtraConditions.HasSynced() {
			return false
		}
	}
	for _, pol := range p.ContributesPolicies {
		if pol.Policies != nil && !pol.Policies.HasSynced() {
			return false
		}
	}
	if p.ExtraHasSynced != nil && !p.ExtraHasSynced() {
		return false
	}
	return true
}

type K8sGatewayExtensions2 struct {
	Plugins []Plugin
}

func CloneObjectMetaForStatus(m metav1.ObjectMeta) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            m.GetName(),
		Namespace:       m.GetNamespace(),
		ResourceVersion: m.GetResourceVersion(),
	}
}

// GatewayControllerExtension is an interface for extending the Gateway controller with custom behavior
type GatewayControllerExtension interface {
	// Register is called to allow the extension to interact with the Queue used to reconcile Gateways,
	// and access to a ResourceEventHandler that the extension can use to integrate additional Gateway parameter events
	// that should contribute to triggering Gateway reconciliation
	Register(gatewayQueue controllers.Queue, gatewayParamEventHandler cache.ResourceEventHandler)

	// Start is called to start the extension. It must be non-blocking.
	Start(context.Context) error

	// Stop is called to stop the extension.
	Stop() error
}

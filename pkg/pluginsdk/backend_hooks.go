package pluginsdk

import (
	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

// BackendHooks groups every hook a PolicyPlugin can contribute to backend,
// cluster and endpoint translation. All fields are optional.
//
// Hooks from different plugins run in (Group, Kind) order. That order keeps
// generated protos stable; it does not define policy precedence.
//
// Translation of one backend runs in this order:
//  1. ProcessBackend, once per policy of the plugin's kind attached to the
//     backend (after MergePolicies, if set).
//  2. ProcessBaseCluster, for every backend.
//  3. ProcessBaseClusterResources, for every backend.
//  4. Per client: PerClientCluster, then PerClientEndpoints.
type BackendHooks struct {
	// ProcessBackend applies one attached policy to the shared base cluster.
	// Setting it also makes the plugin's policies attachable to backends.
	ProcessBackend ProcessBackend
	// ProcessBaseCluster is a client-independent mutation that runs for every
	// backend, policy attached or not; see ProcessBaseCluster.
	ProcessBaseCluster ProcessBaseCluster
	// ProcessBaseClusterResources contributes listeners and secrets that ship
	// with the base cluster; see ProcessBaseClusterResources.
	ProcessBaseClusterResources ProcessBaseClusterResources
	// ClaimEmittedClusters declares destinations the configuration never names;
	// only plugins that pick a destination at request time need it. See
	// ClusterEmissionClaim.
	ClaimEmittedClusters ClaimEmittedClusters
	// PerClientCluster mutates the cluster for individual clients.
	PerClientCluster *PerClientCluster
	// PerClientEndpoints edits the endpoints for individual clients.
	PerClientEndpoints *PerClientEndpoints
}

// PerClientCluster is a per-client cluster hook together with the declaration
// of the backend fields it reads. The two are one unit: the framework retains
// a client's cluster while InputsHash is unchanged, so they must be written and
// reviewed together. Use pkg/pluginsdk/overlaytest to check InputsHash against
// Overlay.
//
// A PerClientCluster without Overlay registers nothing.
type PerClientCluster struct {
	// Overlay decides, per client, whether the cluster needs a mutation on top
	// of the shared base; see PerClientClusterOverlay.
	Overlay PerClientClusterOverlay

	// InputsHash must hash every BackendObjectIR field Overlay reads, or a change to an unhashed field leaves clients on stale clusters;
	// see OverlayInputsHash. Inputs fetched through krt.Fetch are tracked
	// separately and need not be hashed.
	//
	// Leaving it nil is a plugin bug for Overlay: the framework logs an error
	// and reruns every client on every write to the backend. That is never
	// stale, only expensive.
	InputsHash OverlayInputsHash
}

// PerClientEndpoints is a per-client endpoint editor together with the
// predicate that rules backends out of it. The predicate decides whether a
// backend's inline CLA can be built once on the shared base, so it must agree
// with the editor: overlaytest.AssertMayApplyCoversEndpointHook checks the
// pair.
//
// A PerClientEndpoints without Edit registers nothing.
type PerClientEndpoints struct {
	// Edit edits one client's endpoint inputs; see EndpointEditorPlugin.
	Edit EndpointEditorPlugin

	// MayApply reports whether Edit can contribute to this backend
	// for any client. False skips the hook: it must then return zero and
	// leave inputs unchanged for every client. Nil assumes it may apply, which
	// forces a per-client build of every backend's inline CLA.
	//
	// If every endpoint hook rules a backend out and prioritization is
	// client-independent, the framework builds its inline CLA on the shared
	// base. Hooks that only read their own attached policies can use
	// AttachedPolicyEndpointsMayApply. Fetch through the supplied
	// base-translation HandlerContext to register dependencies that invalidate
	// this decision.
	MayApply func(kctx krt.HandlerContext, backend ir.BackendObjectIR) bool
}

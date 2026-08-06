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
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/statussync"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

// ErrNotFound is returned when a requested resource is not found
var ErrNotFound = errors.New("not found")

type (
	EndpointsInputs = endpoints.EndpointsInputs
	ProcessBackend  func(ctx context.Context, pol ir.PolicyIR, in ir.BackendObjectIR, out *envoyclusterv3.Cluster)
	EndpointPlugin  func(
		kctx krt.HandlerContext,
		ctx context.Context,
		ucc ir.UniquelyConnectedClient,
		out *EndpointsInputs,
	) uint64
)

// TODO: consider changing PerClientProcessBackend to look like this:
// PerClientProcessBackend  func(kctx krt.HandlerContext, ctx context.Context, ucc ir.UniquelyConnectedClient, in ir.BackendObjectIR)
// so that it only attaches the policy to the backend, and doesn't modify the backend (except for attached policies) or the cluster itself.
// leaving as is for now as this requires better understanding of how krt would handle this.
type PerClientProcessBackend func(
	kctx krt.HandlerContext,
	ctx context.Context,
	ucc ir.UniquelyConnectedClient,
	in ir.BackendObjectIR,
	out *envoyclusterv3.Cluster,
)

// PolicyStatusInputs is provided to a PolicyPlugin's RegisterPolicyStatus hook. The plugin
// registers its raw collection, keyed report reducer, and just-in-time writer.
type PolicyStatusInputs struct {
	// Ctx is the controller's root context. It is captured by the plugin's
	// desired-status builders, which run for the lifetime of the process.
	Ctx context.Context
	// Collections is where the plugin registers its raw policy collection with
	// statussync.RegisterResource and its report reducer with
	// statussync.RegisterResourceReports.
	Collections *StatusCollections
	// StatusContributions contains Gateway- and Backend-produced facts keyed by status owner.
	StatusContributions krt.Collection[reports.StatusContribution]
	// ContributionsByTarget selects all contributions for one status owner.
	ContributionsByTarget krt.Index[reports.StatusKey, reports.StatusContribution]
	// KrtOpts supplies standard collection lifecycle and debugging options.
	KrtOpts krtutil.KrtOptions
	// NotReady re-queues policies whose client cannot see them yet. Plugins reading through
	// a delayed client must pass it to their writer, or a policy enqueued before that
	// client's informer loads silently never gets status.
	NotReady *statussync.NotReadyRequeuer
	// RegisterWriter registers the writer that persists this plugin's policy status.
	RegisterWriter func(gvk schema.GroupVersionKind, syncer statussync.ResourceStatusSyncer)
}

// StatusCollections aliases the statussync type for plugin convenience.
type StatusCollections = statussync.StatusCollections

type PolicyPlugin struct {
	Name                      string
	NewGatewayTranslationPass func(tctx ir.GwTranslationCtx, reporter reporter.Reporter) ir.ProxyTranslationPass

	// Backend processing for envoy proxy
	ProcessBackend            ProcessBackend
	PerClientProcessBackend   PerClientProcessBackend
	PerClientProcessEndpoints EndpointPlugin

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

	// PolicyStatusFromGatewayReports indicates that policy status should be reported from the
	// Gateway translation report path rather than the backend-only report path.
	PolicyStatusFromGatewayReports bool
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

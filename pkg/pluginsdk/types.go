package pluginsdk

import (
	"context"
	"encoding/json"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/endpoints"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
)

type (
	EndpointsInputs = endpoints.EndpointsInputs
	ProcessBackend  func(ctx context.Context, pol ir.PolicyIR, in ir.BackendObjectIR, out *envoyclusterv3.Cluster)
	EndpointPlugin  func(
		kctx krt.HandlerContext,
		ctx context.Context,
		ucc ir.UniqlyConnectedClient,
		out *EndpointsInputs,
	) uint64
)

// TODO: consider changing PerClientProcessBackend to look like this:
// PerClientProcessBackend  func(kctx krt.HandlerContext, ctx context.Context, ucc ir.UniqlyConnectedClient, in ir.BackendObjectIR)
// so that it only attaches the policy to the backend, and doesn't modify the backend (except for attached policies) or the cluster itself.
// leaving as is for now as this requires better understanding of how krt would handle this.
type PerClientProcessBackend func(
	kctx krt.HandlerContext,
	ctx context.Context,
	ucc ir.UniqlyConnectedClient,
	in ir.BackendObjectIR,
	out *envoyclusterv3.Cluster,
)

type (
	// GetPolicyStatusFn is a type that plugins can implement to get the PolicyStatus for the given policy
	GetPolicyStatusFn func(context.Context, types.NamespacedName) (gwv1alpha2.PolicyStatus, error)
	// PatchPolicyStatusFn is a type that plugins can implement to patch the PolicyStatus for the given policy
	PatchPolicyStatusFn func(context.Context, types.NamespacedName, gwv1alpha2.PolicyStatus) error
)

type PolicyPlugin struct {
	Name                      string
	NewGatewayTranslationPass func(tctx ir.GwTranslationCtx, reporter reporter.Reporter) ir.ProxyTranslationPass

	// Backend processing for envoy proxy
	ProcessBackend            ProcessBackend
	PerClientProcessBackend   PerClientProcessBackend
	PerClientProcessEndpoints EndpointPlugin

	// Backend processing for agent gateway
	ProcessAgentBackend func(pol ir.PolicyIR, in ir.BackendObjectIR) error

	Policies       krt.Collection[ir.PolicyWrapper]
	GlobalPolicies func(krt.HandlerContext) ir.PolicyIR
	// PoliciesFetch can optionally be set if the plugin needs a custom mechanism for fetching the policy IR,
	// rather than the default behavior of fetching by name from the aggregated policy KRT collection
	PoliciesFetch func(n, ns string) ir.PolicyIR
	MergePolicies func(pols []ir.PolicyAtt) ir.PolicyAtt

	GetPolicyStatus   GetPolicyStatusFn
	PatchPolicyStatus PatchPolicyStatusFn
}

type BackendPlugin struct {
	ir.BackendInit
	AliasKinds []schema.GroupKind
	Backends   krt.Collection[ir.BackendObjectIR]
	Endpoints  krt.Collection[ir.EndpointsForBackend]
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

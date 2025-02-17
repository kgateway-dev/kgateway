package extensionsplug

import (
	"context"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"

	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/reports"
)

type AttachmentPoints uint

const (
	UpstreamAttachmentPoint AttachmentPoints = 1 << iota
	GatewayAttachmentPoint
	RouteAttachmentPoint
)

func (a AttachmentPoints) Has(p AttachmentPoints) bool {
	return a&p != 0
}

type EndpointPlugin func(kctx krt.HandlerContext, ctx context.Context, ucc ir.UniqlyConnectedClient, in ir.EndpointsForUpstream) (*envoy_config_endpoint_v3.ClusterLoadAssignment, uint64)
type GetBackendForRefPlugin func(kctx krt.HandlerContext, key ir.ObjectSource, port int32) *ir.Upstream

type PolicyPlugin struct {
	Name                      string
	NewGatewayTranslationPass func(ctx context.Context, tctx ir.GwTranslationCtx) ir.ProxyTranslationPass

	GetBackendForRef GetBackendForRefPlugin
	ProcessUpstream  func(ctx context.Context, pol ir.PolicyIR, in ir.Upstream, out *envoy_config_cluster_v3.Cluster)
	// TODO: consider changing PerClientProcessUpstream too look like this:
	// PerClientProcessUpstream  func(kctx krt.HandlerContext, ctx context.Context, ucc ir.UniqlyConnectedClient, in ir.Upstream)
	// so that it only attaches the policy to the upstream, and doesn't modify the upstream (except for attached policies) or the cluster itself.
	// leaving as is for now as this requires better understanding of how krt would handle this.
	PerClientProcessUpstream  func(kctx krt.HandlerContext, ctx context.Context, ucc ir.UniqlyConnectedClient, in ir.Upstream, out *envoy_config_cluster_v3.Cluster)
	PerClientProcessEndpoints EndpointPlugin

	Policies       krt.Collection[ir.PolicyWrapper]
	GlobalPolicies func(krt.HandlerContext, AttachmentPoints) ir.PolicyIR
	PoliciesFetch  func(n, ns string) ir.PolicyIR
}

type UpstreamPlugin struct {
	ir.UpstreamInit
	Upstreams krt.Collection[ir.Upstream]
	Endpoints krt.Collection[ir.EndpointsForUpstream]
}

type KGwTranslator interface {
	// This function is called by the reconciler when a K8s Gateway resource is created or updated.
	// It returns an instance of the kgateway Proxy resource, that should configure a target kgateway Proxy workload.
	// A null return value indicates the K8s Gateway resource failed to translate into a kgateway Proxy. The error will be reported on the provided reporter.
	Translate(kctx krt.HandlerContext,
		ctx context.Context,
		gateway *ir.Gateway,
		reporter reports.Reporter) *ir.GatewayIR
}
type GwTranslatorFactory func(gw *gwv1.Gateway) KGwTranslator
type ContributesPolicies map[schema.GroupKind]PolicyPlugin

type Plugin struct {
	ContributesPolicies
	ContributesUpstreams    map[schema.GroupKind]UpstreamPlugin
	ContributesGwTranslator GwTranslatorFactory
	// extra has sync beyong primary resources in the collections above
	ExtraHasSynced func() bool
}

func (p PolicyPlugin) AttachmentPoints() AttachmentPoints {
	var ret AttachmentPoints
	if p.ProcessUpstream != nil {
		ret = ret | UpstreamAttachmentPoint
	}
	if p.NewGatewayTranslationPass != nil {
		ret = ret | GatewayAttachmentPoint
	}
	return ret
}

func (p Plugin) HasSynced() bool {
	for _, up := range p.ContributesUpstreams {
		if up.Upstreams != nil && !up.Upstreams.Synced().HasSynced() {
			return false
		}
		if up.Endpoints != nil && !up.Endpoints.Synced().HasSynced() {
			return false
		}
	}
	for _, pol := range p.ContributesPolicies {
		if pol.Policies != nil && !pol.Policies.Synced().HasSynced() {
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

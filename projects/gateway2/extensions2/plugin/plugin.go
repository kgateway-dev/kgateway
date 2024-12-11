package extensionsplug

import (
	"context"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	"github.com/solo-io/gloo/projects/gateway2/ir"
	"github.com/solo-io/gloo/projects/gateway2/krtcollections"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/solo-io/gloo/projects/gateway2/reports"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type EndpointPlugin func(kctx krt.HandlerContext, ctx context.Context, ucc krtcollections.UniqlyConnectedClient, in krtcollections.EndpointsForUpstream) (*envoy_config_endpoint_v3.ClusterLoadAssignment, uint64)

type PolicyPlugin struct {
	Name                      string
	NewGatewayTranslationPass func(ctx context.Context, tctx ir.GwTranslationCtx) ir.ProxyTranslationPass
	ProcessUpstream           func(ctx context.Context, pol ir.PolicyIR, in ir.Upstream, out *envoy_config_cluster_v3.Cluster)
	PerClientProcessUpstream  func(kctx krt.HandlerContext, ctx context.Context, ucc krtcollections.UniqlyConnectedClient, in ir.Upstream, out *envoy_config_cluster_v3.Cluster)
	PerClientProcessEndpoints EndpointPlugin

	Policies      krt.Collection[ir.PolicyWrapper]
	PoliciesFetch func(n, ns string) ir.PolicyIR
}

type UpstreamPlugin struct {
	ir.UpstreamInit
	Upstreams krt.Collection[ir.Upstream]
	Endpoints krt.Collection[krtcollections.EndpointsForUpstream]
}

type K8sGwTranslator interface {
	// This function is called by the reconciler when a K8s Gateway resource is created or updated.
	// It returns an instance of the k8sgateway Proxy resource, that should configure a target k8sgateway Proxy workload.
	// A null return value indicates the K8s Gateway resource failed to translate into a k8sgateway Proxy. The error will be reported on the provided reporter.
	Translate(kctx krt.HandlerContext,
		ctx context.Context,
		gateway *ir.Gateway,
		reporter reports.Reporter) *ir.GatewayIR
}
type GwTranslatorFactory func(gw *gwv1.Gateway) K8sGwTranslator
type Plugin struct {
	ContributesPolicies     map[schema.GroupKind]PolicyPlugin
	ContributesUpstreams    map[schema.GroupKind]UpstreamPlugin
	ContributesGwTranslator GwTranslatorFactory
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
	return true
}

type K8sGatewayExtensions2 struct {
	Plugins []Plugin
}

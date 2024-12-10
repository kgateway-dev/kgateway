package extensionsplug

import (
	"context"

	"github.com/solo-io/gloo/projects/gateway2/ir"
	"github.com/solo-io/gloo/projects/gateway2/krtcollections"
	"github.com/solo-io/gloo/projects/gateway2/reports"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type UpstreamImpl struct {
	ir.UpstreamInit
	Upstreams krt.Collection[ir.Upstream]
	Endpoints krt.Collection[krtcollections.EndpointsForUpstream]
}

type Plugin struct {
	ContributesPolicies  map[schema.GroupKind]ir.PolicyImpl
	ContributesUpstreams map[schema.GroupKind]UpstreamImpl
	ContributesGwClasses map[string]interface {
		// TranslateProxy This function is called by the reconciler when a K8s Gateway resource is created or updated.
		// It returns an instance of the k8sgateway Proxy resource, that should configure a target k8sgateway Proxy workload.
		// A null return value indicates the K8s Gateway resource failed to translate into a k8sgateway Proxy. The error will be reported on the provided reporter.
		TranslateProxy(
			ctx context.Context,
			gateway *gwv1.Gateway,
			writeNamespace string,
			reporter reports.Reporter,
		) *ir.GatewayIR
	}
}

type K8sGatewayExtensions2 struct {
	Plugins []Plugin
}

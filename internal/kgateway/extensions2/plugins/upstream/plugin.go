package upstream

import (
	"bytes"
	"context"
	"maps"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_hcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	awspb "github.com/solo-io/envoy-gloo/go/config/filter/http/aws_lambda/v2"
	skubeclient "istio.io/istio/pkg/config/schema/kubeclient"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	extensionsplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/plugins"
	"github.com/kgateway-dev/kgateway/v2/pkg/client/clientset/versioned"
)

const (
	ParameterGroup = "gloo.solo.io"
	ParameterKind  = "Parameter"
)
const (
	ExtensionName = "Upstream"
	FilterName    = "io.solo.aws_lambda"
)

var (
	ParameterGK = schema.GroupKind{
		Group: ParameterGroup,
		Kind:  ParameterKind,
	}
)

type upstreamDestination struct {
	FunctionName string
}

func (d *upstreamDestination) CreationTime() time.Time {
	return time.Time{}
}

func (d *upstreamDestination) Equals(in any) bool {
	d2, ok := in.(*upstreamDestination)
	if !ok {
		return false
	}
	return d.FunctionName == d2.FunctionName
}

type UpstreamIr struct {
	AwsSecret *ir.Secret
}

func (u *UpstreamIr) data() map[string][]byte {
	if u.AwsSecret == nil {
		return nil
	}
	return u.AwsSecret.Data
}

func (u *UpstreamIr) Equals(other any) bool {
	otherUpstream, ok := other.(*UpstreamIr)
	if !ok {
		return false
	}
	return maps.EqualFunc(u.data(), otherUpstream.data(), func(a, b []byte) bool {
		return bytes.Equal(a, b)
	})
}

type upstreamPlugin struct {
	needFilter map[string]bool
}

func registerTypes(ourCli versioned.Interface) {
	skubeclient.Register[*v1alpha1.Upstream](
		v1alpha1.UpstreamGVK.GroupVersion().WithResource("upstreams"),
		v1alpha1.UpstreamGVK,
		func(c skubeclient.ClientGetter, namespace string, o metav1.ListOptions) (runtime.Object, error) {
			return ourCli.GatewayV1alpha1().Upstreams(namespace).List(context.Background(), o)
		},
		func(c skubeclient.ClientGetter, namespace string, o metav1.ListOptions) (watch.Interface, error) {
			return ourCli.GatewayV1alpha1().Upstreams(namespace).Watch(context.Background(), o)
		},
	)
}

func NewPlugin(ctx context.Context, commoncol *common.CommonCollections) extensionsplug.Plugin {

	registerTypes(commoncol.OurClient)

	col := krt.WrapClient(kclient.New[*v1alpha1.Upstream](commoncol.Client), commoncol.KrtOpts.ToOptions("Upstreams")...)

	gk := v1alpha1.UpstreamGVK.GroupKind()
	translate := buildTranslateFunc(commoncol.Secrets)
	ucol := krt.NewCollection(col, func(krtctx krt.HandlerContext, i *v1alpha1.Upstream) *ir.Upstream {

		// resolve secrets

		return &ir.Upstream{
			ObjectSource: ir.ObjectSource{
				Kind:      gk.Kind,
				Group:     gk.Group,
				Namespace: i.GetNamespace(),
				Name:      i.GetName(),
			},
			GvPrefix:          "upstream",
			CanonicalHostname: hostname(i),
			Obj:               i,
			ObjIr:             translate(krtctx, i),
		}
	})

	endpoints := krt.NewCollection(col, func(krtctx krt.HandlerContext, i *v1alpha1.Upstream) *ir.EndpointsForUpstream {
		return processEndpoints(i)
	})
	return extensionsplug.Plugin{
		ContributesUpstreams: map[schema.GroupKind]extensionsplug.UpstreamPlugin{
			gk: {
				UpstreamInit: ir.UpstreamInit{
					InitUpstream: processUpstream,
				},
				Endpoints: endpoints,
				Upstreams: ucol,
			},
		},
		ContributesPolicies: map[schema.GroupKind]extensionsplug.PolicyPlugin{
			ParameterGK: {
				Name:                      "upstream",
				NewGatewayTranslationPass: newPlug,
				//			AttachmentPoints: []ir.AttachmentPoints{ir.HttpBackendRefAttachmentPoint},
				PoliciesFetch: func(n, ns string) ir.PolicyIR {
					// virtual policy - we don't have a real policy object
					return &upstreamDestination{
						FunctionName: n,
					}
				},
			},
		},
	}
}

func buildTranslateFunc(secrets *krtcollections.SecretIndex) func(krtctx krt.HandlerContext, i *v1alpha1.Upstream) *UpstreamIr {
	return func(krtctx krt.HandlerContext, i *v1alpha1.Upstream) *UpstreamIr {
		// resolve secrets
		var ir UpstreamIr
		if i.Spec.Aws != nil {
			ns := i.GetNamespace()
			secretRef := gwv1.SecretObjectReference{
				Name: gwv1.ObjectName(i.Spec.Aws.SecretRef.Name),
			}
			secret, _ := secrets.GetSecret(krtctx, krtcollections.From{GroupKind: v1alpha1.UpstreamGVK.GroupKind(), Namespace: ns}, secretRef)
			if secret != nil {
				ir.AwsSecret = secret
			} else {
				// TODO: handle error and write it to status
				// return error
			}
		}
		return &ir
	}
}

func processUpstream(ctx context.Context, in ir.Upstream, out *envoy_config_cluster_v3.Cluster) {
	up, ok := in.Obj.(*v1alpha1.Upstream)
	if !ok {
		// log - should never happen
		return
	}

	ir, ok := in.ObjIr.(*UpstreamIr)
	if !ok {
		// log - should never happen
		return
	}

	spec := up.Spec

	switch {
	case spec.Static != nil:
		processStatic(ctx, spec.Static, out)
	case spec.Aws != nil:
		processAws(ctx, spec.Aws, ir, out)
	}
}

func hostname(in *v1alpha1.Upstream) string {
	if in.Spec.Static != nil {
		if len(in.Spec.Static.Hosts) > 0 {
			return string(in.Spec.Static.Hosts[0].Host)
		}
	}
	return ""
}

func processEndpoints(up *v1alpha1.Upstream) *ir.EndpointsForUpstream {

	spec := up.Spec

	switch {
	case spec.Static != nil:
		return processEndpointsStatic(spec.Static)
	case spec.Aws != nil:
		return processEndpointsAws(spec.Aws)
	}
	return nil
}

func newPlug(ctx context.Context, tctx ir.GwTranslationCtx) ir.ProxyTranslationPass {
	return &upstreamPlugin{}
}

func (p *upstreamPlugin) Name() string {
	return "upstream"
}

// called 1 time for each listener
func (p *upstreamPlugin) ApplyListenerPlugin(ctx context.Context, pCtx *ir.ListenerContext, out *envoy_config_listener_v3.Listener) {
}

func (p *upstreamPlugin) ApplyHCM(ctx context.Context,
	pCtx *ir.HcmContext,
	out *envoy_hcm.HttpConnectionManager) error { //no-op
	return nil
}

func (p *upstreamPlugin) ApplyVhostPlugin(ctx context.Context, pCtx *ir.VirtualHostContext, out *envoy_config_route_v3.VirtualHost) {
}

// called 0 or more times
func (p *upstreamPlugin) ApplyForRoute(ctx context.Context, pCtx *ir.RouteContext, outputRoute *envoy_config_route_v3.Route) error {

	return nil
}

func (p *upstreamPlugin) ApplyForRouteBackend(
	ctx context.Context, policy ir.PolicyIR,
	pCtx *ir.RouteBackendContext,
) error {
	pol, ok := policy.(*upstreamDestination)
	if !ok {
		return nil
		// todo: should we return fmt.Errorf("internal error: policy is not a upstreamDestination")
	}
	return p.processBackendAws(ctx, pCtx, pol)
}

// called 1 time per listener
// if a plugin emits new filters, they must be with a plugin unique name.
// any filter returned from route config must be disabled, so it doesnt impact other routes.
func (p *upstreamPlugin) HttpFilters(ctx context.Context, fc ir.FilterChainCommon) ([]plugins.StagedHttpFilter, error) {
	if !p.needFilter[fc.FilterChainName] {
		return nil, nil
	}
	filterConfig := &awspb.AWSLambdaConfig{}
	pluginStage := plugins.DuringStage(plugins.OutAuthStage)
	f, _ := plugins.NewStagedFilter(FilterName, filterConfig, pluginStage)

	return []plugins.StagedHttpFilter{
		f,
	}, nil
}

func (p *upstreamPlugin) UpstreamHttpFilters(ctx context.Context) ([]plugins.StagedUpstreamHttpFilter, error) {
	return nil, nil
}

func (p *upstreamPlugin) NetworkFilters(ctx context.Context) ([]plugins.StagedNetworkFilter, error) {
	return nil, nil
}

// called 1 time (per envoy proxy). replaces GeneratedResources
func (p *upstreamPlugin) ResourcesToAdd(ctx context.Context) ir.Resources {
	return ir.Resources{}
}

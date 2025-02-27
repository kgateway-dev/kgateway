package backend

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
	"istio.io/istio/pkg/config/schema/kubeclient"
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
	ExtensionName = "Backend"
	FilterName    = "io.solo.aws_lambda"
)

var (
	ParameterGK = schema.GroupKind{
		Group: ParameterGroup,
		Kind:  ParameterKind,
	}
)

type backendDestination struct {
	FunctionName string
}

func (d *backendDestination) CreationTime() time.Time {
	return time.Time{}
}

func (d *backendDestination) Equals(in any) bool {
	d2, ok := in.(*backendDestination)
	if !ok {
		return false
	}
	return d.FunctionName == d2.FunctionName
}

type BackendIr struct {
	AwsSecret *ir.Secret
}

func (u *BackendIr) data() map[string][]byte {
	if u.AwsSecret == nil {
		return nil
	}
	return u.AwsSecret.Data
}

func (u *BackendIr) Equals(other any) bool {
	otherUpstream, ok := other.(*BackendIr)
	if !ok {
		return false
	}
	return maps.EqualFunc(u.data(), otherUpstream.data(), func(a, b []byte) bool {
		return bytes.Equal(a, b)
	})
}

type backendPlugin struct {
	needFilter map[string]bool
}

func registerTypes(ourCli versioned.Interface) {
	kubeclient.Register[*v1alpha1.Backend](
		v1alpha1.BackendGVK.GroupVersion().WithResource("backends"),
		v1alpha1.BackendGVK,
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (runtime.Object, error) {
			return ourCli.GatewayV1alpha1().Backends(namespace).List(context.Background(), o)
		},
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (watch.Interface, error) {
			return ourCli.GatewayV1alpha1().Backends(namespace).Watch(context.Background(), o)
		},
	)
}

func NewPlugin(ctx context.Context, commoncol *common.CommonCollections) extensionsplug.Plugin {
	registerTypes(commoncol.OurClient)

	col := krt.WrapClient(kclient.New[*v1alpha1.Backend](commoncol.Client), commoncol.KrtOpts.ToOptions("Backends")...)

	gk := v1alpha1.BackendGVK.GroupKind()
	translate := buildTranslateFunc(commoncol.Secrets)
	ucol := krt.NewCollection(col, func(krtctx krt.HandlerContext, i *v1alpha1.Backend) *ir.BackendObjectIR {
		// resolve secrets
		return &ir.BackendObjectIR{
			ObjectSource: ir.ObjectSource{
				Kind:      gk.Kind,
				Group:     gk.Group,
				Namespace: i.GetNamespace(),
				Name:      i.GetName(),
			},
			GvPrefix:          "backend",
			CanonicalHostname: hostname(i),
			Obj:               i,
			ObjIr:             translate(krtctx, i),
		}
	})

	endpoints := krt.NewCollection(col, func(krtctx krt.HandlerContext, i *v1alpha1.Backend) *ir.EndpointsForBackend {
		return processEndpoints(i)
	})
	return extensionsplug.Plugin{
		ContributesBackends: map[schema.GroupKind]extensionsplug.BackendPlugin{
			gk: {
				BackendInit: ir.BackendInit{
					InitBackend: processUpstream,
				},
				Endpoints: endpoints,
				Backends:  ucol,
			},
		},
		ContributesPolicies: map[schema.GroupKind]extensionsplug.PolicyPlugin{
			ParameterGK: {
				Name:                      "backend",
				NewGatewayTranslationPass: newPlug,
				//			AttachmentPoints: []ir.AttachmentPoints{ir.HttpBackendRefAttachmentPoint},
				PoliciesFetch: func(n, ns string) ir.PolicyIR {
					// virtual policy - we don't have a real policy object
					return &backendDestination{
						FunctionName: n,
					}
				},
			},
		},
	}
}

func buildTranslateFunc(secrets *krtcollections.SecretIndex) func(krtctx krt.HandlerContext, i *v1alpha1.Backend) *BackendIr {
	return func(krtctx krt.HandlerContext, i *v1alpha1.Backend) *BackendIr {
		// resolve secrets
		var ir BackendIr
		if i.Spec.Aws != nil {
			ns := i.GetNamespace()
			secretRef := gwv1.SecretObjectReference{
				Name: gwv1.ObjectName(i.Spec.Aws.SecretRef.Name),
			}
			secret, _ := secrets.GetSecret(krtctx, krtcollections.From{GroupKind: v1alpha1.BackendGVK.GroupKind(), Namespace: ns}, secretRef)
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

func processUpstream(ctx context.Context, in ir.BackendObjectIR, out *envoy_config_cluster_v3.Cluster) {
	up, ok := in.Obj.(*v1alpha1.Backend)
	if !ok {
		// log - should never happen
		return
	}

	ir, ok := in.ObjIr.(*BackendIr)
	if !ok {
		// log - should never happen
		return
	}

	spec := up.Spec
	switch {
	case spec.Type == v1alpha1.BackendTypeStatic:
		processStatic(ctx, spec.Static, out)
	case spec.Type == v1alpha1.BackendTypeAWS:
		processAws(ctx, spec.Aws, ir, out)
	}
}

func hostname(in *v1alpha1.Backend) string {
	if in.Spec.Type != v1alpha1.BackendTypeStatic {
		return ""
	}
	if in.Spec.Static != nil {
		if len(in.Spec.Static.Hosts) > 0 {
			return string(in.Spec.Static.Hosts[0].Host)
		}
	}
	return ""
}

func processEndpoints(up *v1alpha1.Backend) *ir.EndpointsForBackend {
	spec := up.Spec
	switch {
	case spec.Type == v1alpha1.BackendTypeStatic:
		return processEndpointsStatic(spec.Static)
	case spec.Type == v1alpha1.BackendTypeAWS:
		return processEndpointsAws(spec.Aws)
	}
	return nil
}

func newPlug(ctx context.Context, tctx ir.GwTranslationCtx) ir.ProxyTranslationPass {
	return &backendPlugin{}
}

func (p *backendPlugin) Name() string {
	return ExtensionName
}

// called 1 time for each listener
func (p *backendPlugin) ApplyListenerPlugin(ctx context.Context, pCtx *ir.ListenerContext, out *envoy_config_listener_v3.Listener) {
}

func (p *backendPlugin) ApplyHCM(ctx context.Context,
	pCtx *ir.HcmContext,
	out *envoy_hcm.HttpConnectionManager) error { //no-op
	return nil
}

func (p *backendPlugin) ApplyVhostPlugin(ctx context.Context, pCtx *ir.VirtualHostContext, out *envoy_config_route_v3.VirtualHost) {
}

// called 0 or more times
func (p *backendPlugin) ApplyForRoute(ctx context.Context, pCtx *ir.RouteContext, outputRoute *envoy_config_route_v3.Route) error {
	return nil
}

func (p *backendPlugin) ApplyForRouteBackend(
	ctx context.Context, policy ir.PolicyIR,
	pCtx *ir.RouteBackendContext,
) error {
	pol, ok := policy.(*backendDestination)
	if !ok {
		return nil
		// todo: should we return fmt.Errorf("internal error: policy is not a upstreamDestination")
	}
	return p.processBackendAws(ctx, pCtx, pol)
}

// called 1 time per listener
// if a plugin emits new filters, they must be with a plugin unique name.
// any filter returned from route config must be disabled, so it doesnt impact other routes.
func (p *backendPlugin) HttpFilters(ctx context.Context, fc ir.FilterChainCommon) ([]plugins.StagedHttpFilter, error) {
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

func (p *backendPlugin) UpstreamHttpFilters(ctx context.Context) ([]plugins.StagedUpstreamHttpFilter, error) {
	return nil, nil
}

func (p *backendPlugin) NetworkFilters(ctx context.Context) ([]plugins.StagedNetworkFilter, error) {
	return nil, nil
}

// called 1 time (per envoy proxy). replaces GeneratedResources
func (p *backendPlugin) ResourcesToAdd(ctx context.Context) ir.Resources {
	return ir.Resources{}
}

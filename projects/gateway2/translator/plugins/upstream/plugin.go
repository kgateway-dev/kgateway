package upstream

import (
	"bytes"
	"context"
	"fmt"
	"maps"
	"net/http"

	awspb "github.com/solo-io/gloo/projects/controller/pkg/api/external/envoy/extensions/aws"
	"github.com/solo-io/go-utils/contextutils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/solo-io/gloo/projects/controller/pkg/plugins"
	"github.com/solo-io/gloo/projects/gateway2/api/v1alpha1"
	extensions "github.com/solo-io/gloo/projects/gateway2/extensions2"
	"github.com/solo-io/gloo/projects/gateway2/krtcollections"
	"github.com/solo-io/gloo/projects/gateway2/model"
	"github.com/solo-io/gloo/projects/gateway2/reports"
	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/kube/kubetypes"
	corev1 "k8s.io/api/core/v1"
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

type UpstreamDestination struct {
	metav1.TypeMeta
	metav1.ObjectMeta

	FunctionName string
}
type UpstreamIr struct {
	AwsSecret *corev1.Secret
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

type plugin2 struct {
	needFilter map[string]bool
}

func NewPlugin2(ctx context.Context, istioClient kube.Client, secrets krt.Collection[*corev1.Secret], dbg *krt.DebugHandler) *extensions.Plugin {

	col := SetupCollectionDynamic[v1alpha1.Upstream](
		ctx,
		istioClient,
		v1alpha1.GroupVersion.WithResource("upstreams"),
		krt.WithName("Upstreams"), krt.WithDebugging(dbg),
	)
	gk := v1alpha1.UpstreamGVK.GroupKind()
	translate := buildTranslateFunc(secrets)
	ucol := krt.NewCollection(col, func(krtctx krt.HandlerContext, i *v1alpha1.Upstream) *model.Upstream {

		// resolve secrets

		return &model.Upstream{
			ObjectSource: model.ObjectSource{
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

	epndpoints := krt.NewCollection(col, func(krtctx krt.HandlerContext, i *v1alpha1.Upstream) *krtcollections.EndpointsForUpstream {
		return processEndpoints(i)
	})
	return &extensions.Plugin{
		ContributesUpstreams: map[schema.GroupKind]extensions.UpstreamImpl{
			gk: {
				Endpoints:       epndpoints,
				ProcessUpstream: processUpstream,
				Upstreams:       ucol,
			},
		},
		ContributesPolicies: map[schema.GroupKind]extensions.PolicyImpl{
			ParameterGK: {
				AttachmentPoints: []model.AttachmentPoints{model.HttpBackendRefAttachmentPoint},
				PoliciesFetch: func(n, ns string) model.Policy {
					// virtual policy - we don't have a real policy object
					return model.PolicyWrapper{
						Policy: &UpstreamDestination{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: ns,
								Name:      n,
							},
							FunctionName: n,
						},
					}
				},
			},
		},
	}
}

func buildTranslateFunc(secrets krt.Collection[*corev1.Secret]) func(krtctx krt.HandlerContext, i *v1alpha1.Upstream) *UpstreamIr {
	return func(krtctx krt.HandlerContext, i *v1alpha1.Upstream) *UpstreamIr {
		// resolve secrets
		var ir UpstreamIr
		if i.Spec.Aws != nil {
			ns := i.GetNamespace()
			secret := krt.FetchOne(krtctx, secrets, krt.FilterObjectName(types.NamespacedName{Namespace: ns, Name: i.Spec.Aws.SecretRef.Name}))
			if secret != nil {
				ir.AwsSecret = *secret
			}
		}
		return &ir
	}
}

func processUpstream(ctx context.Context, in model.Upstream, out *envoy_config_cluster_v3.Cluster) {
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

func processEndpoints(up *v1alpha1.Upstream) *krtcollections.EndpointsForUpstream {

	spec := up.Spec

	switch {
	case spec.Static != nil:
		return processEndpointsStatic(spec.Static)
	case spec.Aws != nil:
		return processEndpointsAws(spec.Aws)
	}
	return nil
}

func newPlug(ctx context.Context, tctx extensions.GwTranslationCtx) extensions.ProxyTranslationPass {
	return &plugin2{}
}

func (p *plugin2) Name() string {
	return "directresponse"
}

// called 1 time for each listener
func (p *plugin2) ApplyListenerPlugin(ctx context.Context, pCtx *extensions.ListenerContext, out *envoy_config_listener_v3.Listener) {
}

func (p *plugin2) ApplyVhostPlugin(ctx context.Context, pCtx *extensions.VirtualHostContext, out *envoy_config_route_v3.VirtualHost) {
}

// called 0 or more times
func (p *plugin2) ApplyForRoute(ctx context.Context, pCtx *extensions.RouteContext, outputRoute *envoy_config_route_v3.Route) error {
	dr, ok := pCtx.Policy.(*v1alpha1.DirectResponse)
	if !ok {
		return fmt.Errorf("internal error: policy is not a DirectResponse")
	}

	// TODO: if we want to validate that only one applies, the context can contain all attached policies of
	// this GK.

	// at this point, we have a valid DR reference that we should apply to the route.
	if outputRoute.GetAction() != nil {
		// the output route already has an action, which is incompatible with the DirectResponse,
		// so we'll return an error. note: the direct response plugin runs after other route plugins
		// that modify the output route (e.g. the redirect plugin), so this should be a rare case.
		errMsg := fmt.Sprintf("DirectResponse cannot be applied to route with existing action: %T", outputRoute.GetAction())
		pCtx.Reporter.SetCondition(reports.RouteCondition{
			Type:    gwv1.RouteConditionAccepted,
			Status:  metav1.ConditionFalse,
			Reason:  gwv1.RouteReasonIncompatibleFilters,
			Message: errMsg,
		})
		outputRoute.Action = &envoy_config_route_v3.Route_DirectResponse{
			DirectResponse: &envoy_config_route_v3.DirectResponseAction{
				Status: http.StatusInternalServerError,
			},
		}
		return fmt.Errorf(errMsg)
	}

	outputRoute.Action = &envoy_config_route_v3.Route_DirectResponse{
		DirectResponse: &envoy_config_route_v3.DirectResponseAction{
			Status: dr.GetStatusCode(),
			Body: &corev3.DataSource{
				Specifier: &corev3.DataSource_InlineString{
					InlineString: dr.GetBody(),
				},
			},
		},
	}
	return nil
}

func (p *plugin2) ApplyForRouteBackend(
	ctx context.Context,
	pCtx *extensions.RouteBackendContext,
	policy metav1.Object,
) error {
	pol, ok := policy.(*UpstreamDestination)
	if !ok {
		return nil
		// todo: should we return fmt.Errorf("internal error: policy is not a UpstreamDestination")
	}
	return p.processBackendAws(ctx, pCtx, pol)
}

// called 1 time per listener
// if a plugin emits new filters, they must be with a plugin unique name.
// any filter returned from route config must be disabled, so it doesnt impact other routes.
func (p *plugin2) HttpFilters(ctx context.Context, fc model.FilterChainCommon) ([]plugins.StagedHttpFilter, error) {
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

func (p *plugin2) UpstreamHttpFilters(ctx context.Context) ([]plugins.StagedUpstreamHttpFilter, error) {
	return nil, nil
}

func (p *plugin2) NetworkFilters(ctx context.Context) ([]plugins.StagedNetworkFilter, error) {
	return nil, nil
}

// called 1 time (per envoy proxy). replaces GeneratedResources
func (p *plugin2) ResourcesToAdd(ctx context.Context) extensions.Resources {
	return extensions.Resources{}
}

// SetupCollectionDynamic uses the dynamic client to setup an informer for a resource
// and then uses an intermediate krt collection to type the unstructured resource.
// This is a temporary workaround until we update to the latest istio version and can
// uncomment the code below for registering types.
// HACK: we don't want to use this long term, but it's letting me push forward with deveopment
func SetupCollectionDynamic[T any](
	ctx context.Context,
	client kube.Client,
	gvr schema.GroupVersionResource,
	opts ...krt.CollectionOption,
) krt.Collection[*T] {
	logger := contextutils.LoggerFrom(ctx)
	logger.Infof("setting up dynamic collection for %s", gvr.String())
	delayedClient := kclient.NewDelayedInformer[*unstructured.Unstructured](client, gvr, kubetypes.DynamicInformer, kclient.Filter{})
	mapper := krt.WrapClient(delayedClient, opts...)
	return krt.NewCollection(mapper, func(krtctx krt.HandlerContext, i *unstructured.Unstructured) **T {
		var empty T
		out := &empty
		err := runtime.DefaultUnstructuredConverter.FromUnstructured(i.UnstructuredContent(), out)
		if err != nil {
			logger.DPanic("failed converting unstructured into %T: %v", empty, i)
			return nil
		}
		return &out
	})
}

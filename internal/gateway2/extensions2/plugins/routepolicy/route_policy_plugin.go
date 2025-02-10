package routepolicy

import (
	"context"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"k8s.io/apimachinery/pkg/runtime/schema"

	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/internal/gateway2/extensions2/common"
	extensionplug "github.com/kgateway-dev/kgateway/internal/gateway2/extensions2/plugin"
	extensionsplug "github.com/kgateway-dev/kgateway/internal/gateway2/extensions2/plugin"
	"github.com/kgateway-dev/kgateway/internal/gateway2/ir"
	"github.com/kgateway-dev/kgateway/internal/gateway2/plugins"
	"github.com/kgateway-dev/kgateway/internal/gateway2/utils"
	"github.com/kgateway-dev/kgateway/internal/gateway2/utils/krtutil"
	transformationpb "github.com/solo-io/envoy-gloo/go/config/filter/http/transformation/v2"
)

const filterStage = "route_policy_transformation"

var (
	pluginStage = plugins.AfterStage(plugins.AuthZStage)
)

type routeOptsPlugin struct {
	ct   time.Time
	spec routeSpecIr
}

type routeSpecIr struct {
	timeout   *durationpb.Duration
	transform *anypb.Any
	errors    []error
}

func (d *routeOptsPlugin) CreationTime() time.Time {
	return d.ct
}

func (d *routeOptsPlugin) Equals(in any) bool {
	d2, ok := in.(*routeOptsPlugin)
	if !ok {
		return false
	}

	if !proto.Equal(d.spec.timeout, d2.spec.timeout) {
		return false
	}
	if !proto.Equal(d.spec.transform, d2.spec.transform) {
		return false
	}

	return true
}

type routeOptsPluginGwPass struct {
	needFilter bool
}

func NewPlugin(ctx context.Context, commoncol *common.CommonCollections) extensionplug.Plugin {
	col := krtutil.SetupCollectionDynamic[v1alpha1.RoutePolicy](
		ctx,
		commoncol.Client,
		v1alpha1.SchemeGroupVersion.WithResource("routepolicies"),
		commoncol.KrtOpts.ToOptions("RoutePolicy")...,
	)
	gk := v1alpha1.RoutePolicyGVK.GroupKind()
	policyCol := krt.NewCollection(col, func(krtctx krt.HandlerContext, policyCR *v1alpha1.RoutePolicy) *ir.PolicyWrapper {
		var pol = &ir.PolicyWrapper{
			ObjectSource: ir.ObjectSource{
				Group:     gk.Group,
				Kind:      gk.Kind,
				Namespace: policyCR.Namespace,
				Name:      policyCR.Name,
			},
			Policy:     policyCR,
			PolicyIR:   &routeOptsPlugin{ct: policyCR.CreationTimestamp.Time, spec: toSpec(policyCR.Spec)},
			TargetRefs: convert(policyCR.Spec.TargetRef),
		}
		return pol
	})

	return extensionplug.Plugin{
		ContributesPolicies: map[schema.GroupKind]extensionsplug.PolicyPlugin{
			v1alpha1.RoutePolicyGVK.GroupKind(): {
				//AttachmentPoints: []ir.AttachmentPoints{ir.HttpAttachmentPoint},
				NewGatewayTranslationPass: NewGatewayTranslationPass,
				Policies:                  policyCol,
			},
		},
	}
}

func toSpec(spec v1alpha1.RoutePolicySpec) routeSpecIr {
	var ret routeSpecIr
	if spec.Timeout > 0 {
		ret.timeout = durationpb.New(time.Second * time.Duration(spec.Timeout))
	}
	var err error
	ret.transform, err = toTransformFilterConfig(&spec.Transformation)
	if err != nil {
		ret.errors = append(ret.errors, err)
	}

	return ret
}

func toTransform(t *v1alpha1.Transform) *transformationpb.Transformation_TransformationTemplate {

	hasTransform := false
	tt := &transformationpb.Transformation_TransformationTemplate{
		TransformationTemplate: &transformationpb.TransformationTemplate{
			Headers: map[string]*transformationpb.InjaTemplate{},
		},
	}
	for _, h := range t.Set {
		tt.TransformationTemplate.Headers[string(h.Name)] = &transformationpb.InjaTemplate{
			Text: string(h.Value),
		}
		hasTransform = true
	}

	for _, h := range t.Add {
		tt.TransformationTemplate.HeadersToAppend = append(tt.TransformationTemplate.HeadersToAppend, &transformationpb.TransformationTemplate_HeaderToAppend{
			Key: string(h.Name),
			Value: &transformationpb.InjaTemplate{
				Text: string(h.Value),
			},
		})
		hasTransform = true
	}

	tt.TransformationTemplate.HeadersToRemove = make([]string, 0, len(t.Remove))
	for _, h := range t.Remove {
		tt.TransformationTemplate.HeadersToRemove = append(tt.TransformationTemplate.HeadersToRemove, string(h))
		hasTransform = true
	}

	//BODY
	if t.Body == nil {
		tt.TransformationTemplate.BodyTransformation = &transformationpb.TransformationTemplate_Passthrough{
			Passthrough: &transformationpb.Passthrough{},
		}
	} else {
		if t.Body.ParseAs == v1alpha1.BodyParseBehaviorAsString {
			tt.TransformationTemplate.ParseBodyBehavior = transformationpb.TransformationTemplate_DontParse
		}
		if value := t.Body.Value; value != nil {
			hasTransform = true
			tt.TransformationTemplate.BodyTransformation = &transformationpb.TransformationTemplate_Body{
				Body: &transformationpb.InjaTemplate{
					Text: string(*value),
				},
			}
		}
	}

	if !hasTransform {
		return nil
	}
	return tt
}

func toTransformFilterConfig(t *v1alpha1.TransformationPolicy) (*anypb.Any, error) {
	if t == nil {
		return nil, nil
	}

	var reqt *transformationpb.Transformation
	var respt *transformationpb.Transformation

	if rtt := toTransform(t.Request); rtt != nil {
		reqt = &transformationpb.Transformation{
			TransformationType: rtt,
		}
	}
	if rtt := toTransform(t.Response); rtt != nil {
		respt = &transformationpb.Transformation{
			TransformationType: rtt,
		}
	}
	if reqt == nil && respt == nil {
		return nil, nil
	}
	reqm := &transformationpb.RouteTransformations_RouteTransformation_RequestMatch{
		RequestTransformation:  reqt,
		ResponseTransformation: respt,
	}

	envoyT := &transformationpb.RouteTransformations{
		Transformations: []*transformationpb.RouteTransformations_RouteTransformation{
			{
				Match: &transformationpb.RouteTransformations_RouteTransformation_RequestMatch_{
					RequestMatch: reqm,
				},
			},
		},
	}

	return utils.MessageToAny(envoyT)
}

func convert(targetRef v1alpha1.LocalPolicyTargetReference) []ir.PolicyTargetRef {
	return []ir.PolicyTargetRef{{
		Kind:  string(targetRef.Kind),
		Name:  string(targetRef.Name),
		Group: string(targetRef.Group),
	}}
}

func NewGatewayTranslationPass(ctx context.Context, tctx ir.GwTranslationCtx) ir.ProxyTranslationPass {
	return &routeOptsPluginGwPass{}
}
func (p *routeOptsPlugin) Name() string {
	return "routepolicies"
}

// called 1 time for each listener
func (p *routeOptsPluginGwPass) ApplyListenerPlugin(ctx context.Context, pCtx *ir.ListenerContext, out *envoy_config_listener_v3.Listener) {
}

func (p *routeOptsPluginGwPass) ApplyVhostPlugin(ctx context.Context, pCtx *ir.VirtualHostContext, out *envoy_config_route_v3.VirtualHost) {
}

// called 0 or more times
func (p *routeOptsPluginGwPass) ApplyForRoute(ctx context.Context, pCtx *ir.RouteContext, outputRoute *envoy_config_route_v3.Route) error {
	policy, ok := pCtx.Policy.(*routeOptsPlugin)
	if !ok {
		return nil
	}

	if policy.spec.timeout != nil && outputRoute.GetRoute() != nil {
		outputRoute.GetRoute().Timeout = policy.spec.timeout
	}

	if policy.spec.transform != nil {
		if outputRoute.TypedPerFilterConfig == nil {
			outputRoute.TypedPerFilterConfig = make(map[string]*anypb.Any)
		}
		outputRoute.TypedPerFilterConfig[filterStage] = policy.spec.transform
		p.needFilter = true
	}

	return nil
}

func (p *routeOptsPluginGwPass) ApplyForRouteBackend(
	ctx context.Context,
	policy ir.PolicyIR,
	pCtx *ir.RouteBackendContext,
) error {
	return nil
}

// called 1 time per listener
// if a plugin emits new filters, they must be with a plugin unique name.
// any filter returned from route config must be disabled, so it doesnt impact other routes.
func (p *routeOptsPluginGwPass) HttpFilters(ctx context.Context, fcc ir.FilterChainCommon) ([]plugins.StagedHttpFilter, error) {

	if p.needFilter {
		return []plugins.StagedHttpFilter{
			plugins.MustNewStagedFilter(filterStage,
				&transformationpb.FilterTransformations{},
				pluginStage),
		}, nil
	}

	return nil, nil
}

func (p *routeOptsPluginGwPass) UpstreamHttpFilters(ctx context.Context) ([]plugins.StagedUpstreamHttpFilter, error) {
	return nil, nil
}

func (p *routeOptsPluginGwPass) NetworkFilters(ctx context.Context) ([]plugins.StagedNetworkFilter, error) {
	return nil, nil
}

// called 1 time (per envoy proxy). replaces GeneratedResources
func (p *routeOptsPluginGwPass) ResourcesToAdd(ctx context.Context) ir.Resources {
	return ir.Resources{}
}

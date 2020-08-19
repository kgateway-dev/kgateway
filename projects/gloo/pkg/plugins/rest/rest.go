package rest

/*
if this destination spec has rest service spec
this will grab the parameters from the route extension
*/
import (
	"context"

	"github.com/solo-io/gloo/projects/gloo/pkg/upstreams"

	"github.com/gogo/protobuf/proto"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/errors"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"

	transformapi "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/transformation"
	glooplugins "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/pluginutils"
	transformutils "github.com/solo-io/gloo/projects/gloo/pkg/plugins/utils/transformation"
	"github.com/solo-io/go-utils/contextutils"

	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/transformation"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

type UpstreamWithServiceSpec interface {
	GetServiceSpec() *glooplugins.ServiceSpec
}

type plugin struct {
	transformsAdded   *bool
	recordedUpstreams map[core.ResourceRef]*glooplugins.ServiceSpec_Rest
	ctx               context.Context
}

func NewPlugin(transformsAdded *bool) plugins.Plugin {
	return &plugin{transformsAdded: transformsAdded}
}

func (p *plugin) Init(params plugins.InitParams) error {
	p.ctx = params.Ctx
	p.recordedUpstreams = make(map[core.ResourceRef]*glooplugins.ServiceSpec_Rest)
	return nil
}

func (p *plugin) ProcessUpstream(params plugins.Params, in *v1.Upstream, _ *envoyapi.Cluster) error {
	if withServiceSpec, ok := in.UpstreamType.(UpstreamWithServiceSpec); ok {
		serviceSpec := withServiceSpec.GetServiceSpec()
		if serviceSpec == nil {
			return nil
		}

		if serviceSpec.PluginType == nil {
			return nil
		}

		restServiceSpec, ok := serviceSpec.PluginType.(*glooplugins.ServiceSpec_Rest)
		if !ok {
			return nil
		}
		if restServiceSpec.Rest == nil {
			return errors.Errorf("%v has an empty rest service spec", in.Metadata.Ref())
		}
		p.recordedUpstreams[in.Metadata.Ref()] = restServiceSpec
	}
	return nil
}

func (p *plugin) ProcessRoute(params plugins.RouteParams, in *v1.Route, out *envoyroute.Route) error {
	return pluginutils.MarkPerFilterConfig(p.ctx, params.Snapshot, in, out, transformation.FilterName, func(spec *v1.Destination) (proto.Message, error) {
		// check if it's rest destination
		if spec.DestinationSpec == nil {
			return nil, nil
		}
		restDestinationSpec, ok := spec.DestinationSpec.DestinationType.(*v1.DestinationSpec_Rest)
		if !ok {
			return nil, nil
		}

		// get upstream
		upstreamRef, err := upstreams.DestinationToUpstreamRef(spec)
		if err != nil {
			contextutils.LoggerFrom(p.ctx).Error(err)
			return nil, err
		}
		restServiceSpec, ok := p.recordedUpstreams[*upstreamRef]
		if !ok {
			return nil, errors.Errorf("%v does not have a rest service spec", *upstreamRef)
		}
		funcname := restDestinationSpec.Rest.FunctionName
		transformationorig := restServiceSpec.Rest.Transformations[funcname]
		if transformationorig == nil {
			return nil, errors.Errorf("unknown function %v", funcname)
		}

		// copy to prevent changing the original in memory.
		transformation := *transformationorig

		// add extensions from the destination spec
		transformation.Extractors, err = transformutils.CreateRequestExtractors(params.Ctx, restDestinationSpec.Rest.Parameters)
		if err != nil {
			return nil, err
		}

		// get function
		ret := &transformapi.RouteTransformations{
			RequestTransformation: &transformapi.Transformation{
				TransformationType: &transformapi.Transformation_TransformationTemplate{
					TransformationTemplate: &transformation,
				},
			},
		}

		*p.transformsAdded = true
		if restDestinationSpec.Rest.ResponseTransformation != nil {
			// TODO(yuval-k): should we add \ support response parameters?
			ret.ResponseTransformation = &transformapi.Transformation{
				TransformationType: &transformapi.Transformation_TransformationTemplate{
					TransformationTemplate: restDestinationSpec.Rest.ResponseTransformation,
				},
			}
		}

		return ret, nil
	})
}

package rest

import (
	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"

	"github.com/pkg/errors"

	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/log"
	"github.com/solo-io/gloo/pkg/plugins"
	"github.com/solo-io/gloo/pkg/plugins/common/transformation"
)

const (
	ServiceTypeREST = "REST"
)

func init() {
	plugins.Register(NewPlugin())
}

func NewPlugin() *Plugin {
	return &Plugin{transformation: transformation.NewTransformationPlugin()}
}

type Plugin struct {
	transformation transformation.Plugin
}

func (p *Plugin) GetDependencies(_ *v1.Config) *plugins.Dependencies {
	return nil
}

func isOurs(in *v1.Upstream) bool {
	return in.ServiceInfo != nil && in.ServiceInfo.Type == ServiceTypeREST
}

func (p *Plugin) ProcessUpstream(params *plugins.UpstreamPluginParams, in *v1.Upstream, out *envoyapi.Cluster) error {
	if !isOurs(in) {
		return nil
	}
	p.transformation.ActivateFilterForCluster(out)

	return nil
}

func (p *Plugin) ProcessRoute(pluginParams *plugins.RoutePluginParams, in *v1.Route, out *envoyroute.Route) error {
	getTransformationFunction := createTransformationForRestFunction(pluginParams.Upstreams)
	if err := p.transformation.AddRequestTransformationsToRoute(getTransformationFunction, in, out); err != nil {
		return errors.Wrap(err, "failed to process request transformation")
	}
	if err := p.transformation.AddResponseTransformationsToRoute(in, out); err != nil {
		return errors.Wrap(err, "failed to process response transformation")
	}
	return nil
}

func createTransformationForRestFunction(upstreams []*v1.Upstream) transformation.GetTransformationFunction {
	return func(fnDestination *v1.Destination_Function) (*transformation.TransformationTemplate, error) {
		fn, err := findFunction(upstreams, fnDestination.Function.UpstreamName, fnDestination.Function.FunctionName)
		if err != nil {
			return nil, errors.Wrap(err, "finding function")
		}
		if fn == nil {
			return nil, nil
		}

		outputTemplates, err := DecodeFunctionSpec(fn.Spec)
		if err != nil {
			return nil, errors.Wrap(err, "decoding function spec")
		}

		// if the the function doesn't need a transformation, also return nil
		needsTransformation := outputTemplates.Body != nil

		// create templates
		// right now it's just a no-op, user writes inja directly
		headerTemplates := make(map[string]*transformation.InjaTemplate)
		for k, v := range outputTemplates.Headers {
			needsTransformation = true
			headerTemplates[k] = &transformation.InjaTemplate{Text: v}
		}

		if outputTemplates.Path != "" {
			needsTransformation = true
			headerTemplates[":path"] = &transformation.InjaTemplate{Text: outputTemplates.Path}
		}

		// this function doesn't request any kind of transformation
		if !needsTransformation {
			log.Debugf("does not need transformation: %v", outputTemplates)
			return nil, nil
		}

		template := &transformation.TransformationTemplate{
			Headers: headerTemplates,
		}

		if outputTemplates.Body != nil {
			template.BodyTransformation = &transformation.TransformationTemplate_Body{
				Body: &transformation.InjaTemplate{
					Text: *outputTemplates.Body,
				},
			}
		} else {
			template.BodyTransformation = &transformation.TransformationTemplate_Passthrough{
				Passthrough: &transformation.Passthrough{},
			}
		}

		return template, nil
	}
}

func findFunction(upstreams []*v1.Upstream, upstreamName, functionName string) (*v1.Function, error) {
	for _, us := range upstreams {
		if us.Name == upstreamName {
			if !isOurs(us) {
				return nil, nil
			}
			for _, fn := range us.Functions {
				if fn.Name == functionName {
					return fn, nil
				}
			}
		}
	}
	return nil, errors.Errorf("function %v/%v not found", upstreamName, functionName)
}

func (p *Plugin) HttpFilters(_ *plugins.HttpFilterPluginParams) []plugins.StagedHttpFilter {
	filter := p.transformation.GetTransformationFilter()
	if filter == nil {
		return nil
	}

	return []plugins.StagedHttpFilter{*filter}
}

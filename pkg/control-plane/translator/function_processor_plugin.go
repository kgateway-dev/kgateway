package translator

import (
	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/gogo/protobuf/types"
	"github.com/pkg/errors"

	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/plugins"
)

type functionalPluginProcessor struct {
	functionPlugins []plugins.FunctionPlugin
}

func newFunctionalPluginProcessor(functionPlugins []plugins.FunctionPlugin) *functionalPluginProcessor {
	return &functionalPluginProcessor{functionPlugins: functionPlugins}
}

func (p *functionalPluginProcessor) GetDependencies(_ *v1.Config) *plugins.Dependencies {
	return nil
}

func (p *functionalPluginProcessor) ProcessUpstream(params *plugins.UpstreamPluginParams, in *v1.Upstream, out *envoyapi.Cluster) error {
	for _, function := range in.Functions {
		envoyFunctionSpec, err := p.getFunctionSpec(in, function.Spec)
		if err != nil {
			return errors.Wrapf(err, "processing function %v/%v failed", in.Name, function.Name)
		}
		// not all functions are meant to have specs
		// for example translation filter
		if envoyFunctionSpec == nil {
			continue
		}
		addEnvoyFunctionSpec(out, function.Name, envoyFunctionSpec)
	}
	return nil
}

func (p *functionalPluginProcessor) getFunctionSpec(upstream *v1.Upstream, spec v1.FunctionSpec) (*types.Struct, error) {
	for _, functionPlugin := range p.functionPlugins {
		var serviceType string
		if upstream.ServiceInfo != nil {
			serviceType = upstream.ServiceInfo.Type
		}
		params := &plugins.FunctionPluginParams{
			UpstreamType: upstream.Type,
			ServiceType:  serviceType,
		}
		envoyFunctionSpec, err := functionPlugin.ParseFunctionSpec(params, spec)
		if err != nil {
			return nil, errors.Wrap(err, "invalid function spec")
		}
		// try until we get a plugin that handles this function
		if envoyFunctionSpec == nil {
			continue
		}
		return envoyFunctionSpec, nil
	}
	// no function plugin found
	return nil, nil
}

func addEnvoyFunctionSpec(out *envoyapi.Cluster, funcName string, spec *types.Struct) {
	if out.Metadata == nil {
		out.Metadata = &envoycore.Metadata{}
	}
	multiFunctionMetadata := getFunctionalFilterMetadata(multiFunctionListDestinationKey, out.Metadata)

	if multiFunctionMetadata.Fields[funcName] == nil {
		multiFunctionMetadata.Fields[funcName] = &types.Value{}
	}
	multiFunctionMetadata.Fields[funcName].Kind = &types.Value_StructValue{StructValue: spec}
}

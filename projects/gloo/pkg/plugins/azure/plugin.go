package azure

import (
	"context"
	"fmt"

	"github.com/solo-io/gloo/projects/gloo/pkg/upstreams"
	"github.com/solo-io/go-utils/contextutils"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoyauth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/gogo/protobuf/proto"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/azure"
	transformationapi "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/transformation"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/pluginutils"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/transformation"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/errors"
)

const (
	masterKeyName = "_master"
)

type plugin struct {
	recordedUpstreams map[core.ResourceRef]*azure.UpstreamSpec
	apiKeys           map[string]string
	ctx               context.Context
	transformsAdded   *bool
}

func NewPlugin(transformsAdded *bool) plugins.Plugin {
	return &plugin{transformsAdded: transformsAdded}
}

func (p *plugin) Init(params plugins.InitParams) error {
	p.ctx = params.Ctx
	p.recordedUpstreams = make(map[core.ResourceRef]*azure.UpstreamSpec)
	return nil
}

func (p *plugin) ProcessUpstream(params plugins.Params, in *v1.Upstream, out *envoyapi.Cluster) error {
	upstreamSpec, ok := in.UpstreamSpec.UpstreamType.(*v1.UpstreamSpec_Azure)
	if !ok {
		// not ours
		return nil
	}
	azureUpstream := upstreamSpec.Azure
	p.recordedUpstreams[in.Metadata.Ref()] = azureUpstream

	// configure Envoy cluster routing info
	out.ClusterDiscoveryType = &envoyapi.Cluster_Type{
		Type: envoyapi.Cluster_LOGICAL_DNS,
	}
	// TODO(yuval-k): why do we need to make sure we use ipv4 only dns?
	out.DnsLookupFamily = envoyapi.Cluster_V4_ONLY
	hostname := GetHostname(upstreamSpec.Azure)

	pluginutils.EnvoySingleEndpointLoadAssignment(out, hostname, 443)

	out.TlsContext = &envoyauth.UpstreamTlsContext{
		// TODO(yuval-k): Add verification context
		Sni: hostname,
	}

	if azureUpstream.SecretRef.Name != "" {
		secrets, err := params.Snapshot.Secrets.Find(azureUpstream.SecretRef.Strings())
		if err != nil {
			return errors.Wrapf(err, "azure secrets for ref %v not found", azureUpstream.SecretRef)
		}
		azureSecrets, ok := secrets.Kind.(*v1.Secret_Azure)
		if !ok {
			return errors.Errorf("secret %v is not an Azure secret", secrets.GetMetadata().Ref())
		}
		p.apiKeys = azureSecrets.Azure.ApiKeys
	}

	return nil
}

func (p *plugin) ProcessRoute(params plugins.RouteParams, in *v1.Route, out *envoyroute.Route) error {
	return pluginutils.MarkPerFilterConfig(p.ctx, params.Snapshot, in, out, transformation.FilterName, func(spec *v1.Destination) (proto.Message, error) {
		// check if it's azure upstream destination
		if spec.DestinationSpec == nil {
			return nil, nil
		}
		azureDestinationSpec, ok := spec.DestinationSpec.DestinationType.(*v1.DestinationSpec_Azure)
		if !ok {
			return nil, nil
		}

		upstreamRef, err := upstreams.DestinationToUpstreamRef(spec)
		if err != nil {
			contextutils.LoggerFrom(p.ctx).Error(err)
			return nil, err
		}
		upstreamSpec, ok := p.recordedUpstreams[*upstreamRef]
		if !ok {
			// TODO(yuval-k): panic in debug
			return nil, errors.Errorf("%v is not an Azure upstream", *upstreamRef)
		}

		// get function
		functionName := azureDestinationSpec.Azure.FunctionName
		for _, functionSpec := range upstreamSpec.Functions {
			if functionSpec.FunctionName == functionName {
				path, err := getPath(functionSpec, p.apiKeys)
				if err != nil {
					return nil, err
				}

				*p.transformsAdded = true

				hostname := GetHostname(upstreamSpec)
				// TODO: consider adding a new add headers transformation allow adding headers with no templates to improve performance.
				ret := &transformationapi.RouteTransformations{
					RequestTransformation: &transformationapi.Transformation{
						TransformationType: &transformationapi.Transformation_TransformationTemplate{
							TransformationTemplate: &transformationapi.TransformationTemplate{
								Headers: map[string]*transformationapi.InjaTemplate{
									":path": {
										Text: path,
									},
									":authority": {
										Text: hostname,
									},
								},
								BodyTransformation: &transformationapi.TransformationTemplate_Passthrough{
									Passthrough: &transformationapi.Passthrough{},
								},
							},
						},
					},
				}

				return ret, nil
			}
		}
		return nil, errors.Errorf("unknown function %v", functionName)
	})
}

func getPath(functionSpec *azure.UpstreamSpec_FunctionSpec, apiKeys map[string]string) (string, error) {
	functionName := functionSpec.FunctionName

	pathParameters, err := getPathParameters(functionSpec, apiKeys)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("/api/%s%s", functionName, pathParameters), nil
}

func getPathParameters(functionSpec *azure.UpstreamSpec_FunctionSpec, apiKeys map[string]string) (string, error) {
	var keyNames []string
	switch functionSpec.AuthLevel {
	case azure.UpstreamSpec_FunctionSpec_Anonymous:
		return "", nil
	case azure.UpstreamSpec_FunctionSpec_Function:
		// TODO(talnordan): Consider whether using the "function" authentication level should require
		// using a function key and not a master key. This is a product decision. From the technical
		// point of view, a master key does satisfy the "function" authentication level.
		keyNames = []string{functionSpec.FunctionName, masterKeyName}
	case azure.UpstreamSpec_FunctionSpec_Admin:
		keyNames = []string{masterKeyName}
	}

	apiKey, err := getApiKey(apiKeys, keyNames)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("?code=%s", apiKey), nil
}

func getApiKey(apiKeys map[string]string, keyNames []string) (string, error) {
	for _, keyName := range keyNames {
		apiKey, ok := apiKeys[keyName]
		if ok && apiKey != "" {
			return apiKey, nil
		}
	}

	return "", fmt.Errorf("secret not found for key names %v", keyNames)
}

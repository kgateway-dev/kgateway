package azure

import (
	"context"
	"fmt"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoyauth "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/protobuf/proto"
	transformationapi "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/transformation"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/azure"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/pluginutils"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/transformation"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"github.com/solo-io/gloo/projects/gloo/pkg/upstreams"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/errors"
)

var (
	_ plugins.Plugin         = new(plugin)
	_ plugins.RoutePlugin    = new(plugin)
	_ plugins.UpstreamPlugin = new(plugin)
)

const (
	ExtensionName = "azure"
	masterKeyName = "_master"
)

type plugin struct {
	settings          *v1.Settings
	recordedUpstreams map[string]*azure.UpstreamSpec
	apiKeys           map[string]string
	ctx               context.Context
}

func NewPlugin() plugins.Plugin {
	return &plugin{}
}

func (p *plugin) Name() string {
	return ExtensionName
}

func (p *plugin) Init(params plugins.InitParams) {
	p.settings = params.Settings
	p.ctx = params.Ctx
	p.recordedUpstreams = make(map[string]*azure.UpstreamSpec)
	p.apiKeys = make(map[string]string)
}

func (p *plugin) ProcessUpstream(params plugins.Params, in *v1.Upstream, out *envoy_config_cluster_v3.Cluster) error {
	upstreamSpec, ok := in.GetUpstreamType().(*v1.Upstream_Azure)
	if !ok {
		// not ours
		return nil
	}
	azureUpstream := upstreamSpec.Azure
	p.recordedUpstreams[translator.UpstreamToClusterName(in.GetMetadata().Ref())] = azureUpstream

	// configure Envoy cluster routing info
	out.ClusterDiscoveryType = &envoy_config_cluster_v3.Cluster_Type{
		Type: envoy_config_cluster_v3.Cluster_LOGICAL_DNS,
	}
	// TODO(yuval-k): why do we need to make sure we use ipv4 only dns?
	out.DnsLookupFamily = envoy_config_cluster_v3.Cluster_V4_ONLY
	hostname := GetHostname(upstreamSpec.Azure)

	pluginutils.EnvoySingleEndpointLoadAssignment(out, hostname, 443)

	commonTlsContext, err := utils.GetCommonTlsContextFromUpstreamOptions(p.settings.GetUpstreamOptions())
	if err != nil {
		return err
	}
	tlsContext := &envoyauth.UpstreamTlsContext{
		CommonTlsContext: commonTlsContext,
		// TODO(yuval-k): Add verification context
		Sni: hostname,
	}
	typedConfig, err := utils.MessageToAny(tlsContext)
	if err != nil {
		return err
	}
	out.TransportSocket = &envoy_config_core_v3.TransportSocket{
		Name:       wellknown.TransportSocketTls,
		ConfigType: &envoy_config_core_v3.TransportSocket_TypedConfig{TypedConfig: typedConfig},
	}

	if azureUpstream.GetSecretRef().GetName() != "" {
		secrets, err := params.Snapshot.Secrets.Find(azureUpstream.GetSecretRef().Strings())
		if err != nil {
			return errors.Wrapf(err, "azure secrets for ref %v not found", azureUpstream.GetSecretRef())
		}
		azureSecrets, ok := secrets.GetKind().(*v1.Secret_Azure)
		if !ok {
			return errors.Errorf("secret %v is not an Azure secret", secrets.GetMetadata().Ref())
		}
		p.apiKeys = azureSecrets.Azure.GetApiKeys()
	}

	return nil
}

func (p *plugin) ProcessRoute(params plugins.RouteParams, in *v1.Route, out *envoy_config_route_v3.Route) error {
	return pluginutils.MarkPerFilterConfig(p.ctx, params.Snapshot, in, out, transformation.FilterName,
		func(spec *v1.Destination) (proto.Message, error) {
			// check if it's azure upstream destination
			if spec.GetDestinationSpec() == nil {
				return nil, nil
			}
			azureDestinationSpec, ok := spec.GetDestinationSpec().GetDestinationType().(*v1.DestinationSpec_Azure)
			if !ok {
				return nil, nil
			}

			upstreamRef, err := upstreams.DestinationToUpstreamRef(spec)
			if err != nil {
				contextutils.LoggerFrom(p.ctx).Error(err)
				return nil, err
			}
			upstreamSpec, ok := p.recordedUpstreams[translator.UpstreamToClusterName(upstreamRef)]
			if !ok {
				// TODO(yuval-k): panic in debug
				return nil, errors.Errorf("%v is not an Azure upstream", *upstreamRef)
			}

			// get function
			functionName := azureDestinationSpec.Azure.GetFunctionName()
			for _, functionSpec := range upstreamSpec.GetFunctions() {
				if functionSpec.GetFunctionName() == functionName {
					path, err := getPath(functionSpec, p.apiKeys)
					if err != nil {
						return nil, err
					}

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
		},
	)
}

func getPath(functionSpec *azure.UpstreamSpec_FunctionSpec, apiKeys map[string]string) (string, error) {
	functionName := functionSpec.GetFunctionName()

	pathParameters, err := getPathParameters(functionSpec, apiKeys)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("/api/%s%s", functionName, pathParameters), nil
}

func getPathParameters(functionSpec *azure.UpstreamSpec_FunctionSpec, apiKeys map[string]string) (string, error) {
	var keyNames []string
	switch functionSpec.GetAuthLevel() {
	case azure.UpstreamSpec_FunctionSpec_Anonymous:
		return "", nil
	case azure.UpstreamSpec_FunctionSpec_Function:
		// TODO(talnordan): Consider whether using the "function" authentication level should require
		// using a function key and not a master key. This is a product decision. From the technical
		// point of view, a master key does satisfy the "function" authentication level.
		keyNames = []string{functionSpec.GetFunctionName(), masterKeyName}
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

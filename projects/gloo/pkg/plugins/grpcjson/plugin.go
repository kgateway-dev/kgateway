package grpcjson

import (
	"context"
	"encoding/base64"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_extensions_filters_http_grpc_json_transcoder_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/grpc_json_transcoder/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	glooplugins "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/grpc_json"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/pluginutils"
)

var (
	_ plugins.Plugin           = new(plugin)
	_ plugins.UpstreamPlugin   = new(plugin)
	_ plugins.HttpFilterPlugin = new(plugin)
	_ plugins.RoutePlugin      = new(plugin)

	NoConfigMapRefError = func() error {
		return eris.Errorf("a configmap ref must be provided")
	}
	ConfigMapNotFoundError = func(configRef *grpc_json.GrpcJsonTranscoder_DescriptorConfigMap) error {
		return eris.Errorf("configmap %s:%s cannot be found", configRef.GetConfigMapRef().GetNamespace(), configRef.GetConfigMapRef().GetName())
	}
	ConfigMapNoValuesError = func(configRef *grpc_json.GrpcJsonTranscoder_DescriptorConfigMap) error {
		return eris.Errorf("configmap %s:%s does not contain any values", configRef.GetConfigMapRef().GetNamespace(), configRef.GetConfigMapRef().GetName())
	}
	NoConfigMapKeyError = func(configRef *grpc_json.GrpcJsonTranscoder_DescriptorConfigMap, numValues int) error {
		return eris.Errorf("key must be provided for configmap %s:%s which contains %d values",
			configRef.GetConfigMapRef().GetNamespace(), configRef.GetConfigMapRef().GetName(), numValues)
	}
	NoDataError = func(configRef *grpc_json.GrpcJsonTranscoder_DescriptorConfigMap, key string) error {
		return eris.Errorf("configmap %s:%s does not contain a value for key %s", configRef.GetConfigMapRef().GetNamespace(), configRef.GetConfigMapRef().GetName(), key)
	}
	DecodingError = func(configRef *grpc_json.GrpcJsonTranscoder_DescriptorConfigMap, key string) error {
		return eris.Errorf("config map %s:%s contains a value for key %s but is not base64-encoded",
			configRef.GetConfigMapRef().GetNamespace(), configRef.GetConfigMapRef().GetName(), key)
	}
)

const (
	// ExtensionName for the grpc to json Transcoder plugin
	ExtensionName = "gprc_json"
)

// filter info
var pluginStage = plugins.BeforeStage(plugins.OutAuthStage)

type plugin struct {
	// Map of ResourceRef.Key() (namespace.name) for gRPC Upstreams --> grpcJsonTranscoder filter to add to routes
	upstreamFilters map[string]plugins.StagedHttpFilter
	// List of listeners that need an empty gRPC json filter that will be overridden by a route
	affectedListeners map[*v1.HttpListener]int
}

func NewPlugin() *plugin {
	return &plugin{}
}

func (p *plugin) Name() string {
	return ExtensionName
}

func (p *plugin) Init(params plugins.InitParams) {
	p.upstreamFilters = make(map[string]plugins.StagedHttpFilter)
	p.affectedListeners = make(map[*v1.HttpListener]int)
}
func (p *plugin) ProcessUpstream(params plugins.Params, in *v1.Upstream, out *envoy_config_cluster_v3.Cluster) error {

	upstreamType, ok := in.GetUpstreamType().(v1.ServiceSpecGetter)
	if !ok {
		return nil
	}

	if upstreamType.GetServiceSpec() == nil {
		return nil
	}

	grpcJsonConf, ok := upstreamType.GetServiceSpec().GetPluginType().(*glooplugins.ServiceSpec_GrpcJsonTranscoder)
	if !ok {
		return nil
	}
	envoyGrpcJsonConf, err := translateGlooToEnvoyGrpcJson(params, grpcJsonConf.GrpcJsonTranscoder)
	if err != nil {
		return err
	}
	// Discovery will create an empty serviceSpec if the service does not provide descriptors which does not warrant a filter.
	if envoyGrpcJsonConf.GetDescriptorSet() == nil {
		return nil
	}
	grpcJsonFilter, err := plugins.NewStagedFilter(wellknown.GRPCJSONTranscoder, envoyGrpcJsonConf, pluginStage)
	p.upstreamFilters[in.GetMetadata().Ref().Key()] = grpcJsonFilter
	// GRPC transcoding always requires http2
	if out.GetHttp2ProtocolOptions() == nil {
		out.Http2ProtocolOptions = &envoy_config_core_v3.Http2ProtocolOptions{}
	}
	return nil
}
func (p *plugin) HttpFilters(params plugins.Params, listener *v1.HttpListener) ([]plugins.StagedHttpFilter, error) {
	grpcJsonConf := listener.GetOptions().GetGrpcJsonTranscoder()
	if grpcJsonConf != nil {
		// put the config from the gateway resource on the listener
		envoyGrpcJsonConf, err := translateGlooToEnvoyGrpcJson(params, grpcJsonConf)
		if err != nil {
			return nil, err
		}
		grpcJsonFilter, err := plugins.NewStagedFilter(wellknown.GRPCJSONTranscoder, envoyGrpcJsonConf, pluginStage)
		if err != nil {
			return nil, eris.Wrapf(err, "generating filter config")
		}
		return []plugins.StagedHttpFilter{grpcJsonFilter}, nil
	} else if len(p.upstreamFilters) > 0 {
		if _, ok := p.affectedListeners[listener]; !ok {
			return nil, nil
		}
		// There needs to be a filter on the listener to be able to set filters on routes
		// Envoy errors if this config is empty
		emptyGlooConfig := &grpc_json.GrpcJsonTranscoder{
			DescriptorSet: &grpc_json.GrpcJsonTranscoder_ProtoDescriptor{ProtoDescriptor: "/to/override"},
		}
		envoyGrpcJsonConf, err := translateGlooToEnvoyGrpcJson(params, emptyGlooConfig)
		if err != nil {
			return nil, err
		}
		grpcJsonFilter, err := plugins.NewStagedFilter(wellknown.GRPCJSONTranscoder, envoyGrpcJsonConf, pluginStage)
		if err != nil {
			return nil, eris.Wrapf(err, "generating filter config")
		}
		return []plugins.StagedHttpFilter{grpcJsonFilter}, nil
	}
	return nil, nil
}
func (p *plugin) ProcessRoute(params plugins.RouteParams, in *v1.Route, out *envoy_config_route_v3.Route) error {
	if len(p.upstreamFilters) == 0 {
		return nil
	}
	routeUpstreams, err := pluginutils.DestinationUpstreams(params.Snapshot, in.GetRouteAction())
	if err != nil {
		return err
	}
	for _, us := range routeUpstreams {
		if filter, ok := p.upstreamFilters[us.Key()]; ok {
			if out.GetTypedPerFilterConfig() == nil {
				out.TypedPerFilterConfig = make(map[string]*any.Any)
			}
			// Assume that a single route won't have upstreams with multiple different grpc configurations
			out.GetTypedPerFilterConfig()[wellknown.GRPCJSONTranscoder] = filter.HttpFilter.GetTypedConfig()
			p.affectedListeners[params.HttpListener] = 1
			return nil
		}
	}
	return nil
}
func translateGlooToEnvoyGrpcJson(params plugins.Params, grpcJsonConf *grpc_json.GrpcJsonTranscoder) (*envoy_extensions_filters_http_grpc_json_transcoder_v3.GrpcJsonTranscoder, error) {

	envoyGrpcJsonConf := &envoy_extensions_filters_http_grpc_json_transcoder_v3.GrpcJsonTranscoder{
		DescriptorSet:                nil, // may be set in multiple ways
		Services:                     grpcJsonConf.GetServices(),
		PrintOptions:                 translateGlooToEnvoyPrintOptions(grpcJsonConf.GetPrintOptions()),
		MatchIncomingRequestRoute:    grpcJsonConf.GetMatchIncomingRequestRoute(),
		IgnoredQueryParameters:       grpcJsonConf.GetIgnoredQueryParameters(),
		AutoMapping:                  grpcJsonConf.GetAutoMapping(),
		IgnoreUnknownQueryParameters: grpcJsonConf.GetIgnoreUnknownQueryParameters(),
		ConvertGrpcStatus:            grpcJsonConf.GetConvertGrpcStatus(),
	}

	// Convert from our descriptor storages to the appropriate type
	switch typedDescriptorSet := grpcJsonConf.GetDescriptorSet().(type) {
	case *grpc_json.GrpcJsonTranscoder_ProtoDescriptorConfigMap:
		protoDesc, err := translateConfigMapToProtoBin(params.Ctx, params.Snapshot, typedDescriptorSet.ProtoDescriptorConfigMap)
		if err != nil {
			return nil, err
		}
		envoyGrpcJsonConf.DescriptorSet = &envoy_extensions_filters_http_grpc_json_transcoder_v3.GrpcJsonTranscoder_ProtoDescriptorBin{ProtoDescriptorBin: protoDesc}
	case *grpc_json.GrpcJsonTranscoder_ProtoDescriptor:
		envoyGrpcJsonConf.DescriptorSet = &envoy_extensions_filters_http_grpc_json_transcoder_v3.GrpcJsonTranscoder_ProtoDescriptor{ProtoDescriptor: typedDescriptorSet.ProtoDescriptor}
	case *grpc_json.GrpcJsonTranscoder_ProtoDescriptorBin:
		envoyGrpcJsonConf.DescriptorSet = &envoy_extensions_filters_http_grpc_json_transcoder_v3.GrpcJsonTranscoder_ProtoDescriptorBin{ProtoDescriptorBin: typedDescriptorSet.ProtoDescriptorBin}
	}

	return envoyGrpcJsonConf, nil
}

func translateGlooToEnvoyPrintOptions(options *grpc_json.GrpcJsonTranscoder_PrintOptions) *envoy_extensions_filters_http_grpc_json_transcoder_v3.GrpcJsonTranscoder_PrintOptions {
	if options == nil {
		return nil
	}
	return &envoy_extensions_filters_http_grpc_json_transcoder_v3.GrpcJsonTranscoder_PrintOptions{
		AddWhitespace:              options.GetAddWhitespace(),
		AlwaysPrintPrimitiveFields: options.GetAlwaysPrintPrimitiveFields(),
		AlwaysPrintEnumsAsInts:     options.GetAlwaysPrintEnumsAsInts(),
		PreserveProtoFieldNames:    options.GetPreserveProtoFieldNames(),
	}
}

// get the proto descriptor data from a ConfigMap
func translateConfigMapToProtoBin(ctx context.Context, snap *gloosnapshot.ApiSnapshot, configRef *grpc_json.GrpcJsonTranscoder_DescriptorConfigMap) ([]byte, error) {
	if configRef.GetConfigMapRef() == nil {
		return nil, NoConfigMapRefError()
	}

	// make sure the referenced configmap exists in the gloo snapshot
	configMap, err := snap.Artifacts.Find(configRef.GetConfigMapRef().Strings())
	if err != nil {
		return nil, ConfigMapNotFoundError(configRef)
	}

	// make sure the configmap has data
	data := configMap.GetData()
	if len(data) == 0 {
		return nil, ConfigMapNoValuesError(configRef)
	}

	// get the base64-encoded proto descriptor string
	var protoDescriptor string
	key := configRef.GetKey()
	if key != "" {
		// if there is an explicit key, use it
		protoDescriptor = data[key]
	} else {
		// if there is exactly one value, use it
		if len(data) == 1 {
			for k, v := range data {
				key = k
				protoDescriptor = v
				break
			}
		} else {
			// if there are multiple key-value pairs, an explicit key must be provided
			return nil, NoConfigMapKeyError(configRef, len(data))
		}
	}

	if protoDescriptor == "" {
		return nil, NoDataError(configRef, key)
	}

	// decode the base64-encoded proto descriptor
	decodedBytes, err := base64.StdEncoding.DecodeString(protoDescriptor)
	if err != nil {
		return nil, DecodingError(configRef, key)
	}

	return decodedBytes, nil
}

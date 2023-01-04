package grpcjson

import (
	"context"
	"encoding/base64"

	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/rotisserie/eris"

	envoy_extensions_filters_http_grpc_json_transcoder_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/grpc_json_transcoder/v3"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/grpc_json"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/go-utils/contextutils"
)

var (
	_ plugins.Plugin           = new(plugin)
	_ plugins.HttpFilterPlugin = new(plugin)
)

const (
	// ExtensionName for the grpc to json Transcoder plugin
	ExtensionName = "gprc_json"
)

// filter info
var pluginStage = plugins.BeforeStage(plugins.OutAuthStage)

type plugin struct{}

func NewPlugin() *plugin {
	return &plugin{}
}

func (p *plugin) Name() string {
	return ExtensionName
}

func (p *plugin) Init(_ plugins.InitParams) {
}

func (p *plugin) HttpFilters(params plugins.Params, listener *v1.HttpListener) ([]plugins.StagedHttpFilter, error) {
	grpcJsonConf := listener.GetOptions().GetGrpcJsonTranscoder()
	if grpcJsonConf == nil {
		return nil, nil
	}

	envoyGrpcJsonConf, err := translateGlooToEnvoyGrpcJson(params, grpcJsonConf)
	if err != nil {
		return nil, err
	}

	grpcJsonFilter, err := plugins.NewStagedFilter(wellknown.GRPCJSONTranscoder, envoyGrpcJsonConf, pluginStage)
	if err != nil {
		return nil, eris.Wrapf(err, "generating filter config")
	}

	return []plugins.StagedHttpFilter{grpcJsonFilter}, nil
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

	// Convert from our descriptor storages to the appropriate tiype
	switch typedDescriptorSet := grpcJsonConf.GetDescriptorSet().(type) {
	case *grpc_json.GrpcJsonTranscoder_ProtoDescriptorConfigMap:
		bytes := translateConfigMapToProtoBin(params.Ctx, params.Snapshot, typedDescriptorSet.ProtoDescriptorConfigMap)
		envoyGrpcJsonConf.DescriptorSet = &envoy_extensions_filters_http_grpc_json_transcoder_v3.GrpcJsonTranscoder_ProtoDescriptorBin{ProtoDescriptorBin: bytes}
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

func translateConfigMapToProtoBin(ctx context.Context, snap *gloosnapshot.ApiSnapshot, configRef *grpc_json.GrpcJsonTranscoder_DescriptorConfigMap) (out []byte) {

	configMap, err := snap.Artifacts.Find(configRef.GetConfigMapRef().Strings())
	if err != nil {
		contextutils.LoggerFrom(ctx).Warnf("config map %s:%s cannot be found", configRef.GetConfigMapRef().Namespace, configRef.GetConfigMapRef().Name)
		return
	}

	// if a key set is provided pull from that value
	potentialDescriptor := configMap.GetData()[configRef.GetKey()]

	// if the descriptor was empty then return early
	if potentialDescriptor == "" {
		contextutils.LoggerFrom(ctx).Warnf("config map %s:%s does not contain a value for key %s", configRef.GetConfigMapRef().Namespace, configRef.GetConfigMapRef().Name, configRef.GetKey())
		return
	}

	// we support both base64 encoded and non-encoded values
	// if the value is base64 encoded then decode it
	if configRef.GetEncoding() == grpc_json.GrpcJsonTranscoder_DescriptorConfigMap_BASE64 {
		decodedDescriptor, err := base64.StdEncoding.DecodeString(potentialDescriptor)
		if err != nil {
			contextutils.LoggerFrom(ctx).Warnf(
				"config map %s:%s contains a value for key %s but is not base64 encoded",
				configRef.GetConfigMapRef().Namespace, configRef.GetConfigMapRef().Name, configRef.GetKey())
		}
		return decodedDescriptor
	}

	// if encoding is unset or set to unencoded then return the value as is
	return []byte(potentialDescriptor)
}

package aws

import (
	"unicode/utf8"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoyauth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"

	"github.com/gogo/protobuf/types"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"

	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/coreplugins/common"
	"github.com/solo-io/gloo/pkg/plugins"
)

func init() {
	plugins.Register(&Plugin{})
}

type Plugin struct {
	isNeeded bool
}

const (
	// define Upstream type name
	UpstreamTypeAws = "aws"

	// generic plugin info
	filterName  = "io.solo.lambda"
	pluginStage = plugins.OutAuth

	// filter-specific metadata
	filterMetadataKeyAsync = "async"

	// upstream-specific metadata
	AwsAccessKey = "access_key"
	AwsSecretKey = "secret_key"
	awsRegion    = "region"
	awsHost      = "host"

	// function-specific metadata
	functionNameKey      = "name"
	functionQualifierKey = "qualifier"
)

//go:generate protoc -I=./ -I=${GOPATH}/src/github.com/gogo/protobuf/ -I=${GOPATH}/src/github.com/gogo/protobuf/protobuf/ --gogo_out=Mgoogle/protobuf/wrappers.proto=github.com/gogo/protobuf/types:${GOPATH}/src upstream_spec.proto

func (p *Plugin) GetDependencies(cfg *v1.Config) *plugins.Dependencies {
	deps := new(plugins.Dependencies)
	for _, upstream := range cfg.Upstreams {
		if upstream.Type != UpstreamTypeAws {
			continue
		}
		awsUpstream, err := DecodeUpstreamSpec(upstream.Spec)
		if err != nil {
			// errors will be handled during validation
			// TODO: consider logging error here
			continue
		}
		deps.SecretRefs = append(deps.SecretRefs, awsUpstream.SecretRef)
	}
	return deps
}

func (p *Plugin) HttpFilters(params *plugins.HttpFilterPluginParams) []plugins.StagedHttpFilter {

	defer func() { p.isNeeded = false }()

	if p.isNeeded {
		return []plugins.StagedHttpFilter{{HttpFilter: &envoyhttp.HttpFilter{Name: filterName}, Stage: pluginStage}}
	}
	return nil
}

func (p *Plugin) ProcessRoute(_ *plugins.RoutePluginParams, in *v1.Route, out *envoyroute.Route) error {
	executionStyle, err := GetExecutionStyle(in.Extensions)
	if err != nil {
		return err
	}
	if executionStyle == ExecutionStyleNone {
		return nil
	}
	setRouteAsync(executionStyle == ExecutionStyleAsync, out)
	return nil
}

func setRouteAsync(async bool, out *envoyroute.Route) {
	if out.Metadata == nil {
		out.Metadata = &envoycore.Metadata{}
	}
	common.InitFilterMetadataField(filterName, filterMetadataKeyAsync, out.Metadata).Kind = &types.Value_BoolValue{
		BoolValue: async,
	}
}

func (p *Plugin) ProcessUpstream(params *plugins.UpstreamPluginParams, in *v1.Upstream, out *envoyapi.Cluster) error {
	if in.Type != UpstreamTypeAws {
		return nil
	}
	p.isNeeded = true

	out.Type = envoyapi.Cluster_LOGICAL_DNS
	// need to make sure we use ipv4 only dns
	out.DnsLookupFamily = envoyapi.Cluster_V4_ONLY

	awsUpstream, err := DecodeUpstreamSpec(in.Spec)
	if err != nil {
		return errors.Wrap(err, "invalid AWS upstream spec")
	}

	out.Hosts = append(out.Hosts, &envoycore.Address{Address: &envoycore.Address_SocketAddress{SocketAddress: &envoycore.SocketAddress{
		Address:       awsUpstream.GetLambdaHostname(),
		PortSpecifier: &envoycore.SocketAddress_PortValue{PortValue: 443},
	}}})
	out.TlsContext = &envoyauth.UpstreamTlsContext{
		Sni: awsUpstream.GetLambdaHostname(),
	}

	awsSecrets, ok := params.Secrets[awsUpstream.SecretRef]
	if !ok {
		return errors.Errorf("aws secrets for ref %v not found", awsUpstream.SecretRef)
	}

	var secretErrs error

	accessKey, ok := awsSecrets.Data[AwsAccessKey]
	if !ok {
		secretErrs = multierror.Append(secretErrs, errors.Errorf("key %v missing from provided secret", AwsAccessKey))
	}
	if accessKey == "" || !utf8.Valid([]byte(accessKey)) {
		secretErrs = multierror.Append(secretErrs, errors.Errorf("%s not a valid string", AwsAccessKey))
	}
	secretKey, ok := awsSecrets.Data[AwsSecretKey]
	if !ok {
		secretErrs = multierror.Append(secretErrs, errors.Errorf("key %v missing from provided secret", AwsSecretKey))
	}
	if secretKey == "" || !utf8.Valid([]byte(secretKey)) {
		secretErrs = multierror.Append(secretErrs, errors.Errorf("%s not a valid string", AwsSecretKey))
	}

	if out.Metadata == nil {
		out.Metadata = &envoycore.Metadata{}
	}
	common.InitFilterMetadata(filterName, out.Metadata)
	out.Metadata.FilterMetadata[filterName] = &types.Struct{
		Fields: map[string]*types.Value{
			AwsAccessKey: {Kind: &types.Value_StringValue{StringValue: accessKey}},
			AwsSecretKey: {Kind: &types.Value_StringValue{StringValue: secretKey}},
			awsRegion:    {Kind: &types.Value_StringValue{StringValue: awsUpstream.Region}},
			awsHost:      {Kind: &types.Value_StringValue{StringValue: awsUpstream.GetLambdaHostname()}},
		},
	}

	return secretErrs
}

func (p *Plugin) ParseFunctionSpec(params *plugins.FunctionPluginParams, in v1.FunctionSpec) (*types.Struct, error) {
	if params.UpstreamType != UpstreamTypeAws {
		return nil, nil
	}
	functionSpec, err := DecodeFunctionSpec(in)
	if err != nil {
		return nil, errors.Wrap(err, "invalid lambda function spec")
	}
	return &types.Struct{
		Fields: map[string]*types.Value{
			functionNameKey:      {Kind: &types.Value_StringValue{StringValue: functionSpec.FunctionName}},
			functionQualifierKey: {Kind: &types.Value_StringValue{StringValue: functionSpec.Qualifier}},
		},
	}, nil
}

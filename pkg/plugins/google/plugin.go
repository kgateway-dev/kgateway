package google

import (
	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoyauth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"

	"github.com/gogo/protobuf/types"
	"github.com/pkg/errors"

	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/coreplugins/common"
	"github.com/solo-io/gloo/pkg/plugins"
)

func init() {
	plugins.Register(&Plugin{})
}

//go:generate protoc -I=./ -I=${GOPATH}/src/github.com/gogo/protobuf/ -I=${GOPATH}/src/github.com/gogo/protobuf/protobuf/ --gogo_out=Mgoogle/protobuf/wrappers.proto=github.com/gogo/protobuf/types:${GOPATH}/src spec.proto

type Plugin struct {
	isNeeded bool
}

const (
	// define Upstream type name
	UpstreamTypeGoogle = "google"

	// generic plugin info
	filterName  = "io.solo.gcloudfunc"
	pluginStage = plugins.OutAuth

	googleRegion = "region"

	// function-specific metadata
	functionHost = "host"
	functionPath = "path"
)

func (p *Plugin) GetDependencies(cfg *v1.Config) *plugins.Dependencies {

	return nil
}

func (p *Plugin) HttpFilters(params *plugins.HttpFilterPluginParams) []plugins.StagedHttpFilter {
	defer func() { p.isNeeded = false }()

	if p.isNeeded {
		return []plugins.StagedHttpFilter{{HttpFilter: &envoyhttp.HttpFilter{Name: filterName}, Stage: pluginStage}}
	}
	return nil
}

func (p *Plugin) ProcessRoute(_ *plugins.RoutePluginParams, in *v1.Route, out *envoyroute.Route) error {
	return nil
}

func (p *Plugin) ProcessUpstream(params *plugins.UpstreamPluginParams, in *v1.Upstream, out *envoyapi.Cluster) error {
	if in.Type != UpstreamTypeGoogle {
		return nil
	}
	p.isNeeded = true

	out.Type = envoyapi.Cluster_LOGICAL_DNS
	// need to make sure we use ipv4 only dns
	out.DnsLookupFamily = envoyapi.Cluster_V4_ONLY

	googleUpstream, err := DecodeUpstreamSpec(in.Spec)
	if err != nil {
		return errors.Wrap(err, "invalid Google upstream spec")
	}

	out.Hosts = append(out.Hosts, &envoycore.Address{Address: &envoycore.Address_SocketAddress{SocketAddress: &envoycore.SocketAddress{
		Address:       googleUpstream.GetGFuncHostname(),
		PortSpecifier: &envoycore.SocketAddress_PortValue{PortValue: 443},
	}}})
	out.TlsContext = &envoyauth.UpstreamTlsContext{
		Sni: googleUpstream.GetGFuncHostname(),
	}

	if out.Metadata == nil {
		out.Metadata = &envoycore.Metadata{}
	}
	common.InitFilterMetadata(filterName, out.Metadata)
	out.Metadata.FilterMetadata[filterName] = &types.Struct{
		Fields: map[string]*types.Value{
			googleRegion: {Kind: &types.Value_StringValue{StringValue: googleUpstream.Region}},
		},
	}

	return nil
}

func (p *Plugin) ParseFunctionSpec(params *plugins.FunctionPluginParams, in v1.FunctionSpec) (*types.Struct, error) {
	if params.UpstreamType != UpstreamTypeGoogle {
		return nil, nil
	}
	functionSpec, err := DecodeFunctionSpec(in)
	if err != nil {
		return nil, errors.Wrap(err, "invalid google function spec")
	}
	return &types.Struct{
		Fields: map[string]*types.Value{
			functionHost: {Kind: &types.Value_StringValue{StringValue: functionSpec.host}},
			functionPath: {Kind: &types.Value_StringValue{StringValue: functionSpec.path}},
		},
	}, nil
}

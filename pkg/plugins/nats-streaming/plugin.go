package natsstreaming

import (
	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"

	"github.com/gogo/protobuf/types"
	"github.com/solo-io/gloo/pkg/protoutil"

	"github.com/pkg/errors"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/coreplugins/common"
	"github.com/solo-io/gloo/pkg/log"
	"github.com/solo-io/gloo/pkg/plugins"
)

//go:generate protoc --gogo_out=Mgoogle/protobuf/duration.proto=github.com/gogo/protobuf/types:. -I=${GOPATH}/src/github.com/gogo/protobuf/ -I=${GOPATH}/src/github.com/gogo/protobuf/protobuf -I=.  nats_streaming_filter.proto

func init() {
	plugins.Register(&Plugin{})
}

type Plugin struct {
	filters []plugins.StagedHttpFilter
}

const (
	ServiceTypeNatsStreaming = "nats-streaming"

	// generic plugin info
	filterName  = "io.solo.nats_streaming"
	pluginStage = plugins.OutAuth

	// ClusterId key for NATS streaming
	ClusterId = "cluster_id"
	// DiscoverPrefix key for NATS streaming
	DiscoverPrefix = "discover_prefix"

	// DefaultClusterId default NATS streaming cluster ID
	DefaultClusterId = "test-cluster"
	// DefaultDiscoverPrefix default NATS streaming discover prefix
	DefaultDiscoverPrefix = "_STAN.discover"
)

type ServiceProperties struct {
	ClusterID      string `json:"cluster_id"`
	DiscoverPrefix string `json:"discover_prefix"`
}

func EncodeServiceProperties(props ServiceProperties) *types.Struct {
	s, err := protoutil.MarshalStruct(props)
	if err != nil {
		panic(err)
	}
	return s
}

func (p *Plugin) GetDependencies(cfg *v1.Config) *plugins.Dependencies {
	return nil
}

func (p *Plugin) HttpFilters(params *plugins.HttpFilterPluginParams) []plugins.StagedHttpFilter {
	filters := p.filters
	p.filters = nil
	return filters
}

func (p *Plugin) ProcessUpstream(params *plugins.UpstreamPluginParams, in *v1.Upstream, out *envoyapi.Cluster) error {
	if in.ServiceInfo == nil || in.ServiceInfo.Type != ServiceTypeNatsStreaming {
		return nil
	}
	var props ServiceProperties
	if in.ServiceInfo.Properties != nil {
		err := protoutil.UnmarshalStruct(in.ServiceInfo.Properties, &props)
		if err != nil {
			return errors.Wrap(err, "unmarshalling serviceinfo.properties")
		}
	}

	cid := props.ClusterID
	if cid == "" {
		cid = DefaultClusterId
	}
	dp := props.DiscoverPrefix
	if dp == "" {
		dp = DefaultDiscoverPrefix
	}
	if out.Metadata == nil {
		out.Metadata = &envoycore.Metadata{}
	}
	common.InitFilterMetadataField(filterName, ClusterId, out.Metadata).Kind = &types.Value_StringValue{StringValue: cid}
	common.InitFilterMetadataField(filterName, DiscoverPrefix, out.Metadata).Kind = &types.Value_StringValue{StringValue: dp}

	p.filters = append(p.filters, plugins.StagedHttpFilter{HttpFilter: &envoyhttp.HttpFilter{Name: filterName, Config: natsConfig(out.Name)}, Stage: pluginStage})

	return nil
}

func natsConfig(cluster string) *types.Struct {
	natsStreaming := NatsStreaming{
		MaxConnections: 1,
		Cluster:        cluster,
	}

	filterConfig, err := protoutil.MarshalStruct(&natsStreaming)
	if err != nil {
		log.Warnf("error in nats plugin: %v", err)
		return nil
	}
	return filterConfig
}

func (p *Plugin) ParseFunctionSpec(params *plugins.FunctionPluginParams, in v1.FunctionSpec) (*types.Struct, error) {
	if params.ServiceType != ServiceTypeNatsStreaming {
		return nil, nil
	}
	return nil, errors.New("functions are not required for service type " + ServiceTypeNatsStreaming)
}

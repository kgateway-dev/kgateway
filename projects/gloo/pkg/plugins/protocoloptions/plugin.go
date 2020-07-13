package protocoloptions

import (
	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/golang/protobuf/ptypes/wrappers"
	errors "github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

const MinWindowSize = 65535
const MaxWindowSize = 2147483647

type Plugin struct {
}

// Compile-time assertion
var _ plugins.Plugin = &Plugin{}
var _ plugins.UpstreamPlugin = &Plugin{}

func NewPlugin() *Plugin {
	return &Plugin{}
}

func (p *Plugin) Init(params plugins.InitParams) error {
	return nil
}

func (p *Plugin) ProcessUpstream(params plugins.Params, in *v1.Upstream, out *envoyapi.Cluster) error {

	if in.GetUseHttp2() == nil || !in.GetUseHttp2().Value {
		return nil
	}

	if out.Http2ProtocolOptions == nil {
		out.Http2ProtocolOptions = &envoycore.Http2ProtocolOptions{}
	}

	// Both these values default to 268435456 if unset.
	sws := in.GetInitialStreamWindowSize()
	if sws != nil {
		if validateWindowSize(sws.Value) {
			out.Http2ProtocolOptions.InitialStreamWindowSize = &wrappers.UInt32Value{Value: sws.Value}
		} else {
			return errors.Errorf("Invalid Initial Steam Window Size: %d", sws.Value)
		}
	}

	cws := in.GetInitialConnectionWindowSize()
	if cws != nil {
		if validateWindowSize(cws.Value) {
			out.Http2ProtocolOptions.InitialConnectionWindowSize = &wrappers.UInt32Value{Value: cws.Value}
		} else {
			return errors.Errorf("Invalid Initial Connection Window Size: %d", cws.Value)
		}
	}

	return nil
}

func validateWindowSize(size uint32) bool {
	if size < MinWindowSize || size > MaxWindowSize {
		return false
	}
	return true
}

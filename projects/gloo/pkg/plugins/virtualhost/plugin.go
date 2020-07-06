package virtualhost

import (
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

type Plugin struct {
}

// Compile-time assertion
var _ plugins.Plugin = &Plugin{}
var _ plugins.VirtualHostPlugin = &Plugin{}

func NewPlugin() *Plugin {
	return &Plugin{}
}

func (p *Plugin) Init(params plugins.InitParams) error {
	return nil
}

func (p *Plugin) ProcessVirtualHost(params plugins.VirtualHostParams, in *v1.VirtualHost, out *envoyroute.VirtualHost) error {
	// Both these values default to false if unset, so there's need to set anything if input is nil.
	// (Input is a google.protobuf.BoolValue, so it can be true, false, or nil)
	if irac := in.GetOptions().GetIncludeRequestAttemptCount(); irac != nil {
		out.IncludeRequestAttemptCount = irac.Value
	}
	if iacir := in.GetOptions().GetIncludeAttemptCountInResponse(); iacir != nil {
		out.IncludeAttemptCountInResponse = iacir.Value
	}
	return nil
}

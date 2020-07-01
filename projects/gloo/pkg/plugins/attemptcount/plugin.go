package attemptcount

import (
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

// todo what're those???
const (
	ExtensionName      = "attempt-count"
	EnvoyExtensionName = "envoy-attempt-count"
	CustomDomain       = "custom"
	requestType        = "both"

	customStage    = 1
)

type Plugin struct {
}

func NewPlugin() *Plugin {
	return &Plugin{}
}

func (p *Plugin) Init(params plugins.InitParams) error {
	return nil
}

func (p *Plugin) ProcessVirtualHost(params plugins.VirtualHostParams, in *v1.VirtualHost, out *envoyroute.VirtualHost) error {
	// both these values default to false if unset in envoy, so no need to set anything if input is nil.
	if irac := in.GetOptions().GetIncludeRequestAttemptCount(); irac != nil {
		out.IncludeRequestAttemptCount = irac.Value
	}
	if irac := in.GetOptions().GetIncludeAttemptCountInResponse(); irac != nil {
		out.IncludeAttemptCountInResponse = irac.Value
	}
	return nil
}

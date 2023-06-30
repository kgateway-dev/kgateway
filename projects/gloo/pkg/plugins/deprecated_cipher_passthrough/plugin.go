package deprecated_cipher_passthrough

import (
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/enterprise_warning"
)

var (
	_ plugins.Plugin         = new(plugin)
	_ plugins.ListenerPlugin = new(plugin)
)

const (
	ExtensionName = "deprecated_cipher_passthrough"
)

type plugin struct{}

func NewPlugin() *plugin {
	return &plugin{}
}

func (p *plugin) Init(_ plugins.InitParams) {
}

func (p *plugin) Name() string {
	return ExtensionName
}

func (p *plugin) ProcessListener(params plugins.Params, in *v1.Listener, out *envoy_config_listener_v3.Listener) error {
	if usesEnterpriseOnlyFeatures(in) {
		return enterprise_warning.GetErrorForEnterpriseOnlyExtensions([]string{ExtensionName})
	}

	return nil
}

// usesEnterpriseOnlyFeatures returns true if the listener uses passthrough ciphers
func usesEnterpriseOnlyFeatures(in *v1.Listener) bool {
	matchedListeners := in.GetHybridListener().GetMatchedListeners()
	for _, matchedListener := range matchedListeners {
		if len(matchedListener.GetMatcher().GetPassthroughCipherSuites()) > 0 {
			return true
		}
	}
	return false
}

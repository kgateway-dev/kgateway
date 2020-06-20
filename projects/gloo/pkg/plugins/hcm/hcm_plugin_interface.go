package hcm

import (
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/hcm"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

// Other plugins may implement this interface if they need to make modifications to a listener's HttpConnectionManager
// settings
type HcmPlugin interface {
	plugins.Plugin
	ProcessHcmSettings(cfg *envoyhttp.HttpConnectionManager, settings *hcm.HttpConnectionManagerSettings) error
}

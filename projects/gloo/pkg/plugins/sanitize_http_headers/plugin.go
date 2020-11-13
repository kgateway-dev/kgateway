package sanitize_http_headers

import (
	"github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/extauth"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

const (
	ExtensionName      = "sanitize_http_headers"
	SanitizeFilterName = "io.solo.filters.http.sanitize"
)

var (
	sanitizeFilterStage = plugins.BeforeStage(plugins.AuthNStage)
)

type Plugin struct {
}

var _ plugins.Plugin = new(Plugin)

func NewPlugin() *Plugin {
	return &Plugin{}
}

func (p *Plugin) Init(params plugins.InitParams) error {
	return nil
}

func (p *Plugin) HttpFilters(params plugins.Params, listener *v1.HttpListener) ([]plugins.StagedHttpFilter, error) {
	var filters []plugins.StagedHttpFilter

	// check if we should sanitize for cluster_header
	if sanitizeClusterHeader := listener.GetOptions().GetSanitizeClusterHeader(); sanitizeClusterHeader == nil || sanitizeClusterHeader.GetValue() == false {
		return filters, nil
	}

	headersToRemoveSet := make(map[string]int)

	// get all headers used for cluster_header destination
	hosts := listener.GetVirtualHosts()
	for _, host := range hosts {
		routes := host.GetRoutes()
		for _, route := range routes {
			if header := route.GetRouteAction().GetClusterHeader(); header != "" {
				headersToRemoveSet[header] = 0
			}
		}
	}
	// sanitize those headers from downstreams
	if len(headersToRemoveSet) > 0 {
		var headersToRemove []string
		for header := range headersToRemoveSet {
			headersToRemove = append(headersToRemove, header)
		}
		sanitizeConf := &extauth.Sanitize{HeadersToRemove: headersToRemove}
		stagedFilter, err := plugins.NewStagedFilterWithConfig(SanitizeFilterName, sanitizeConf, sanitizeFilterStage)
		if err != nil {
			return nil, err
		}
		filters = append(filters, stagedFilter)
	}
	return filters, nil
}

package registry

import (
	"fmt"

	"github.com/solo-io/gloo/v2/pkg/translator/httproute/filterplugins/mirror"
	"github.com/solo-io/gloo/v2/pkg/translator/httproute/filterplugins/redirect"
	"github.com/solo-io/gloo/v2/pkg/translator/httproute/filterplugins/urlrewrite"

	"github.com/solo-io/gloo/v2/pkg/translator/httproute/filterplugins"
	"github.com/solo-io/gloo/v2/pkg/translator/httproute/filterplugins/headermodifier"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type HTTPFilterPluginRegistry interface {
	GetStandardPlugin(filterType gwv1.HTTPRouteFilterType) (filterplugins.FilterPlugin, error)
	GetExtensionPlugin(extensionRef *gwv1.LocalObjectReference) (filterplugins.FilterPlugin, error)
}

type httpFilterPluginRegistry struct {
	standardPlugins  map[gwv1.HTTPRouteFilterType]filterplugins.FilterPlugin
	extensionPlugins map[schema.GroupKind]filterplugins.FilterPlugin
}

func (h *httpFilterPluginRegistry) GetStandardPlugin(filterType gwv1.HTTPRouteFilterType) (filterplugins.FilterPlugin, error) {
	p, ok := h.standardPlugins[filterType]
	if !ok {
		return nil, fmt.Errorf("")
	}
	return p, nil
}

func (h *httpFilterPluginRegistry) GetExtensionPlugin(extensionRef *gwv1.LocalObjectReference) (filterplugins.FilterPlugin, error) {
	if extensionRef == nil {
		return nil, fmt.Errorf("extension ref is required")
	}
	p, ok := h.extensionPlugins[schema.GroupKind{
		Group: string(extensionRef.Group),
		Kind:  string(extensionRef.Kind),
	}]
	if !ok {
		return nil, fmt.Errorf("no support for extension ref %+v", extensionRef)
	}
	return p, nil
}

func NewHTTPFilterPluginRegistry() HTTPFilterPluginRegistry {
	return &httpFilterPluginRegistry{
		standardPlugins: map[gwv1.HTTPRouteFilterType]filterplugins.FilterPlugin{
			gwv1.HTTPRouteFilterRequestHeaderModifier:  headermodifier.NewPlugin(),
			gwv1.HTTPRouteFilterResponseHeaderModifier: headermodifier.NewPlugin(),
			gwv1.HTTPRouteFilterURLRewrite:             urlrewrite.NewPlugin(),
			gwv1.HTTPRouteFilterRequestRedirect:        redirect.NewPlugin(),
			gwv1.HTTPRouteFilterRequestMirror:          mirror.NewPlugin(),
		},
		extensionPlugins: map[schema.GroupKind]filterplugins.FilterPlugin{},
	}
}

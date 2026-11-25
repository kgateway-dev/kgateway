package admin

import (
	"cmp"
	"net/http"

	istiogvk "istio.io/istio/pkg/config/schema/gvk"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/version"
)

type watchedResource struct {
	Group    string `json:"group"`
	Version  string `json:"version"`
	Resource string `json:"resource"`
	Kind     string `json:"kind,omitempty"`
}

var additionalKinds = map[schema.GroupVersionResource]schema.GroupVersionKind{
	wellknown.BackendGVR:             wellknown.BackendGVK,
	wellknown.BackendConfigPolicyGVR: wellknown.BackendConfigPolicyGVK,
	wellknown.DirectResponseGVR:      wellknown.DirectResponseGVK,
	wellknown.GatewayExtensionGVR:    wellknown.GatewayExtensionGVK,
	wellknown.GatewayParametersGVR:   wellknown.GatewayParametersGVK,
	wellknown.ListenerPolicyGVR:      wellknown.ListenerPolicyGVK,
	wellknown.TrafficPolicyGVR:       wellknown.TrafficPolicyGVK,
	wellknown.TCPRouteGVR:            wellknown.TCPRouteGVK,
	wellknown.TLSRouteGVR:            wellknown.TLSRouteGVK,
	wellknown.TLSRouteV1Alpha3GVR:    wellknown.TLSRouteV1Alpha3GVK,
	wellknown.XListenerSetGVR:        wellknown.XListenerSetGVK,
}

// NewDiscoveryScopeHandler serves the discovery scope on its own HTTP mux.
func NewDiscoveryScopeHandler(
	common *collections.CommonCollections,
	controllerNames []string,
	resources func() []schema.GroupVersionResource,
) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /discovery-scope", func(w http.ResponseWriter, r *http.Request) {
		selectors, namespaces := common.DiscoveryScope()
		watched := resources()
		kinds := make([]watchedResource, 0, len(watched))
		for _, resource := range watched {
			kind := additionalKinds[resource].Kind
			if known, ok := istiogvk.FromGVR(resource); ok {
				kind = known.Kind
			}
			kinds = append(kinds, watchedResource{Group: resource.Group, Version: resource.Version, Resource: resource.Resource, Kind: kind})
		}
		writeJSON(w, struct {
			Controller      string            `json:"controller"`
			Version         string            `json:"version"`
			ControllerNames []string          `json:"controllerNames"`
			Selectors       []string          `json:"selectors"`
			Namespaces      []string          `json:"namespaces"`
			Kinds           []watchedResource `json:"kinds"`
		}{
			Controller:      "kgateway",
			Version:         cmp.Or(version.Version, version.UndefinedVersion),
			ControllerNames: controllerNames,
			Selectors:       selectors,
			Namespaces:      namespaces,
			Kinds:           kinds,
		}, r)
	})
	return mux
}

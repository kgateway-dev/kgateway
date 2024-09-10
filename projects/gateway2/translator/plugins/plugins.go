package plugins

import (
	"container/list"
	"context"

	"github.com/solo-io/gloo/projects/gateway2/reports"
	"github.com/solo-io/gloo/projects/gateway2/translator/translatorutils"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"k8s.io/apimachinery/pkg/types"

	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// Plugin is an empty type for base plugins, currently no base methods.
type Plugin interface{}

type RouteContext struct {
	// top-level gw Listener
	Listener *gwv1.Listener
	// top-level HTTPRoute
	Route *gwv1.HTTPRoute
	// Hostnames associated with the Route.
	// Note: this should be used over Route.spec.Hostnames as
	// delegatee (child) routes of delegated routes will not have spec.Hostnames set.
	Hostnames []gwv1.Hostname
	// DelegationChain is a doubly linked list containing the delegation chain from child to its ancestors
	// excluding the child itself, where the elements are the DelegationCtx type
	DelegationChain *list.List
	// specific HTTPRouteRule of the HTTPRoute being processed, nil if the entire HTTPRoute is being processed
	// rather than just a specific Rule
	Rule *gwv1.HTTPRouteRule
	// specific Match of the Rule being processed (as there may be multiple Matches per Rule), nil if there no Match
	// for this context, such as when an entire HTTPRoute is being processed
	Match *gwv1.HTTPRouteMatch
	// Reporter for the correct ParentRef associated with this HTTPRoute
	Reporter reports.ParentRefReporter
}

type DelegationCtx struct {
	Ref types.NamespacedName
}

type RoutePlugin interface {
	Plugin

	// ApplyRoutePlugin is called for each Match in a given Rule
	//
	// For delegatee/child routes, this will be called multiple times for
	// each route in the delegation chain starting from the child to the parent
	// up the chain. Plugins may choose to override the existing configuration
	// associated with a route when the plugin is invoked multiple times on the
	// same route but with different configuration.
	ApplyRoutePlugin(
		ctx context.Context,
		routeCtx *RouteContext,
		outputRoute *v1.Route,
	) error
}

type ListenerContext struct {
	// top-level Gateway
	Gateway *gwv1.Gateway
	// gw Listener being processed
	GwListener *gwv1.Listener
}
type ListenerPlugin interface {
	Plugin

	// ApplyListenerPlugin is called for each Listener in a Gateway
	ApplyListenerPlugin(
		ctx context.Context,
		listenerCtx *ListenerContext,
		outputListener *v1.Listener,
	) error
}

type PostTranslationContext struct {
	// TranslatedGateways is the list of Gateways that were generated in a single translation run
	TranslatedGateways []TranslatedGateway
}

type TranslatedGateway struct {
	// Gateway is the input object that produced the Proxy
	Gateway gwv1.Gateway
}

type PostTranslationPlugin interface {
	Plugin

	// ApplyPostTranslationPlugin is executed once at the end of a Gateway API translation run
	ApplyPostTranslationPlugin(
		ctx context.Context,
		postTranslationContext *PostTranslationContext,
	) error
}

type StatusContext struct {
	ProxiesWithReports []translatorutils.ProxyWithReports
}

// Plugin that recieves proxy reports post-xds translation to handle any status reporting necessary
type StatusPlugin interface {
	Plugin

	ApplyStatusPlugin(
		ctx context.Context,
		statusCtx *StatusContext,
	) error
}

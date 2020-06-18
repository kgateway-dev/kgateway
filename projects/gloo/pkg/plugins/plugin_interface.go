package plugins

import (
	"context"
	"sort"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoylistener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	envoyendpoint "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

type InitParams struct {
	Ctx      context.Context
	Settings *v1.Settings
}

type Plugin interface {
	Init(params InitParams) error
}

type Params struct {
	Ctx      context.Context
	Snapshot *v1.ApiSnapshot
}

type VirtualHostParams struct {
	Params
	Proxy    *v1.Proxy
	Listener *v1.Listener
}

type RouteParams struct {
	VirtualHostParams
	VirtualHost *v1.VirtualHost
}

type RouteActionParams struct {
	RouteParams
	Route *v1.Route
}

/*
	Upstream Plugins
*/

// UpstreamPlugin is called after the envoy Cluster has been created for the input Upstream, and allows
// the cluster to be edited before being sent to envoy via CDS
type UpstreamPlugin interface {
	Plugin
	ProcessUpstream(params Params, in *v1.Upstream, out *envoyapi.Cluster) error
}

// Endpoint is called after the envoy ClusterLoadAssignment has been created for the input Upstream, and allows
// the endpoints to be edited before being sent to envoy via EDS
// If one wishes to also modify the corresponding envoy Cluster the above UpstreamPlugin interface should be used.
type EndpointPlugin interface {
	Plugin
	ProcessEndpoints(params Params, in *v1.Upstream, out *envoyendpoint.ClusterLoadAssignment) error
}

/*
	Routing Plugins
*/

type RoutePlugin interface {
	Plugin
	ProcessRoute(params RouteParams, in *v1.Route, out *envoyroute.Route) error
}

// note: any route action plugin can be implemented as a route plugin
// suggestion: if your plugin requires configuration from a RoutePlugin field, implement the RoutePlugin interface
type RouteActionPlugin interface {
	Plugin
	ProcessRouteAction(params RouteActionParams, inAction *v1.RouteAction, out *envoyroute.RouteAction) error
}

type WeightedDestinationPlugin interface {
	Plugin
	ProcessWeightedDestination(params RouteParams, in *v1.WeightedDestination, out *envoyroute.WeightedCluster_ClusterWeight) error
}

/*
	Listener Plugins
*/

type ListenerPlugin interface {
	Plugin
	ProcessListener(params Params, in *v1.Listener, out *envoyapi.Listener) error
}

type ListenerFilterPlugin interface {
	Plugin
	ProcessListenerFilter(params Params, in *v1.Listener) ([]StagedListenerFilter, error)
}

type StagedListenerFilter struct {
	ListenerFilter *envoylistener.Filter
	Stage          FilterStage
}

type StagedListenerFilterList []StagedListenerFilter

func (s StagedListenerFilterList) Len() int {
	return len(s)
}

// filters by Relative Stage, Weighting, Name, and (to ensure stability) index
func (s StagedListenerFilterList) Less(i, j int) bool {
	switch FilterStageComparison(s[i].Stage, s[j].Stage) {
	case -1:
		return true
	case 1:
		return false
	}
	if s[i].ListenerFilter.Name < s[j].ListenerFilter.Name {
		return true
	}
	if s[i].ListenerFilter.Name > s[j].ListenerFilter.Name {
		return false
	}
	if s[i].ListenerFilter.String() < s[j].ListenerFilter.String() {
		return true
	}
	if s[i].ListenerFilter.String() > s[j].ListenerFilter.String() {
		return false
	}
	// ensure stability
	return i < j
}

func (s StagedListenerFilterList) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

// Currently only supported for TCP listeners, plan to change this in the future
type ListenerFilterChainPlugin interface {
	Plugin
	ProcessListenerFilterChain(params Params, in *v1.Listener) ([]*envoylistener.FilterChain, error)
}

type HttpFilterPlugin interface {
	Plugin
	HttpFilters(params Params, listener *v1.HttpListener) ([]StagedHttpFilter, error)
}

type VirtualHostPlugin interface {
	Plugin
	ProcessVirtualHost(params VirtualHostParams, in *v1.VirtualHost, out *envoyroute.VirtualHost) error
}

type StagedHttpFilter struct {
	HttpFilter *envoyhttp.HttpFilter
	Stage      FilterStage
}

type StagedHttpFilterList []StagedHttpFilter

func (s StagedHttpFilterList) Len() int {
	return len(s)
}

// filters by Relative Stage, Weighting, Name, and (to ensure stability) index
func (s StagedHttpFilterList) Less(i, j int) bool {
	switch FilterStageComparison(s[i].Stage, s[j].Stage) {
	case -1:
		return true
	case 1:
		return false
	}
	if s[i].HttpFilter.Name < s[j].HttpFilter.Name {
		return true
	}
	if s[i].HttpFilter.Name > s[j].HttpFilter.Name {
		return false
	}
	if s[i].HttpFilter.String() < s[j].HttpFilter.String() {
		return true
	}
	if s[i].HttpFilter.String() > s[j].HttpFilter.String() {
		return false
	}
	// ensure stability
	return i < j
}

func (s StagedHttpFilterList) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

var _ sort.Interface = StagedHttpFilterList{}

// WellKnownFilterStages are represented by an integer that reflects their relative ordering
type WellKnownFilterStage int

// If new well known filter stages are added, they should be inserted in a position corresponding to their order
const (
	FaultStage     WellKnownFilterStage = iota // Fault injection // First Filter Stage
	CorsStage                                  // Cors stage
	WafStage                                   // Web application firewall stage
	AuthNStage                                 // Authentication stage
	AuthZStage                                 // Authorization stage
	RateLimitStage                             // Rate limiting stage
	AcceptedStage                              // Request passed all the checks and will be forwarded upstream
	OutAuthStage                               // Add auth for the upstream (i.e. aws λ)
	RouteStage                                 // Request is going to upstream // Last Filter Stage
)

type FilterStage struct {
	RelativeTo WellKnownFilterStage
	Weight     int
}

// FilterStageComparison helps implement the sort.Interface Less function for use in other implementations of sort.Interface
// returns -1 if less than, 0 if equal, 1 if greater than
// It is not sufficient to return a Less bool because calling functions need to know if equal or greater when Less is false
func FilterStageComparison(a, b FilterStage) int {
	if a.RelativeTo < b.RelativeTo {
		return -1
	} else if a.RelativeTo > b.RelativeTo {
		return 1
	}
	if a.Weight < b.Weight {
		return -1
	} else if a.Weight > b.Weight {
		return 1
	}
	return 0
}

func BeforeStage(wellKnown WellKnownFilterStage) FilterStage {
	return RelativeToStage(wellKnown, -1)
}
func DuringStage(wellKnown WellKnownFilterStage) FilterStage {
	return RelativeToStage(wellKnown, 0)
}
func AfterStage(wellKnown WellKnownFilterStage) FilterStage {
	return RelativeToStage(wellKnown, 1)
}
func RelativeToStage(wellKnown WellKnownFilterStage, weight int) FilterStage {
	return FilterStage{
		RelativeTo: wellKnown,
		Weight:     weight,
	}
}

/*
	Generation plugins
*/
type ClusterGeneratorPlugin interface {
	Plugin
	GeneratedClusters(params Params) ([]*envoyapi.Cluster, error)
}

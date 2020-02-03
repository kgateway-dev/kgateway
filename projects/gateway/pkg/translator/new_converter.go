package translator

import (
	"fmt"
	"github.com/gogo/protobuf/proto"
	errors "github.com/rotisserie/eris"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	matchersv1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	glooutils "github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
	"strings"
)

var (
	NoActionErr         = errors.New("invalid route: route must specify an action")
	MatcherCountErr     = errors.New("invalid route: routes with delegate actions must omit or specify a single matcher")
	MissingPrefixErr    = errors.New("invalid route: routes with delegate actions must use a prefix matcher")
	InvalidPrefixErr    = errors.New("invalid route: route table matchers must begin with the prefix of their parent route's matcher")
	HasHeaderMatcherErr = errors.New("invalid route: routes with delegate actions cannot use header matchers")
	HasMethodMatcherErr = errors.New("invalid route: routes with delegate actions cannot use method matchers")
	HasQueryMatcherErr  = errors.New("invalid route: routes with delegate actions cannot use query matchers")
	DelegationCycleErr  = func(cycleInfo string) error {
		return errors.Errorf("invalid route: delegation cycle detected: %s", cycleInfo)
	}
	InvalidRouteTableForDelegateErr = func(delegatePrefix, pathString string) error {
		return errors.Wrapf(InvalidPrefixErr, "required prefix: %v, path: %v", delegatePrefix, pathString)
	}
)

// We define this interface to abstract both virtual services and route tables
type ResourceWithRoutes interface {
	InputResource() resources.InputResource
	GetRoutes() []*gatewayv1.Route
}

type ConvertibleVirtualService struct {
	*gatewayv1.VirtualService
}

func (v *ConvertibleVirtualService) GetRoutes() []*gatewayv1.Route {
	return v.GetVirtualHost().GetRoutes()
}

func (v *ConvertibleVirtualService) InputResource() resources.InputResource {
	return v.VirtualService
}

type ConvertibleRouteTable struct {
	*gatewayv1.RouteTable
}

func (v *ConvertibleRouteTable) InputResource() resources.InputResource {
	return v.RouteTable
}

type RouteConverter interface {
	// Converts a Gateway API resource with routes (i.e. a VirtualService or RouteTable)
	// to a set of Gloo API routes (i.e. routes on a Proxy resource).
	ConvertRoute(resource ResourceWithRoutes) ([]*gloov1.Route, error)
}

func NewRouteConverter(selector RouteTableSelector, reports reporter.ResourceReports) RouteConverter {
	return &routeVisitor{
		reports:            reports,
		routeTableSelector: selector,
	}
}

func (rv *routeVisitor) ConvertRoute(resource ResourceWithRoutes) ([]*gloov1.Route, error) {
	return rv.collectRoutes(resource, nil, nil)
}

// Implements the RouteConverter interface by recursively visiting a route tree
type routeVisitor struct {
	// Used to store of errors and warnings for the root resource. This object will be passed to sub-visitors.
	reports reporter.ResourceReports
	//
	routeTableSelector RouteTableSelector
}

type routeInfo struct {
	// The path prefix for the route
	prefix string
	// The options on the route
	options *gloov1.RouteOptions
	// Used to build the name of the route as we traverse the tree
	name string
	// Is true if any route on the current branch is explicitly named by the user
	containsNamedRoute bool
}

func (rv *routeVisitor) collectRoutes(resource ResourceWithRoutes, parentRoute *routeInfo, visitedRouteTables gatewayv1.RouteTableList) ([]*gloov1.Route, error) {
	var routes []*gloov1.Route

	for _, gatewayRoute := range resource.GetRoutes() {

		// Clone route to be safe, since we might mutate it
		routeClone := proto.Clone(gatewayRoute).(*gatewayv1.Route)

		// Set route name
		name, routeHasName := routeName(resource.InputResource(), routeClone, parentRoute)
		routeClone.Name = name

		containsNamedRoute := routeHasName
		if parentRoute != nil {
			containsNamedRoute = containsNamedRoute || parentRoute.containsNamedRoute
		}

		// If the parent route is not nil, this route has been delegated to and we need to perform additional operations
		if parentRoute != nil {
			var err error
			routeClone, err = mergeParentRoute(routeClone, parentRoute)
			if err != nil {
				rv.reports.AddError(resource.InputResource(), err)
				continue
			}
		}

		switch action := routeClone.Action.(type) {
		case *gatewayv1.Route_DelegateAction:

			// Validate the matcher of the delegate route
			prefix, err := getDelegateRoutePrefix(routeClone)
			if err != nil {
				return nil, err
			}

			// Determine the route tables to delegate to
			routeTables, err := rv.routeTableSelector.SelectRouteTables(action.DelegateAction, resource.InputResource().GetMetadata().Namespace)
			if err != nil {
				// Only return warning here
				rv.reports.AddWarning(resource.InputResource(), err.Error())
				// TODO: continue?
				return nil, nil
			}

			for _, routeTable := range routeTables {

				// Check for delegation cycles
				if err := checkForCycles(routeTable, visitedRouteTables); err != nil {
					return nil, err
				}

				currentRouteInfo := &routeInfo{
					prefix:             prefix,
					options:            routeClone.Options,
					name:               name,
					containsNamedRoute: containsNamedRoute,
				}

				// Make a copy of the existing set of visited route tables and pass that into the recursive call.
				// We do NOT want it to be modified.
				visitedRtCopy := append(append([]*gatewayv1.RouteTable{}, visitedRouteTables...), routeTable)

				subRoutes, err := rv.collectRoutes(&ConvertibleRouteTable{routeTable}, currentRouteInfo, visitedRtCopy)
				if err != nil {
					return nil, err
				}

				routes = append(routes, subRoutes...)
			}
		default:

			// If there are no named routes in the tree, wipe the name
			if !containsNamedRoute {
				routeClone.Name = ""
			}

			glooRoute, err := convertSimpleAction(routeClone)
			if err != nil {
				return nil, err
			}
			routes = append(routes, glooRoute)
		}
	}

	for _, r := range routes {
		if err := appendSource(r, resource.InputResource()); err != nil {
			// should never happen
			return nil, err
		}
	}

	glooutils.SortRoutesByPath(routes)

	return routes, nil
}

// Ex name: "vs:myvirtualservice_route:myfirstroute_rt:myroutetable_route:<unnamed>"
func routeName(resource resources.InputResource, route *gatewayv1.Route, parentRouteInfo *routeInfo) (string, bool) {
	var prefix string
	if parentRouteInfo != nil {
		prefix = parentRouteInfo.name + "_"
	}

	resourceKindName := ""
	switch resource.(type) {
	case *gatewayv1.VirtualService:
		resourceKindName = "vs"
	case *gatewayv1.RouteTable:
		resourceKindName = "rt"
	}
	resourceName := resource.GetMetadata().Name

	var isRouteNamed bool
	routeDisplayName := route.Name
	if routeDisplayName == "" {
		routeDisplayName = "<unnamed>"
	} else {
		isRouteNamed = true
	}

	return fmt.Sprintf("%s%s:%s_route:%s", prefix, resourceKindName, resourceName, routeDisplayName), isRouteNamed
}

func convertSimpleAction(simpleRoute *gatewayv1.Route) (*gloov1.Route, error) {
	matchers := []*matchersv1.Matcher{defaults.DefaultMatcher()}
	if len(simpleRoute.Matchers) > 0 {
		matchers = simpleRoute.Matchers
	}

	glooRoute := &gloov1.Route{
		Matchers: matchers,
		Options:  simpleRoute.Options,
		Name:     simpleRoute.Name,
	}

	switch action := simpleRoute.Action.(type) {
	case *gatewayv1.Route_RedirectAction:
		glooRoute.Action = &gloov1.Route_RedirectAction{
			RedirectAction: action.RedirectAction,
		}
	case *gatewayv1.Route_DirectResponseAction:
		glooRoute.Action = &gloov1.Route_DirectResponseAction{
			DirectResponseAction: action.DirectResponseAction,
		}
	case *gatewayv1.Route_RouteAction:
		glooRoute.Action = &gloov1.Route_RouteAction{
			RouteAction: action.RouteAction,
		}
	default:
		return nil, NoActionErr
	}

	return glooRoute, nil
}

// If any of the matching route tables has already been visited, that means we have a delegation cycle.
func checkForCycles(toVisit *gatewayv1.RouteTable, visited gatewayv1.RouteTableList) error {
	for _, alreadyVisitedTable := range visited {
		if toVisit == alreadyVisitedTable {
			return DelegationCycleErr(
				buildCycleInfoString(append(append(gatewayv1.RouteTableList{}, visited...), toVisit)),
			)
		}
	}
	return nil
}

func getDelegateRoutePrefix(route *gatewayv1.Route) (string, error) {
	switch len(route.GetMatchers()) {
	case 0:
		return defaults.DefaultMatcher().GetPrefix(), nil
	case 1:
		matcher := route.GetMatchers()[0]
		var prefix string
		if len(matcher.GetHeaders()) > 0 {
			return prefix, HasHeaderMatcherErr
		}
		if len(matcher.GetMethods()) > 0 {
			return prefix, HasMethodMatcherErr
		}
		if len(matcher.GetQueryParameters()) > 0 {
			return prefix, HasQueryMatcherErr
		}
		if matcher.GetPathSpecifier() == nil {
			return defaults.DefaultMatcher().GetPrefix(), nil // no path specifier provided, default to '/' prefix matcher
		}
		prefix = matcher.GetPrefix()
		if prefix == "" {
			return prefix, MissingPrefixErr
		}
		return prefix, nil
	default:
		return "", MatcherCountErr
	}
}

func mergeParentRoute(child *gatewayv1.Route, parent *routeInfo) (*gatewayv1.Route, error) {

	// Verify that the matchers are compatible with the parent prefix
	if err := isRouteTableValidForDelegatePrefix(parent.prefix, child); err != nil {
		return nil, err
	}

	// Merge plugins from parent routes
	merged, err := mergeRoutePlugins(child.GetOptions(), parent.options)
	if err != nil {
		// Should never happen
		return nil, errors.Wrapf(err, "internal error: merging route plugins from parent to delegated route")
	}

	child.Options = merged

	return child, nil
}

func isRouteTableValidForDelegatePrefix(delegatePrefix string, route *gatewayv1.Route) error {
	for _, match := range route.Matchers {
		// ensure all sub-routes in the delegated route table match the parent prefix
		if pathString := glooutils.PathAsString(match); !strings.HasPrefix(pathString, delegatePrefix) {
			return InvalidRouteTableForDelegateErr(delegatePrefix, pathString)
		}
	}
	return nil
}

// Handles new and deprecated format for referencing a route table
// TODO: remove this function when we remove the deprecated fields from the API
func getRouteTableRef(delegate *gatewayv1.DelegateAction) *core.ResourceRef {
	if delegate.Namespace != "" || delegate.Name != "" {
		return &core.ResourceRef{
			Namespace: delegate.Namespace,
			Name:      delegate.Name,
		}
	}
	return delegate.GetRef()
}

func buildCycleInfoString(routeTables gatewayv1.RouteTableList) string {
	var visitedTables []string
	for _, rt := range routeTables {
		visitedTables = append(visitedTables, fmt.Sprintf("[%s]", rt.Metadata.Ref().Key()))
	}
	return strings.Join(visitedTables, " -> ")
}

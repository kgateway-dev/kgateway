package translator

import (
	"fmt"
	"strings"

	"github.com/gogo/protobuf/types"

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
)

const (
	unnamedRouteName   = "<unnamed>"
	defaultTableWeight = 0
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
	TopLevelVirtualResourceErr = func(rtRef core.Metadata, err error) error {
		return errors.Wrapf(err, "on sub route table %v.%v", rtRef.Name, rtRef.Namespace)
	}
)

type RouteConverter interface {
	// Converts a VirtualService to a set of Gloo API routes (i.e. routes on a Proxy resource).
	ConvertVirtualService(virtualService *gatewayv1.VirtualService) ([]*gloov1.Route, error)
}

func NewRouteConverter(selector RouteTableSelector, indexer RouteTableIndexer, reports reporter.ResourceReports) RouteConverter {
	return &routeVisitor{
		reports:            reports,
		routeTableSelector: selector,
		routeTableIndexer:  indexer,
	}
}

// We define this interface to abstract both virtual services and route tables.
type resourceWithRoutes interface {
	InputResource() resources.InputResource
	GetRoutes() []*gatewayv1.Route
}

type visitableVirtualService struct {
	*gatewayv1.VirtualService
}

func (v *visitableVirtualService) GetRoutes() []*gatewayv1.Route {
	return v.GetVirtualHost().GetRoutes()
}

func (v *visitableVirtualService) InputResource() resources.InputResource {
	return v.VirtualService
}

type visitableRouteTable struct {
	*gatewayv1.RouteTable
}

func (v *visitableRouteTable) InputResource() resources.InputResource {
	return v.RouteTable
}

// Implements Converter interface by recursively visiting a routing resource
type routeVisitor struct {
	// Used to store errors and warnings for the visited virtual services and route tables.
	reports reporter.ResourceReports
	// Used to select route tables for delegated routes.
	routeTableSelector RouteTableSelector
	// Used to sort route tables when multiple ones are matched by a selector.
	routeTableIndexer RouteTableIndexer
}

// Helper object used to store information about previously visited routes.
type routeInfo struct {
	// The path prefix for the route.
	prefix string
	// The options on the route.
	options *gloov1.RouteOptions
	// Used to build the name of the route as we traverse the tree.
	name string
	// Is true if any route on the current route tree branch is explicitly named by the user.
	hasName bool
}

func (rv *routeVisitor) ConvertVirtualService(virtualService *gatewayv1.VirtualService) ([]*gloov1.Route, error) {
	wrapper := &visitableVirtualService{VirtualService: virtualService}
	return rv.visit(wrapper, nil, nil, virtualService)
}

// Performs a depth-first, in-order traversal of a route tree rooted at the given resource.
// The additional arguments are used to store the state of the traversal of the current branch of the route tree.
func (rv *routeVisitor) visit(resource resourceWithRoutes, parentRoute *routeInfo, visitedRouteTables gatewayv1.RouteTableList,
	topLevelVirtualService *gatewayv1.VirtualService) ([]*gloov1.Route, error) {
	var routes []*gloov1.Route

	for _, gatewayRoute := range resource.GetRoutes() {

		// Clone route to be safe, since we might mutate it
		routeClone := proto.Clone(gatewayRoute).(*gatewayv1.Route)

		// Determine route name
		name, routeHasName := routeName(resource.InputResource(), routeClone, parentRoute)
		routeClone.Name = name

		// If the parent route is not nil, this route has been delegated to and we need to perform additional operations
		if parentRoute != nil {
			var err error
			routeClone, err = validateAndMergeParentRoute(routeClone, parentRoute)
			if err != nil {
				rv.reports.AddError(resource.InputResource(), err)
				rv.reports.AddError(topLevelVirtualService,
					TopLevelVirtualResourceErr(resource.InputResource().GetMetadata(), err))
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
				rv.reports.AddWarning(resource.InputResource(), err.Error())
				if parentRoute != nil { // surface error
					rv.reports.AddWarning(topLevelVirtualService,
						TopLevelVirtualResourceErr(resource.InputResource().GetMetadata(), err).Error())
				}
				continue
			}

			// Default missing weights to 0
			for _, routeTable := range routeTables {
				if routeTable.GetWeight() == nil {
					routeTable.Weight = &types.Int32Value{Value: defaultTableWeight}
				}
			}

			routeTablesByWeight, sortedWeights := rv.routeTableIndexer.IndexByWeight(routeTables)

			// Process the route tables in order by weight
			for _, weight := range sortedWeights {
				routeTablesForWeight := routeTablesByWeight[weight]

				var rtRoutesForWeight []*gloov1.Route
				for _, routeTable := range routeTablesForWeight {

					// Check for delegation cycles
					if err := checkForCycles(routeTable, visitedRouteTables); err != nil {
						return nil, err
					}

					// Collect information about this route that are relevant when visiting the delegated route table
					currentRouteInfo := &routeInfo{
						prefix:  prefix,
						options: routeClone.Options,
						name:    name,
						hasName: routeHasName,
					}

					// Make a copy of the existing set of visited route tables. We need to pass this information into
					// the recursive call and we do NOT want the original slice to be modified.
					visitedRtCopy := append(append([]*gatewayv1.RouteTable{}, visitedRouteTables...), routeTable)

					// Recursive call
					subRoutes, err := rv.visit(&visitableRouteTable{routeTable}, currentRouteInfo, visitedRtCopy, topLevelVirtualService)
					if err != nil {
						return nil, err
					}

					rtRoutesForWeight = append(rtRoutesForWeight, subRoutes...)
				}

				// If we have multiple route tables with this weight, we want to try and sort the resulting routes in
				// order to protect against short-circuiting, e.g. we want to avoid `/foo` coming before `/foo/bar`.
				if len(routeTablesForWeight) > 1 {
					glooutils.SortRoutesByPath(rtRoutesForWeight)
				}

				routes = append(routes, rtRoutesForWeight...)
			}

		default:

			// If there are no named routes on this branch of the route tree, then wipe the name.
			if !routeHasName {
				routeClone.Name = ""
			}

			// if this is a routeAction pointing to an upstream without specifying the namespace, set the namespace to that of the parent resource
			if action, ok := routeClone.Action.(*gatewayv1.Route_RouteAction); ok {
				parentNamespace := resource.InputResource().GetMetadata().Namespace
				if upstream := action.RouteAction.GetSingle().GetUpstream(); upstream != nil && upstream.GetNamespace() == "" {
					upstream.Namespace = parentNamespace
				}
				if multiDests := action.RouteAction.GetMulti().GetDestinations(); multiDests != nil {
					for _, dest := range multiDests {
						if upstream := dest.GetDestination().GetUpstream(); upstream != nil && upstream.GetNamespace() == "" {
							upstream.Namespace = parentNamespace
						}
					}
				}
			}
			glooRoute, err := convertSimpleAction(routeClone)
			if err != nil {
				return nil, err
			}
			routes = append(routes, glooRoute)
		}
	}

	// Append source metadata to all the routes
	for _, r := range routes {
		if err := appendSource(r, resource.InputResource()); err != nil {
			// should never happen
			return nil, err
		}
	}

	return routes, nil
}

// Returns the name of the route and a flag that is true if either the route or the parent route are explicitly named.
// Route names have the following format: "vs:myvirtualservice_route:myfirstroute_rt:myroutetable_route:<unnamed>"
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
	default:
		// Should never happen
	}
	resourceName := resource.GetMetadata().Name

	var isRouteNamed bool
	routeDisplayName := route.Name
	if routeDisplayName == "" {
		routeDisplayName = unnamedRouteName
	} else {
		isRouteNamed = true
	}

	// If the current route has no name, but the parent one does, then we consider the resulting route to be named.
	isRouteNamed = isRouteNamed || (parentRouteInfo != nil && parentRouteInfo.hasName)

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
	case *gatewayv1.Route_DelegateAction:
		// Should never happen
		return nil, errors.New("internal error: expected simple route action but found delegation!")
	default:
		return nil, NoActionErr
	}

	return glooRoute, nil
}

// If any of the matching route tables has already been visited, then we have a delegation cycle.
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

func validateAndMergeParentRoute(child *gatewayv1.Route, parent *routeInfo) (*gatewayv1.Route, error) {

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

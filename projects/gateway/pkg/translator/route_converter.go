package translator

import (
	"fmt"
	"strings"

	errors "github.com/rotisserie/eris"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	matchersv1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	glooutils "github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/gogo/protobuf/proto"
)

// Reserved value for route table namespace selection.
// If a selector contains this value in its 'namespace' field, we match route tables from any namespace
const allNamespaceRouteTableSelector = "*"

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

	NoDelegateActionErr = errors.New("internal error: convertDelegateAction() called on route without delegate action")

	RouteTableMissingWarning = func(ref core.ResourceRef) string {
		return fmt.Sprintf("route table %v.%v missing", ref.Namespace, ref.Name)
	}
	NoMatchingRouteTablesWarning    = "no route table matches the given selector"
	InvalidRouteTableForDelegateErr = func(delegatePrefix, pathString string) error {
		return errors.Wrapf(InvalidPrefixErr, "required prefix: %v, path: %v", delegatePrefix, pathString)
	}
	MissingRefAndSelectorWarning = func(res resources.InputResource) string {
		ref := res.GetMetadata().Ref()
		return fmt.Sprintf("cannot determine delegation target for %T %s.%s: you must specify a route table "+
			"either via a resource reference or a selector", res, ref.Namespace, ref.Name)
	}
)

type RouteConverter interface {
	// Converts a Gateway API route (i.e. a route on RouteTables/VirtualServices)
	// to one or more Gloo API routes (i.e. routes on a Proxy resource).
	// Can return multiple routes only if the input route uses delegation.
	ConvertRoute(route *gatewayv1.Route) ([]*gloov1.Route, error)
}

// Implements the RouteConverter interface by recursively visiting a route tree
type routeVisitor struct {
	// This is the root of the subtree of routes that we are going to visit. It can be either a virtual service or a
	// route table. Errors and warnings for the current visitor will be reported on this resource.
	rootResource resources.InputResource
	// All the route tables in the current snapshot.
	tables gatewayv1.RouteTableList
	// Used to keep track of route tables that have already been visited in order to avoid cycles.
	visited gatewayv1.RouteTableList
	// Used to store of errors and warnings for the root resource. This object will be passed to sub-visitors.
	reports reporter.ResourceReports
}

// Initializes and returns a route converter instance.
// - root: root of the subtree of routes that we are going to visit; used primarily as a target to report errors and warnings on.
// - tables: all the route tables that should be considered when resolving delegation chains.
// - reports: this object will be updated with errors and warnings encountered during the conversion process.
func NewRouteVisitor(root resources.InputResource, tables gatewayv1.RouteTableList, reports reporter.ResourceReports) RouteConverter {
	return &routeVisitor{
		rootResource: root,
		tables:       tables,
		reports:      reports,
	}
}

func (rv *routeVisitor) ConvertRoute(gatewayRoute *gatewayv1.Route) ([]*gloov1.Route, error) {
	matchers := []*matchersv1.Matcher{defaults.DefaultMatcher()}
	if len(gatewayRoute.Matchers) > 0 {
		matchers = gatewayRoute.Matchers
	}

	glooRoute := &gloov1.Route{
		Matchers: matchers,
		Options:  gatewayRoute.Options,
	}

	switch action := gatewayRoute.Action.(type) {
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
		return rv.convertDelegateAction(gatewayRoute)
		//default:
		//	return nil, NoActionErr
	}

	return []*gloov1.Route{glooRoute}, nil
}

func (rv *routeVisitor) convertDelegateAction(route *gatewayv1.Route) ([]*gloov1.Route, error) {
	delegate := route.GetDelegateAction()
	if delegate == nil {
		return nil, NoDelegateActionErr
	}

	// Retrieve and validate the matcher prefix
	delegatePrefix, err := getDelegateRoutePrefix(route)
	if err != nil {
		return nil, err
	}

	// Determine the route tables to delegate to
	routeTables := rv.selectRouteTables(delegate)
	if len(routeTables) == 0 {
		return nil, nil
	}

	// Check for delegation cycles
	if err := rv.checkForCycles(routeTables); err != nil {
		return nil, err
	}

	var delegatedRoutes []*gloov1.Route
	for _, routeTable := range routeTables {
		for _, routeTableRoute := range routeTable.Routes {

			// Clone route since we mutate
			routeClone := proto.Clone(routeTableRoute).(*gatewayv1.Route)

			// Merge plugins from parent route
			merged, err := mergeRoutePlugins(routeClone.GetOptions(), route.GetOptions())
			if err != nil {
				// Should never happen
				return nil, errors.Wrapf(err, "internal error: merging route plugins from parent to delegated route")
			}
			routeClone.Options = merged

			// Check if the path prefix is compatible with the one on the parent route
			if err := isRouteTableValidForDelegatePrefix(delegatePrefix, routeClone); err != nil {
				rv.addError(err)
				continue
			}

			// Spawn a new visitor to visit this route table. This recursively calls `ConvertRoute`.
			subRoutes, err := rv.createSubVisitor(routeTable).ConvertRoute(routeClone)
			if err != nil {
				return nil, err
			}
			for _, sub := range subRoutes {
				if err := appendSource(sub, routeTable); err != nil {
					// should never happen
					return nil, err
				}
				delegatedRoutes = append(delegatedRoutes, sub)
			}
		}
	}

	glooutils.SortRoutesByPath(delegatedRoutes)

	return delegatedRoutes, nil
}

func (rv *routeVisitor) selectRouteTables(delegateAction *gatewayv1.DelegateAction) gatewayv1.RouteTableList {
	var routeTables gatewayv1.RouteTableList

	if routeTableRef := getRouteTableRef(delegateAction); routeTableRef != nil {
		// missing refs should only result in a warning
		// this allows resources to be applied asynchronously
		routeTable, err := rv.tables.Find((*routeTableRef).Strings())
		if err != nil {
			rv.addWarning(RouteTableMissingWarning(*routeTableRef))
			return nil
		}
		routeTables = gatewayv1.RouteTableList{routeTable}

	} else if rtSelector := delegateAction.GetSelector(); rtSelector != nil {
		routeTables = routeTablesForSelector(rv.tables, rtSelector, rv.rootResource.GetMetadata().Namespace)

		if len(routeTables) == 0 {
			rv.addWarning(NoMatchingRouteTablesWarning)
			return nil
		}
	} else {
		rv.addWarning(MissingRefAndSelectorWarning(rv.rootResource))
		return nil
	}
	return routeTables
}

// Create a new visitor to visit the current route table
func (rv *routeVisitor) createSubVisitor(routeTable *gatewayv1.RouteTable) *routeVisitor {
	visitor := &routeVisitor{
		rootResource: routeTable,
		tables:       rv.tables,
		reports:      rv.reports,
	}

	// Add all route tables from the parent visitor
	for _, vis := range rv.visited {
		visitor.visited = append(visitor.visited, vis)
	}

	// Add the route table that is the root for the new visitor
	visitor.visited = append(visitor.visited, routeTable)

	return visitor
}

// If any of the matching route tables has already been visited, that means we have a delegation cycle.
func (rv *routeVisitor) checkForCycles(routeTables gatewayv1.RouteTableList) error {
	for _, visited := range rv.visited {
		for _, toVisit := range routeTables {
			if toVisit == visited {
				return DelegationCycleErr(
					buildCycleInfoString(append(append(gatewayv1.RouteTableList{}, rv.visited...), toVisit)),
				)
			}
		}
	}
	return nil
}

func (rv *routeVisitor) addWarning(message string) {
	rv.reports.AddWarning(rv.rootResource, message)
}

func (rv *routeVisitor) addError(err error) {
	rv.reports.AddError(rv.rootResource, err)
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

func routeTablesForSelector(routeTables gatewayv1.RouteTableList, selector *gatewayv1.RouteTableSelector, ownerNamespace string) gatewayv1.RouteTableList {
	type nsSelectorType int
	const (
		// Match route tables in the owner namespace
		owner nsSelectorType = iota
		// Match route tables in all namespaces watched by Gloo
		all
		// Match route tables in the specified namespaces
		list
	)

	nsSelector := owner
	if len(selector.Namespaces) > 0 {
		nsSelector = list
	}
	for _, ns := range selector.Namespaces {
		if ns == allNamespaceRouteTableSelector {
			nsSelector = all
		}
	}

	var labelSelector labels.Selector
	if len(selector.Labels) > 0 {
		labelSelector = labels.SelectorFromSet(selector.Labels)
	}

	var matchingRouteTables gatewayv1.RouteTableList
	for _, candidate := range routeTables {

		// Check whether labels match
		if labelSelector != nil {
			rtLabels := labels.Set(candidate.Metadata.Labels)
			if !labelSelector.Matches(rtLabels) {
				continue
			}
		}

		// Check whether namespace matches
		nsMatches := false
		switch nsSelector {
		case all:
			nsMatches = true
		case owner:
			nsMatches = candidate.Metadata.Namespace == ownerNamespace
		case list:
			for _, ns := range selector.Namespaces {
				if ns == candidate.Metadata.Namespace {
					nsMatches = true
				}
			}
		}

		if nsMatches {
			matchingRouteTables = append(matchingRouteTables, candidate)
		}
	}

	return matchingRouteTables
}

func buildCycleInfoString(routeTables gatewayv1.RouteTableList) string {
	var visitedTables []string
	for _, rt := range routeTables {
		visitedTables = append(visitedTables, fmt.Sprintf("[%s]", rt.Metadata.Ref().Key()))
	}
	return strings.Join(visitedTables, " -> ")
}

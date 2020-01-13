package translator

import (
	"fmt"
	"strings"

	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	matchersv1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	glooutils "github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/go-utils/errors"
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
	// Converts a gateway route to one or more gloo routes.
	// Can return multiple routes only if the input route uses delegation.
	ConvertRoute(gatewayRoute *v1.Route) ([]*gloov1.Route, error)
}

// Implements the RouteConverter interface by recursively visiting a route tree
type routeVisitor struct {
	rootResource resources.InputResource
	tables       v1.RouteTableList
	visited      v1.RouteTableList
	reports      reporter.ResourceReports
}

func NewRouteVisitor(root resources.InputResource, tables v1.RouteTableList, reports reporter.ResourceReports) *routeVisitor {
	return &routeVisitor{
		rootResource: root,
		tables:       tables,
		reports:      reports,
	}
}

func (rv *routeVisitor) ConvertRoute(gatewayRoute *v1.Route) ([]*gloov1.Route, error) {
	matchers := []*matchersv1.Matcher{defaults.DefaultMatcher()}
	if len(gatewayRoute.Matchers) > 0 {
		matchers = gatewayRoute.Matchers
	}

	glooRoute := &gloov1.Route{
		Matchers: matchers,
		Options:  gatewayRoute.Options,
	}

	switch action := gatewayRoute.Action.(type) {
	case *v1.Route_RedirectAction:
		glooRoute.Action = &gloov1.Route_RedirectAction{
			RedirectAction: action.RedirectAction,
		}
	case *v1.Route_DirectResponseAction:
		glooRoute.Action = &gloov1.Route_DirectResponseAction{
			DirectResponseAction: action.DirectResponseAction,
		}
	case *v1.Route_RouteAction:
		glooRoute.Action = &gloov1.Route_RouteAction{
			RouteAction: action.RouteAction,
		}
	case *v1.Route_DelegateAction:
		return rv.convertDelegateAction(gatewayRoute)
	}

	return []*gloov1.Route{glooRoute}, nil
}

func (rv *routeVisitor) convertDelegateAction(route *v1.Route) ([]*gloov1.Route, error) {
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
			routeTableRoute := proto.Clone(routeTableRoute).(*v1.Route)

			// Merge plugins from parent route
			merged, err := mergeRoutePlugins(routeTableRoute.GetOptions(), route.GetOptions())
			if err != nil {
				// Should never happen
				return nil, errors.Wrapf(err, "internal error: merging route plugins from parent to delegated route")
			}
			routeTableRoute.Options = merged

			// Check if the path prefix is
			if err := isRouteTableValidForDelegatePrefix(delegatePrefix, routeTableRoute); err != nil {
				rv.addError(err)
				continue
			}

			// Spawn a new visitor to visit this route table. This recursively calls `ConvertRoute`.
			subRoutes, err := rv.spawn(routeTable).ConvertRoute(routeTableRoute)
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

func (rv *routeVisitor) selectRouteTables(delegateAction *v1.DelegateAction) v1.RouteTableList {
	var routeTables v1.RouteTableList

	if routeTableRef := getRouteTableRef(delegateAction); routeTableRef != nil {
		// missing refs should only result in a warning
		// this allows resources to be applied asynchronously
		routeTable, err := rv.tables.Find((*routeTableRef).Strings())
		if err != nil {
			rv.addWarning(RouteTableMissingWarning(*routeTableRef))
			return nil
		}
		routeTables = v1.RouteTableList{routeTable}

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
func (rv *routeVisitor) spawn(routeTable *v1.RouteTable) RouteConverter {
	visitor := NewRouteVisitor(routeTable, rv.tables, rv.reports)

	// Add all route tables from the parent visitor
	for _, vis := range rv.visited {
		visitor.visited = append(visitor.visited, vis)
	}

	// Add the route table that is the root for the new visitor
	visitor.visited = append(visitor.visited, routeTable)

	return visitor
}

// If any of the matching route tables has already been visited, that means we have a delegation cycle.
func (rv *routeVisitor) checkForCycles(routeTables v1.RouteTableList) error {
	for _, visited := range rv.visited {
		for _, toVisit := range routeTables {
			if toVisit == visited {
				return DelegationCycleErr(
					buildCycleInfoString(append(append(v1.RouteTableList{}, rv.visited...), toVisit)),
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

func getDelegateRoutePrefix(route *v1.Route) (string, error) {
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

func isRouteTableValidForDelegatePrefix(delegatePrefix string, routeTable *v1.Route) error {
	for _, match := range routeTable.Matchers {
		// ensure all sub-routes in the delegated route table match the parent prefix
		if pathString := glooutils.PathAsString(match); !strings.HasPrefix(pathString, delegatePrefix) {
			return InvalidRouteTableForDelegateErr(delegatePrefix, pathString)
		}
	}
	return nil
}

// Handles new and deprecated format for referencing a route table
// TODO: remove this function when we remove the deprecated fields from the API
func getRouteTableRef(delegate *v1.DelegateAction) *core.ResourceRef {
	if delegate.Namespace != "" || delegate.Name != "" {
		return &core.ResourceRef{
			Namespace: delegate.Namespace,
			Name:      delegate.Name,
		}
	}
	return delegate.GetRef()
}

func routeTablesForSelector(routeTables v1.RouteTableList, selector *v1.RouteTableSelector, ownerNamespace string) v1.RouteTableList {
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

	var matchingRouteTables v1.RouteTableList
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

func buildCycleInfoString(routeTables v1.RouteTableList) string {
	var visitedTables []string
	for _, rt := range routeTables {
		visitedTables = append(visitedTables, fmt.Sprintf("[%s]", rt.Metadata.Ref().Key()))
	}
	return strings.Join(visitedTables, " -> ")
}

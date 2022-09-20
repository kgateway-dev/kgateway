package translator

import (
	"fmt"
	"strings"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/transformation"

	"github.com/golang/protobuf/ptypes/wrappers"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/golang/protobuf/proto"
	errors "github.com/rotisserie/eris"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	matchersv1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	gloov1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	glooutils "github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
)

const (
	defaultTableWeight = 0
	// separator for generated route names
	sep = "_"
)

var (
	NoActionErr          = errors.New("invalid route: route must specify an action")
	MatcherCountErr      = errors.New("invalid route: routes with delegate actions must omit or specify a single matcher")
	MissingPrefixErr     = errors.New("invalid route: routes with delegate actions must use a prefix matcher")
	InvalidPrefixErr     = errors.New("invalid route: route table matchers must begin with the prefix of their parent route's matcher")
	InvalidPathMatchErr  = errors.New("invalid route: route table matchers must have the same case sensitivity of their parent route's matcher")
	InvalidHeaderErr     = errors.New("invalid route: route table matchers must have all headers that were specified on their parent route's matcher")
	InvalidQueryParamErr = errors.New("invalid route: route table matchers must have all query params that were specified on their parent route's matcher")
	InvalidMethodErr     = errors.New("invalid route: route table matchers must have all methods that were specified on their parent route's matcher")

	UnnamedRoute = func(index int) string {
		return fmt.Sprintf("<unnamed-%d>", index)
	}
	DelegationCycleErr = func(cycleInfo string) error {
		return errors.Errorf("invalid route: delegation cycle detected: %s", cycleInfo)
	}
	InvalidRouteTableForDelegatePrefixWarning = func(delegatePrefix, prefixString string) error {
		return errors.Wrapf(InvalidPrefixErr, "required prefix: %v, prefix: %v", delegatePrefix, prefixString)
	}
	InvalidRouteTableForDelegateHeadersWarning = func(delegateHeaders, childHeaders []*matchersv1.HeaderMatcher) error {
		return errors.Wrapf(InvalidHeaderErr, "required headers: %v, headers: %v", delegateHeaders, childHeaders)
	}
	InvalidRouteTableForDelegateQueryParamsWarning = func(delegateQueryParams, childQueryParams []*matchersv1.QueryParameterMatcher) error {
		return errors.Wrapf(InvalidQueryParamErr, "required query params: %v, query params: %v", delegateQueryParams, childQueryParams)
	}
	InvalidRouteTableForDelegateMethodsWarning = func(delegateMethods, childMethods []string) error {
		return errors.Wrapf(InvalidMethodErr, "required methods: %v, methods: %v", delegateMethods, childMethods)
	}
	TopLevelVirtualResourceErr = func(rtRef *core.Metadata, err error) error {
		return errors.Wrapf(err, "on sub route table %s", rtRef.Ref().Key())
	}

	InvalidRouteTableForDelegateCaseSensitivePathMatchWarning = func(delegateMatchCaseSensitive, matchCaseSensitive *wrappers.BoolValue) error {
		return errors.Wrapf(InvalidPathMatchErr, "required caseSensitive: %v, caseSensitive: %v", delegateMatchCaseSensitive, matchCaseSensitive)
	}
)

type RouteConverter interface {
	// Converts a VirtualService to a set of Gloo API routes (i.e. routes on a Proxy resource).
	// Since virtual services and route tables are often owned by different teams, it breaks multitenancy if
	// this function cannot return successfully; thus ALL ERRORS are added to the resource reports
	ConvertVirtualService(virtualService *gatewayv1.VirtualService, gateway *gatewayv1.Gateway, proxyName string, snapshot *gloov1snap.ApiSnapshot, reports reporter.ResourceReports) []*gloov1.Route
}

func NewRouteConverter(selector RouteTableSelector, indexer RouteTableIndexer) RouteConverter {
	return &routeVisitor{
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
	// Used to select route tables for delegated routes.
	routeTableSelector RouteTableSelector
	// Used to sort route tables when multiple ones are matched by a selector.
	routeTableIndexer RouteTableIndexer
}

// Helper object used to store information about previously visited routes.
type routeInfo struct {
	// The matcher for the route
	matcher *matchersv1.Matcher
	// The options on the route.
	options *gloov1.RouteOptions
	// Used to build the name of the route as we traverse the tree.
	name string
	// Is true if any route on the current route tree branch is explicitly named by the user.
	hasName bool
	// Whether any child route objects should inherit headers, methods, and query param matchers from the parent.
	inheritableMatchers bool
	// Whether any child route objects should inherit path matchers from the parent.
	inheritablePathMatchers bool
}

// Helper object for reporting errors and warnings
type reporterHelper struct {
	reports                reporter.ResourceReports
	topLevelVirtualService *gatewayv1.VirtualService
	snapshot               *gloov1snap.ApiSnapshot
}

func (r *reporterHelper) addError(resource resources.InputResource, err error) {
	r.reports.AddError(resource, err)

	// If the resource is a Route Table, also add the error to the top level virtual service.
	if rt, ok := resource.(*gatewayv1.RouteTable); ok {
		r.reports.AddError(r.topLevelVirtualService, TopLevelVirtualResourceErr(rt.GetMetadata(), err))
	}
}

func (r *reporterHelper) addWarning(resource resources.InputResource, err error) {
	r.reports.AddWarning(resource, err.Error())

	// If the resource is a Route Table, also add the warning to the top level virtual service.
	if rt, ok := resource.(*gatewayv1.RouteTable); ok {
		r.reports.AddWarning(r.topLevelVirtualService, TopLevelVirtualResourceErr(rt.GetMetadata(), err).Error())
	}
}

// Since virtual services and route tables are often owned by different teams, it breaks multitenancy if
// this function cannot return successfully; thus ALL ERRORS are added to the resource reports
func (rv *routeVisitor) ConvertVirtualService(virtualService *gatewayv1.VirtualService, gateway *gatewayv1.Gateway, proxyName string, snapshot *gloov1snap.ApiSnapshot, reports reporter.ResourceReports) []*gloov1.Route {
	wrapper := &visitableVirtualService{VirtualService: virtualService}
	return rv.visit(
		wrapper,
		gateway,
		proxyName,
		nil,
		nil,
		&reporterHelper{
			reports:                reports,
			topLevelVirtualService: virtualService,
			snapshot:               snapshot,
		},
	)
}

// Performs a depth-first, in-order traversal of a route tree rooted at the given resource.
// The additional arguments are used to store the state of the traversal of the current branch of the route tree.
func (rv *routeVisitor) visit(
	resource resourceWithRoutes,
	gateway *gatewayv1.Gateway,
	proxyName string,
	parentRoute *routeInfo,
	visitedRouteTables gatewayv1.RouteTableList,
	reporterHelper *reporterHelper,
) []*gloov1.Route {
	var routes []*gloov1.Route

	for idx, gatewayRoute := range resource.GetRoutes() {

		// Clone route to be safe, since we might mutate it
		routeClone := proto.Clone(gatewayRoute).(*gatewayv1.Route)

		// Determine route name
		name, routeHasName := routeName(resource.InputResource(), gateway, proxyName, routeClone, parentRoute, idx)
		routeClone.Name = name

		// Merge delegated options into route options
		// Route options specified on the Route override delegated options
		optionRefs := routeClone.GetOptionsConfigRefs().GetDelegateOptions()
		for _, optionRef := range optionRefs {
			routeOpts, err := reporterHelper.snapshot.RouteOptions.Find(optionRef.GetNamespace(), optionRef.GetName())
			if err != nil {
				// missing refs should only result in a warning
				// this allows resources to be applied asynchronously if the validation webhook is configured to allow warnings
				reporterHelper.addWarning(resource.InputResource(), err)
				continue
			}
			if routeClone.GetOptions() == nil {
				routeClone.Options = routeOpts.GetOptions()
				continue
			}
			routeClone.Options = mergeRouteOptions(routeClone.GetOptions(), routeOpts.GetOptions())
		}

		// If the parent route is not nil, this route has been delegated to and we need to perform additional operations
		if parentRoute != nil {
			var err error
			routeClone, err = validateAndMergeParentRoute(routeClone, parentRoute)
			if err != nil {
				// An error occurs here when a delegated route's matcher is incompatible with its parent route's
				// matcher. If we were to add this as an error on the resource, it would cause the entire VirtualHost
				// to get stripped from the Proxy during gateway translation sync, thereby preventing the Proxy from
				// receiving updates to other valid routes within the same VirtualHost.
				// Therefore, we treat these as warnings here, allowing other valid routes on the same VirtualHost to
				// still pass through and get onto the Proxy.
				reporterHelper.addWarning(resource.InputResource(), err)
				continue
			}
		} else {
			// if parent route is nil, parent is virtual host
			if routeClone.GetOptions().GetStagedTransformations().GetInheritTransformation() {
				parentVirtualHost := reporterHelper.topLevelVirtualService.GetVirtualHost()
				// merge StagedTransformations config from virtualHost to child route
				inheritStagedTransformations(routeClone, parentVirtualHost.GetOptions().GetStagedTransformations())
			}
		}

		switch action := routeClone.GetAction().(type) {
		case *gatewayv1.Route_DelegateAction:

			// Validate the matcher of the delegate route
			delegateMatcher, err := getDelegateRouteMatcher(routeClone)
			if err != nil {
				reporterHelper.addError(resource.InputResource(), err)
				continue
			}

			// Determine the route tables to delegate to
			routeTables, err := rv.routeTableSelector.SelectRouteTables(action.DelegateAction, resource.InputResource().GetMetadata().GetNamespace())
			if err != nil {
				reporterHelper.addWarning(resource.InputResource(), err)
				continue
			}

			// Default missing weights to 0
			for _, routeTable := range routeTables {
				if routeTable.GetWeight() == nil {
					routeTable.Weight = &wrappers.Int32Value{Value: defaultTableWeight}
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
						// Note that we do not report the error on the table we are currently visiting, but on the
						// one we are about to visit, since that is the one that started the cycle.
						reporterHelper.addError(routeTable, err)
						continue
					}

					// Collect information about this route that are relevant when visiting the delegated route table
					currentRouteInfo := &routeInfo{
						matcher:                 delegateMatcher,
						options:                 routeClone.GetOptions(),
						name:                    name,
						hasName:                 routeHasName,
						inheritableMatchers:     routeClone.GetInheritableMatchers().GetValue(),
						inheritablePathMatchers: routeClone.GetInheritablePathMatchers().GetValue(),
					}

					// Make a copy of the existing set of visited route tables. We need to pass this information into
					// the recursive call and we do NOT want the original slice to be modified.
					visitedRtCopy := append(append([]*gatewayv1.RouteTable{}, visitedRouteTables...), routeTable)

					// Recursive call
					subRoutes := rv.visit(
						&visitableRouteTable{routeTable},
						gateway,
						proxyName,
						currentRouteInfo,
						visitedRtCopy,
						reporterHelper,
					)
					if err != nil {
						return nil
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
			if action, ok := routeClone.GetAction().(*gatewayv1.Route_RouteAction); ok {
				parentNamespace := resource.InputResource().GetMetadata().GetNamespace()
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
				reporterHelper.addError(resource.InputResource(), err)
				continue
			}
			routes = append(routes, glooRoute)
		}
	}

	// Append source metadata to all the routes
	for _, r := range routes {
		err := appendSource(r, resource.InputResource())
		if err != nil {
			// should never happen
			reporterHelper.addError(resource.InputResource(), err)
		}
	}

	return routes
}

// Returns the name of the route and a flag that is true if either the route or the parent route are explicitly named.
// Route names have the following format: "vs:mygateway_myproxy_myvirtualservice_route:myfirstroute_rt:myroutetable_route:<unnamed-0>"
func routeName(resource resources.InputResource, gateway *gatewayv1.Gateway, proxyName string, route *gatewayv1.Route, parentRouteInfo *routeInfo, index int) (string, bool) {
	nameBuilder := strings.Builder{}
	if parentRouteInfo != nil {
		nameBuilder.WriteString(parentRouteInfo.name)
		nameBuilder.WriteString(sep)
	}

	switch resource.(type) {
	case *gatewayv1.VirtualService:
		nameBuilder.WriteString("vs:")

		// for virtual services, add gateway and proxy name to ensure name uniqueness
		nameBuilder.WriteString(gateway.GetMetadata().GetName())
		nameBuilder.WriteString(sep)
		nameBuilder.WriteString(proxyName)
		nameBuilder.WriteString(sep)
	case *gatewayv1.RouteTable:
		nameBuilder.WriteString("rt:")
	default:
		// Should never happen
	}

	nameBuilder.WriteString(resource.GetMetadata().GetNamespace())
	nameBuilder.WriteString(sep)
	nameBuilder.WriteString(resource.GetMetadata().GetName())
	nameBuilder.WriteString("_route:")

	var isRouteNamed bool
	routeDisplayName := route.GetName()
	if routeDisplayName == "" {
		routeDisplayName = UnnamedRoute(index)
	} else {
		isRouteNamed = true
	}
	nameBuilder.WriteString(routeDisplayName)

	// If the current route has no name, but the parent one does, then we consider the resulting route to be named.
	isRouteNamed = isRouteNamed || (parentRouteInfo != nil && parentRouteInfo.hasName)

	return nameBuilder.String(), isRouteNamed
}

func convertSimpleAction(simpleRoute *gatewayv1.Route) (*gloov1.Route, error) {
	matchers := []*matchersv1.Matcher{defaults.DefaultMatcher()}
	if len(simpleRoute.GetMatchers()) > 0 {
		matchers = simpleRoute.GetMatchers()
	}

	glooRoute := &gloov1.Route{
		Matchers: matchers,
		Options:  simpleRoute.GetOptions(),
		Name:     simpleRoute.GetName(),
	}

	switch action := simpleRoute.GetAction().(type) {
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
	case *gatewayv1.Route_GraphqlApiRef:
		glooRoute.Action = &gloov1.Route_GraphqlApiRef{
			GraphqlApiRef: action.GraphqlApiRef,
		}
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

func getDelegateRouteMatcher(route *gatewayv1.Route) (*matchersv1.Matcher, error) {
	switch len(route.GetMatchers()) {
	case 0:
		return defaults.DefaultMatcher(), nil
	case 1:
		matcher := route.GetMatchers()[0]
		if matcher.GetPathSpecifier() == nil {
			return defaults.DefaultMatcher(), nil // no path specifier provided, default to '/' prefix matcher
		}
		if matcher.GetPrefix() == "" {
			return nil, MissingPrefixErr
		}
		return matcher, nil
	default:
		return nil, MatcherCountErr
	}
}

func validateAndMergeParentRoute(child *gatewayv1.Route, parent *routeInfo) (*gatewayv1.Route, error) {

	// inherit inheritance config from parent if unset
	if child.GetInheritablePathMatchers() == nil {
		child.InheritablePathMatchers = &wrappers.BoolValue{
			Value: parent.inheritablePathMatchers,
		}
	}

	// inherit inheritance config from parent if unset
	if child.GetInheritableMatchers() == nil {
		child.InheritableMatchers = &wrappers.BoolValue{
			Value: parent.inheritableMatchers,
		}
	}

	// inherit route table config from parent
	if child.GetInheritablePathMatchers().GetValue() {
		for _, childMatch := range child.GetMatchers() {
			childMatch.PathSpecifier = parent.matcher.GetPathSpecifier()
			childMatch.CaseSensitive = parent.matcher.GetCaseSensitive()
		}
		if len(child.GetMatchers()) == 0 {
			child.Matchers = []*matchersv1.Matcher{{
				PathSpecifier: parent.matcher.GetPathSpecifier(),
				CaseSensitive: parent.matcher.GetCaseSensitive(),
			}}
		}
	}

	// If the route has no matchers, we fall back to the default prefix matcher like for regular routes.
	if len(child.GetMatchers()) == 0 {
		child.Matchers = []*matchersv1.Matcher{defaults.DefaultMatcher()}
	}

	// inherit route table config from parent
	if child.GetInheritableMatchers().GetValue() {
		for _, childMatch := range child.GetMatchers() {
			childMatch.Headers = append(parent.matcher.GetHeaders(), childMatch.GetHeaders()...)
			childMatch.Methods = append(parent.matcher.GetMethods(), childMatch.GetMethods()...)
			childMatch.QueryParameters = append(parent.matcher.GetQueryParameters(), childMatch.GetQueryParameters()...)
		}
	}
	// If child has inheritTransformations specified, append transformations from parent to child route
	if child.GetOptions().GetStagedTransformations().GetInheritTransformation() {
		inheritStagedTransformations(child, parent.options.GetStagedTransformations())
	}

	// Verify that the matchers are compatible with the parent prefix
	if err := isRouteTableValidForDelegateMatcher(parent.matcher, child); err != nil {
		return nil, err
	}

	// Merge options from parent routes
	// If an option is defined on a parent route, it will override the child route's option
	child.Options = mergeRouteOptions(child.GetOptions(), parent.options)

	return child, nil
}

func isRouteTableValidForDelegateMatcher(parentMatcher *matchersv1.Matcher, childRoute *gatewayv1.Route) error {

	for _, childMatch := range childRoute.GetMatchers() {
		// ensure all sub-routes in the delegated route table match the parent prefix
		if pathString := glooutils.PathAsString(childMatch); !strings.HasPrefix(pathString, parentMatcher.GetPrefix()) {
			return InvalidRouteTableForDelegatePrefixWarning(parentMatcher.GetPrefix(), pathString)
		}

		// ensure all sub-routes matches in the delegated route match the parent case sensitivity
		if !proto.Equal(childMatch.GetCaseSensitive(), parentMatcher.GetCaseSensitive()) {
			return InvalidRouteTableForDelegateCaseSensitivePathMatchWarning(childMatch.GetCaseSensitive(), parentMatcher.GetCaseSensitive())
		}

		// ensure all headers in the delegated route table are a superset of those from the parent route resource
		childHeaderNameToHeader := map[string]*matchersv1.HeaderMatcher{}
		for _, childHeader := range childMatch.GetHeaders() {
			childHeaderNameToHeader[childHeader.GetName()] = childHeader
		}
		for _, parentHeader := range parentMatcher.GetHeaders() {
			if childHeader, ok := childHeaderNameToHeader[parentHeader.GetName()]; !ok {
				return InvalidRouteTableForDelegateHeadersWarning(parentMatcher.GetHeaders(), childMatch.GetHeaders())
			} else if !parentHeader.Equal(childHeader) {
				return InvalidRouteTableForDelegateHeadersWarning(parentMatcher.GetHeaders(), childMatch.GetHeaders())
			}
		}

		// ensure all query parameters in the delegated route table are a superset of those from the parent route resource
		childQueryParamNameToHeader := map[string]*matchersv1.QueryParameterMatcher{}
		for _, childQueryParam := range childMatch.GetQueryParameters() {
			childQueryParamNameToHeader[childQueryParam.GetName()] = childQueryParam
		}
		for _, parentQueryParameter := range parentMatcher.GetQueryParameters() {
			if childQueryParam, ok := childQueryParamNameToHeader[parentQueryParameter.GetName()]; !ok {
				return InvalidRouteTableForDelegateQueryParamsWarning(parentMatcher.GetQueryParameters(), childMatch.GetQueryParameters())
			} else if !parentQueryParameter.Equal(childQueryParam) {
				return InvalidRouteTableForDelegateQueryParamsWarning(parentMatcher.GetQueryParameters(), childMatch.GetQueryParameters())
			}
		}

		// ensure all HTTP methods in the delegated route table are a superset of those from the parent route resource
		childMethodsSet := sets.NewString(childMatch.GetMethods()...)
		if !childMethodsSet.HasAll(parentMatcher.GetMethods()...) {
			return InvalidRouteTableForDelegateMethodsWarning(parentMatcher.GetMethods(), childMatch.GetMethods())
		}
	}
	return nil
}

// Handles new and deprecated format for referencing a route table
// TODO: remove this function when we remove the deprecated fields from the API
func getRouteTableRef(delegate *gatewayv1.DelegateAction) *core.ResourceRef {
	if delegate.GetNamespace() != "" || delegate.GetName() != "" {
		return &core.ResourceRef{
			Namespace: delegate.GetNamespace(),
			Name:      delegate.GetName(),
		}
	}
	return delegate.GetRef()
}

func buildCycleInfoString(routeTables gatewayv1.RouteTableList) string {
	var visitedTables []string
	for _, rt := range routeTables {
		visitedTables = append(visitedTables, fmt.Sprintf("[%s]", rt.GetMetadata().Ref().Key()))
	}
	return strings.Join(visitedTables, " -> ")
}

func inheritStagedTransformations(child *gatewayv1.Route, parentTransformationStages *transformation.TransformationStages) {
	childTransformationStages := child.GetOptions().GetStagedTransformations()
	// inherit transformation config from parent
	mergeTransformations(&childTransformationStages.Regular,
		parentTransformationStages.GetRegular())
	mergeTransformations(&childTransformationStages.Early,
		parentTransformationStages.GetEarly())
}

func mergeTransformations(childTransformationsPtr **transformation.RequestResponseTransformations, parentTransformations *transformation.RequestResponseTransformations) {
	if parentTransformations == nil {
		// no transformations from parent to merge in
		return
	}
	childTransformations := *childTransformationsPtr
	if childTransformations == nil {
		// if child has no transformation config, merge in parent config
		*childTransformationsPtr = parentTransformations
		return
	}
	// Append transformations from parent after child transformation
	// This means that on conflicting transformations, the transformation from the child will be applied
	childTransformations.ResponseTransforms = append(childTransformations.GetResponseTransforms(),
		parentTransformations.GetResponseTransforms()...)
	childTransformations.RequestTransforms = append(childTransformations.GetRequestTransforms(),
		parentTransformations.GetRequestTransforms()...)
}

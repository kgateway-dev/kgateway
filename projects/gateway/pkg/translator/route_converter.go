package translator

//type RouteConverter interface {
//	// Converts a Gateway API route (i.e. a route on RouteTables/VirtualServices)
//	// to one or more Gloo API routes (i.e. routes on a Proxy resource).
//	// Can return multiple routes only if the input route uses delegation.
//	ConvertRoute(route *gatewayv1.Route) ([]*gloov1.Route, error)
//}
//
//// Implements the RouteConverter interface by recursively visiting a route tree
//type routeVisitor struct {
//	// This is the root of the subtree of routes that we are going to visit. It can be either a virtual service or a
//	// route table. Errors and warnings for the current visitor will be reported on this resource.
//	rootResource resources.InputResource
//	// All the route tables in the current snapshot.
//	tables gatewayv1.RouteTableList
//	// Used to keep track of route tables that have already been visited in order to avoid cycles.
//	visited gatewayv1.RouteTableList
//	// Used to store of errors and warnings for the root resource. This object will be passed to sub-visitors.
//	reports reporter.ResourceReports
//	// Used to keep track of the long name of a route as we traverse the tree toward it, including vs, route, and route table ancestors.
//	// Ex name: "vs:myvirtualservice_route:myfirstroute_rt:myroutetable_route:<unnamed>"
//	nameTree string
//	// used to keep track of whether there is a named route anywhere in the tree for naming purposes
//	containsNamedRoute bool
//}
//
//// Initializes and returns a route converter instance.
//// - root: root of the subtree of routes that we are going to visit; used primarily as a target to report errors and warnings on.
//// - tables: all the route tables that should be considered when resolving delegation chains.
//// - reports: this object will be updated with errors and warnings encountered during the conversion process.
//func NewRouteConverter(root *gatewayv1.VirtualService, tables gatewayv1.RouteTableList, reports reporter.ResourceReports) RouteConverter {
//
//	return &routeVisitor{
//		rootResource: root,
//		tables:       tables,
//		reports:      reports,
//		nameTree:     "vs:" + root.Metadata.Name,
//	}
//}
//
//func (rv *routeVisitor) ConvertRoute(gatewayRoute *gatewayv1.Route) ([]*gloov1.Route, error) {
//	matchers := []*matchersv1.Matcher{defaults.DefaultMatcher()}
//	if len(gatewayRoute.Matchers) > 0 {
//		matchers = gatewayRoute.Matchers
//	}
//
//	routeDisplayName := gatewayRoute.Name
//	if routeDisplayName == "" {
//		routeDisplayName = "<unnamed>"
//	} else {
//		rv.containsNamedRoute = true
//	}
//	rv.nameTree += "_route:" + routeDisplayName
//	glooRoute := &gloov1.Route{
//		Matchers: matchers,
//		Options:  gatewayRoute.Options,
//		Name:     rv.nameTree,
//	}
//
//	// if this is a leaf and there are no named routes in the tree, wipe the name
//	if gatewayRoute.GetDelegateAction() == nil && !rv.containsNamedRoute {
//		glooRoute.Name = ""
//	}
//
//	switch action := gatewayRoute.Action.(type) {
//	case *gatewayv1.Route_RedirectAction:
//		glooRoute.Action = &gloov1.Route_RedirectAction{
//			RedirectAction: action.RedirectAction,
//		}
//	case *gatewayv1.Route_DirectResponseAction:
//		glooRoute.Action = &gloov1.Route_DirectResponseAction{
//			DirectResponseAction: action.DirectResponseAction,
//		}
//	case *gatewayv1.Route_RouteAction:
//		glooRoute.Action = &gloov1.Route_RouteAction{
//			RouteAction: action.RouteAction,
//		}
//	case *gatewayv1.Route_DelegateAction:
//		return rv.convertDelegateAction(gatewayRoute)
//	default:
//		return nil, NoActionErr
//	}
//
//	return []*gloov1.Route{glooRoute}, nil
//}
//
//func (rv *routeVisitor) convertDelegateAction(route *gatewayv1.Route) ([]*gloov1.Route, error) {
//	delegate := route.GetDelegateAction()
//	if delegate == nil {
//		return nil, NoDelegateActionErr
//	}
//
//	// Retrieve and validate the matcher prefix
//	delegatePrefix, err := getDelegateRoutePrefix(route)
//	if err != nil {
//		return nil, err
//	}
//
//	// Determine the route tables to delegate to
//	routeTables := rv.selectRouteTables(delegate)
//	if len(routeTables) == 0 {
//		return nil, nil
//	}
//
//	// Check for delegation cycles
//	if err := rv.checkForCycles(routeTables); err != nil {
//		return nil, err
//	}
//
//	var delegatedRoutes []*gloov1.Route
//	for _, routeTable := range routeTables {
//		for _, routeTableRoute := range routeTable.Routes {
//
//			// Clone route since we mutate
//			routeClone := proto.Clone(routeTableRoute).(*gatewayv1.Route)
//
//			// Merge plugins from parent route
//			merged, err := mergeRoutePlugins(routeClone.GetOptions(), route.GetOptions())
//			if err != nil {
//				// Should never happen
//				return nil, errors.Wrapf(err, "internal error: merging route plugins from parent to delegated route")
//			}
//			routeClone.Options = merged
//
//			// Check if the path prefix is compatible with the one on the parent route
//			if err := isRouteTableValidForDelegatePrefix(delegatePrefix, routeClone); err != nil {
//				rv.addError(err)
//				continue
//			}
//
//			// Spawn a new visitor to visit this route table. This recursively calls `ConvertRoute`.
//			subRoutes, err := rv.createSubVisitor(routeTable).ConvertRoute(routeClone)
//			if err != nil {
//				return nil, err
//			}
//			for _, sub := range subRoutes {
//				if err := appendSource(sub, routeTable); err != nil {
//					// should never happen
//					return nil, err
//				}
//				delegatedRoutes = append(delegatedRoutes, sub)
//			}
//		}
//	}
//
//	glooutils.SortRoutesByPath(delegatedRoutes)
//
//	return delegatedRoutes, nil
//}
//
//func (rv *routeVisitor) selectRouteTables(delegateAction *gatewayv1.DelegateAction) gatewayv1.RouteTableList {
//	var routeTables gatewayv1.RouteTableList
//
//	if routeTableRef := getRouteTableRef(delegateAction); routeTableRef != nil {
//		// missing refs should only result in a warning
//		// this allows resources to be applied asynchronously
//		routeTable, err := rv.tables.Find((*routeTableRef).Strings())
//		if err != nil {
//			rv.addWarning(RouteTableMissingWarning(*routeTableRef))
//			return nil
//		}
//		routeTables = gatewayv1.RouteTableList{routeTable}
//
//	} else if rtSelector := delegateAction.GetSelector(); rtSelector != nil {
//		routeTables = RouteTablesForSelector(rv.tables, rtSelector, rv.rootResource.GetMetadata().Namespace)
//
//		if len(routeTables) == 0 {
//			rv.addWarning(NoMatchingRouteTablesWarning)
//			return nil
//		}
//	} else {
//		rv.addWarning(MissingRefAndSelectorWarning(rv.rootResource))
//		return nil
//	}
//	return routeTables
//}
//
//// Create a new visitor to visit the current route table
//func (rv *routeVisitor) createSubVisitor(routeTable *gatewayv1.RouteTable) *routeVisitor {
//	visitor := &routeVisitor{
//		rootResource:       routeTable,
//		tables:             rv.tables,
//		reports:            rv.reports,
//		nameTree:           rv.nameTree + "_rt:" + routeTable.Metadata.Name,
//		containsNamedRoute: rv.containsNamedRoute,
//	}
//
//	// Add all route tables from the parent visitor
//	for _, vis := range rv.visited {
//		visitor.visited = append(visitor.visited, vis)
//	}
//
//	// Add the route table that is the root for the new visitor
//	visitor.visited = append(visitor.visited, routeTable)
//
//	return visitor
//}
//
//// If any of the matching route tables has already been visited, that means we have a delegation cycle.
//func (rv *routeVisitor) checkForCycles(routeTables gatewayv1.RouteTableList) error {
//	for _, visited := range rv.visited {
//		for _, toVisit := range routeTables {
//			if toVisit == visited {
//				return DelegationCycleErr(
//					buildCycleInfoString(append(append(gatewayv1.RouteTableList{}, rv.visited...), toVisit)),
//				)
//			}
//		}
//	}
//	return nil
//}
//
//func (rv *routeVisitor) addWarning(message string) {
//	rv.reports.AddWarning(rv.rootResource, message)
//}
//
//func (rv *routeVisitor) addError(err error) {
//	rv.reports.AddError(rv.rootResource, err)
//}

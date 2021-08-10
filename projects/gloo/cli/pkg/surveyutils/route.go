package surveyutils

import (
	"context"
	"fmt"
	"sort"
	"strings"

	errors "github.com/rotisserie/eris"
	"github.com/solo-io/gloo/pkg/cliutil"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	plugins "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/aws"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/rest"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

const (
	pathMatch_Prefix = "prefix"
	pathMatch_Regex  = "regex"
	pathMatch_Exact  = "exact"
)

var pathMatchOptions = []string{
	pathMatch_Prefix,
	pathMatch_Regex,
	pathMatch_Exact,
}

const (
	NoneOfTheAbove = "None of the above"
)

func getMatcherInteractive(match *options.RouteMatchers) error {
	var pathType string
	if err := cliutil.ChooseFromList(
		"Choose a path match type: ",
		&pathType,
		pathMatchOptions,
	); err != nil {
		return err
	}
	if pathType == "" {
		return errors.Errorf("must specify one of %v", pathMatchOptions)
	}

	var pathMatch string
	if err := cliutil.GetStringInputDefault(
		fmt.Sprintf("What path %v should we match? ", pathType),
		&pathMatch,
		"/",
	); err != nil {
		return err
	}

	switch pathType {
	case "exact":
		match.PathExact = pathMatch
	case "regex":
		match.PathRegex = pathMatch
	case "prefix":
		match.PathPrefix = pathMatch
	default:
		return errors.Errorf("must specify one of %v", pathMatchOptions)
	}

	var headerMsgProvider = func() string {
		return fmt.Sprintf("Add a header matcher for this function (empty to skip)? %v", match.HeaderMatcher.Entries)
	}
	if err := cliutil.GetStringSliceInputLazyPrompt(headerMsgProvider, &match.HeaderMatcher.Entries); err != nil {
		return err
	}

	var httpMsgProvider = func() string {
		return fmt.Sprintf("HTTP Method to match for this route (empty to skip)? %v", match.Methods)
	}
	if err := cliutil.GetStringSliceInputLazyPrompt(httpMsgProvider, &match.Methods); err != nil {
		return err
	}

	return nil
}

func getDestinationInteractive(ctx context.Context, route *options.InputRoute) error {
	dest := &route.Destination
	// collect upstreams list
	ussByKey := make(map[string]*v1.Upstream)
	ugsByKey := make(map[string]*v1.UpstreamGroup)
	var usKeys []string
	for _, ns := range helpers.MustGetNamespaces(ctx) {
		usList, err := helpers.MustNamespacedUpstreamClient(ctx, ns).List(ns, clients.ListOpts{})
		if err != nil {
			return err
		}
		for _, us := range usList {
			ref := us.GetMetadata().Ref()
			ussByKey[ref.Key()] = us
			usKeys = append(usKeys, ref.Key())
		}
	}
	if len(usKeys) == 0 {
		return errors.Errorf("no upstreams found. create an upstream first or enable discovery.")
	}

	for _, ns := range helpers.MustGetNamespaces(ctx) {
		ugList, err := helpers.MustNamespacedUpstreamGroupClient(ctx, ns).List(ns, clients.ListOpts{})
		if err != nil {
			return err
		}
		for _, ug := range ugList {
			ref := ug.GetMetadata().Ref()
			key := "upstream-group: " + ref.Key()
			ugsByKey[key] = ug
			usKeys = append(usKeys, key)
		}
	}

	var usKey string
	if err := cliutil.ChooseFromList(
		"Choose the upstream or upstream group to route to: ",
		&usKey,
		usKeys,
	); err != nil {
		return err
	}

	if ug, ok := ugsByKey[usKey]; ok {
		route.UpstreamGroup = *(ug.GetMetadata().Ref())
		return nil
	}

	us, ok := ussByKey[usKey]
	if !ok {
		return errors.Errorf("internal error: upstream map not populated")
	}
	dest.Upstream = *(us.GetMetadata().Ref())
	switch ut := us.GetUpstreamType().(type) {
	case *v1.Upstream_Aws:
		if err := getAwsDestinationSpecInteractive(&dest.DestinationSpec.Aws, ut.Aws); err != nil {
			return err
		}
	case v1.ServiceSpecGetter:
		svcSpec := ut.GetServiceSpec()
		if svcSpec == nil {
			return nil
		}
		switch svcType := svcSpec.GetPluginType().(type) {
		case *plugins.ServiceSpec_Rest:
			if err := getRestDestinationSpecInteractive(&dest.DestinationSpec.Rest, svcType.Rest); err != nil {
				return err
			}
		}
	}
	return nil
}

func getPluginsInteractive(dest *options.RoutePlugins) error {
	yes, err := cliutil.GetYesInput("do you wish to add a prefix-rewrite transformation to the route [y/n]?\n" +
		"note that this will be overridden if your routes point to function destinations")
	if err != nil {
		return err
	}

	if !yes {
		return nil
	}

	var prefixRewrite string

	if err := cliutil.GetStringInput("rewrite the matched portion of HTTP requests with this prefix: ", &prefixRewrite); err != nil {
		return err
	}

	dest.PrefixRewrite.Value = &prefixRewrite

	return nil
}

func getAwsDestinationSpecInteractive(spec *options.AwsDestinationSpec, ut *aws.UpstreamSpec) error {
	var fnNames []string
	for _, fn := range ut.GetLambdaFunctions() {
		fnNames = append(fnNames, fn.GetLogicalName())
	}
	// Add the option to skip providing a function
	fnNames = append(fnNames, NoneOfTheAbove)
	if err := cliutil.ChooseFromList(
		"which function should this route invoke? ",
		&spec.LogicalName,
		fnNames,
	); err != nil {
		return err
	}

	return nil
}

func getRestDestinationSpecInteractive(spec *options.RestDestinationSpec, restSpec *rest.ServiceSpec) error {
	var fnNames []string
	for fn := range restSpec.GetTransformations() {
		fnNames = append(fnNames, fn)
	}
	sort.Strings(fnNames)
	// Add the option to skip providing a function
	fnNames = append(fnNames, NoneOfTheAbove)
	if err := cliutil.ChooseFromList(
		"which function should this route invoke? ",
		&spec.FunctionName,
		fnNames,
	); err != nil {
		return err
	}

	var headerMsgProvider = func() string {
		return fmt.Sprintf("Add a header parameter for this function (empty to skip)? %v", spec.Parameters.Entries)
	}
	if err := cliutil.GetStringSliceInputLazyPrompt(headerMsgProvider, &spec.Parameters.Entries); err != nil {
		return err
	}

	return nil
}

func AddRouteFlagsInteractive(opts *options.Options) error {
	// collect vs list
	vsByKey := make(map[string]*core.ResourceRef)
	vsKeys := []string{"create a new virtualservice"}
	var namespaces []string
	for _, ns := range helpers.MustGetNamespaces(opts.Top.Ctx) {
		namespaces = append(namespaces, ns)
		vsList, err := helpers.MustNamespacedVirtualServiceClient(opts.Top.Ctx, ns).List(ns, clients.ListOpts{})
		if err != nil {
			return err
		}
		for _, vs := range vsList {
			ref := vs.GetMetadata().Ref()
			vsByKey[ref.Key()] = ref
			vsKeys = append(vsKeys, ref.Key())
		}
	}

	var vsKey string
	if err := cliutil.ChooseFromList(
		"Choose a Virtual Service to add the route to: (empty to add "+
			"to a default virtual service. the default virtual service matches "+
			"the '*' domain and will be created if it does not exist) ",
		&vsKey,
		vsKeys,
	); err != nil {
		return err
	}
	opts.Metadata.Name = vsByKey[vsKey].GetName()
	opts.Metadata.Namespace = vsByKey[vsKey].GetNamespace()

	if opts.Metadata.GetName() == "" || opts.Metadata.GetNamespace() == "" {
		if err := cliutil.GetStringInput("name of the virtual service: ", &opts.Metadata.Name); err != nil {
			return err
		}
		if err := cliutil.ChooseFromList(
			"namespace of the virtual service: ",
			&opts.Metadata.Namespace,
			namespaces,
		); err != nil {
			return err
		}
	} else {
		// only get the insert index if the vs is predefined
		if err := cliutil.GetUint32Input(
			fmt.Sprintf("where do you want to insert the route in the "+
				"virtual service's route list? "),
			&opts.Add.Route.InsertIndex,
		); err != nil {
			return err
		}
	}

	if err := getMatcherInteractive(&opts.Add.Route.Matcher); err != nil {
		return err
	}
	if err := getDestinationInteractive(opts.Top.Ctx, &opts.Add.Route); err != nil {
		return err
	}
	if err := getPluginsInteractive(&opts.Add.Route.Plugins); err != nil {
		return err
	}

	return nil
}

func RemoveRouteFlagsInteractive(opts *options.Options) error {
	_, route, err := SelectRouteInteractive(opts, "Choose a Virtual Service from which to remove the route: ", "Choose the route you wish to remove: ")
	if err != nil {
		return err
	}
	opts.Remove.Route.RemoveIndex = uint32(route)
	return nil
}

func SelectRouteInteractive(opts *options.Options, virtualServicePrompt, routePrompt string) (*gatewayv1.VirtualService, int, error) {
	vsvc, err := SelectVirtualServiceInteractiveWithPrompt(opts, virtualServicePrompt)
	if err != nil {
		return nil, 0, err
	}
	routeindex, err := SelectRouteFromVirtualServiceInteractive(vsvc, routePrompt)
	return vsvc, routeindex, err
}

func SelectRouteFromVirtualServiceInteractive(vs *gatewayv1.VirtualService, routePrompt string) (int, error) {

	if vs.GetVirtualHost() == nil {
		return 0, errors.Errorf("invalid virtual service %v", vs.GetMetadata().Ref())
	}
	if len(vs.GetVirtualHost().GetRoutes()) == 0 {
		return 0, errors.Errorf("no routes defined for virtual service %v", vs.GetMetadata().Ref())
	}

	var routes []string
	for i, r := range vs.GetVirtualHost().GetRoutes() {
		routes = append(routes, fmt.Sprintf("%v: %+v", i, matchersString(r.GetMatchers())))
	}

	var chosenRoute string
	if err := cliutil.ChooseFromList(routePrompt,
		&chosenRoute,
		routes,
	); err != nil {
		return 0, err
	}

	for i, route := range routes {
		if route == chosenRoute {
			return i, nil
		}
	}

	return 0, errors.Errorf("can't find route")
}

func matchersString(matchers []*matchers.Matcher) string {
	var matchersStrings []string
	for _, matcher := range matchers {
		matchersStrings = append(matchersStrings, fmt.Sprintf("%+v", matcher))
	}
	return strings.Join(matchersStrings, ", ")
}

func SelectVirtualServiceInteractiveWithPrompt(opts *options.Options, prompt string) (*gatewayv1.VirtualService, error) {
	// collect vs list
	vsByKey := make(map[string]*gatewayv1.VirtualService)
	var vsKeys []string
	var namespaces []string
	for _, ns := range helpers.MustGetNamespaces(opts.Top.Ctx) {
		namespaces = append(namespaces, ns)
		vsList, err := helpers.MustNamespacedVirtualServiceClient(opts.Top.Ctx, ns).List(ns, clients.ListOpts{Ctx: opts.Top.Ctx})
		if err != nil {
			return nil, err
		}
		for _, vs := range vsList {
			ref := vs.GetMetadata().Ref()
			vsByKey[ref.Key()] = vs
			vsKeys = append(vsKeys, ref.Key())
		}
	}

	if len(vsKeys) == 0 {
		return nil, errors.Errorf("no virtual services found")
	}

	var vsKey string
	if err := cliutil.ChooseFromList(
		prompt,
		&vsKey,
		vsKeys,
	); err != nil {
		return nil, err
	}
	opts.Metadata.Name = vsByKey[vsKey].GetMetadata().Name
	opts.Metadata.Namespace = vsByKey[vsKey].GetMetadata().Namespace

	return vsByKey[vsKey], nil
}

func SelectVirtualServiceInteractive(opts *options.Options) error {
	_, err := SelectVirtualServiceInteractiveWithPrompt(opts, "Choose a Virtual Service: ")
	return err
}

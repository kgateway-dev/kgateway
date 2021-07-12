package translator

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	errors "github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/go-utils/hashutils"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
)

var (
	NoVirtualHostErr = func(vs *v1.VirtualService) error {
		return errors.Errorf("virtual service [%s] does not specify a virtual host", vs.Metadata.Ref().Key())
	}
	InvalidRegexErr = func(vsRef, regexErr string) error {
		return errors.Errorf("virtual service [%s] has a regex matcher with invalid regex, %s",
			vsRef, regexErr)
	}
	DomainInOtherVirtualServicesErr = func(domain string, conflictingVsRefs []string) error {
		if domain == "" {
			return errors.Errorf("domain conflict: other virtual services that belong to the same Gateway"+
				" as this one don't specify a domain (and thus default to '*'): %v", conflictingVsRefs)
		}
		return errors.Errorf("domain conflict: the [%s] domain is present in other virtual services "+
			"that belong to the same Gateway as this one: %v", domain, conflictingVsRefs)
	}
	GatewayHasConflictingVirtualServicesErr = func(conflictingDomains []string) error {
		var loggedDomains []string
		for _, domain := range conflictingDomains {
			if domain == "" {
				domain = "EMPTY_DOMAIN"
			}
			loggedDomains = append(loggedDomains, domain)
		}
		return errors.Errorf("domain conflict: the following domains are present in more than one of the "+
			"virtual services associated with this gateway: %v", loggedDomains)
	}
	ConflictingMatcherErr = func(vh string, matcher *matchers.Matcher) error {
		return errors.Errorf("virtual host [%s] has conflicting matcher: %v", vh, matcher)
	}
	UnorderedPrefixErr = func(vh, prefix string, matcher *matchers.Matcher) error {
		return errors.Errorf("virtual host [%s] has unordered prefix routes, earlier prefix [%s] short-circuited "+
			"later route [%v]", vh, prefix, matcher)
	}
	UnorderedRegexErr = func(vh, regex string, matcher *matchers.Matcher) error {
		return errors.Errorf("virtual host [%s] has unordered regex routes, earlier regex [%s] short-circuited "+
			"later route [%v]", vh, regex, matcher)
	}

	VirtualServiceSelectorInvalidExpressionWarning = errors.New("the virtual service selector expression is invalid")
	// Map connecting Gloo Virtual Services expression operator values and Kubernetes expression operator string values.
	VirtualServiceExpressionOperatorValues = map[v1.VirtualServiceSelectorExpressions_Expression_Operator]selection.Operator{
		v1.VirtualServiceSelectorExpressions_Expression_Equals:       selection.Equals,
		v1.VirtualServiceSelectorExpressions_Expression_DoubleEquals: selection.DoubleEquals,
		v1.VirtualServiceSelectorExpressions_Expression_NotEquals:    selection.NotEquals,
		v1.VirtualServiceSelectorExpressions_Expression_In:           selection.In,
		v1.VirtualServiceSelectorExpressions_Expression_NotIn:        selection.NotIn,
		v1.VirtualServiceSelectorExpressions_Expression_Exists:       selection.Exists,
		v1.VirtualServiceSelectorExpressions_Expression_DoesNotExist: selection.DoesNotExist,
		v1.VirtualServiceSelectorExpressions_Expression_GreaterThan:  selection.GreaterThan,
		v1.VirtualServiceSelectorExpressions_Expression_LessThan:     selection.LessThan,
	}
)

type HttpTranslator struct {
	WarnOnRouteShortCircuiting bool
}

func (t *HttpTranslator) GenerateListeners(ctx context.Context, snap *v1.ApiSnapshot, filteredGateways []*v1.Gateway, reports reporter.ResourceReports) []*gloov1.Listener {
	if len(snap.VirtualServices) == 0 {
		snapHash := hashutils.MustHash(snap)
		contextutils.LoggerFrom(ctx).Debugf("%v had no virtual services", snapHash)
		return nil
	}
	var result []*gloov1.Listener
	for _, gateway := range filteredGateways {
		if gateway.GetHttpGateway() == nil {
			continue
		}

		virtualServices := getVirtualServicesForGateway(gateway, snap.VirtualServices, reports)
		validateVirtualServiceDomains(gateway, virtualServices, reports)
		// Merge delegated options into route options
		// Route options specified on the Route override delegated options
		listener := t.desiredListenerForHttp(gateway, virtualServices, snap, reports)
		result = append(result, listener)
	}
	return result
}

// Errors will be added to the report object.
func validateVirtualServiceDomains(gateway *v1.Gateway, virtualServices v1.VirtualServiceList, reports reporter.ResourceReports) {

	// Index the virtual services for this gateway by the domain
	vsByDomain := map[string]v1.VirtualServiceList{}
	for _, vs := range virtualServices {

		// Add warning and skip if no virtual host
		if vs.VirtualHost == nil {
			reports.AddWarning(vs, NoVirtualHostErr(vs).Error())
			continue
		}

		// Not specifying any domains is not an error per se, but we need to check whether multiple virtual services
		// don't specify any, so we use the empty string as a placeholder in this function.
		domains := append([]string{}, vs.VirtualHost.Domains...)
		if len(domains) == 0 {
			domains = []string{""}
		}

		for _, domain := range domains {
			vsByDomain[domain] = append(vsByDomain[domain], vs)
		}
	}

	var conflictingDomains []string
	for domain, vsWithThisDomain := range vsByDomain {
		if len(vsWithThisDomain) > 1 {
			conflictingDomains = append(conflictingDomains, domain)
			for i, vs := range vsWithThisDomain {
				var conflictingVsNames []string
				for j, otherVs := range vsWithThisDomain {
					if i != j {
						conflictingVsNames = append(conflictingVsNames, otherVs.Metadata.Ref().Key())
					}
				}
				reports.AddError(vs, DomainInOtherVirtualServicesErr(domain, conflictingVsNames))
			}
		}
	}
	if len(conflictingDomains) > 0 {
		reports.AddError(gateway, GatewayHasConflictingVirtualServicesErr(conflictingDomains))
	}
}

func getVirtualServicesForGateway(gateway *v1.Gateway, virtualServices v1.VirtualServiceList, reports reporter.ResourceReports) v1.VirtualServiceList {

	var virtualServicesForGateway v1.VirtualServiceList
	for _, vs := range virtualServices {
		contains, err := GatewayContainsVirtualService(gateway, vs)
		if err != nil {
			reports.AddError(gateway, err)
			continue
		}
		if contains {
			virtualServicesForGateway = append(virtualServicesForGateway, vs)
		}
	}

	return virtualServicesForGateway
}

func GatewayContainsVirtualService(gateway *v1.Gateway, virtualService *v1.VirtualService) (bool, error) {
	httpGateway := gateway.GetHttpGateway()
	if httpGateway == nil {
		return false, nil
	}
	if gateway.Ssl != hasSsl(virtualService) {
		return false, nil
	}

	if httpGateway.VirtualServiceExpressions != nil {
		return virtualServiceValidForSelectorExpressions(virtualService, httpGateway.GetVirtualServiceExpressions(),
			httpGateway.VirtualServiceNamespaces)
	}
	if httpGateway.VirtualServiceSelector != nil {
		return virtualServiceMatchesLabels(virtualService, httpGateway.GetVirtualServiceSelector(),
			httpGateway.GetVirtualServiceNamespaces()), nil
	}
	// use individual refs to collect virtual services
	virtualServiceRefs := httpGateway.VirtualServices

	if len(virtualServiceRefs) == 0 {
		return virtualServiceNamespaceValidForGateway(httpGateway.GetVirtualServiceNamespaces(), virtualService), nil
	}

	vsRef := virtualService.Metadata.Ref()

	for _, ref := range virtualServiceRefs {
		if ref.Equal(vsRef) {
			return true, nil
		}
	}

	return false, nil
}
func virtualServiceMatchesLabels(virtualService *v1.VirtualService, validLabels map[string]string, virtualServiceNamespaces []string) bool {
	vsLabels := labels.Set(virtualService.Metadata.Labels)
	var labelSelector labels.Selector

	// Check whether labels match (strict equality)
	labelSelector = labels.SelectorFromSet(validLabels)
	return labelSelector.Matches(vsLabels) && virtualServiceNamespaceValidForGateway(virtualServiceNamespaces, virtualService)
}
func virtualServiceValidForSelectorExpressions(virtualService *v1.VirtualService, selector *v1.VirtualServiceSelectorExpressions, virtualServiceNamespaces []string) (bool, error) {

	vsLabels := labels.Set(virtualService.Metadata.Labels)
	// Check whether labels match (expression requirements)
	if len(selector.Expressions) > 0 {
		var requirements labels.Requirements
		for _, expression := range selector.Expressions {
			r, err := labels.NewRequirement(
				expression.Key,
				VirtualServiceExpressionOperatorValues[expression.Operator],
				expression.Values)
			if err != nil {
				return false, errors.Wrap(VirtualServiceSelectorInvalidExpressionWarning, err.Error())
			}
			requirements = append(requirements, *r)
		}
		if !virtualServiceLabelsMatchesExpressionRequirements(requirements, vsLabels) {
			return false, nil
		}
	}
	// check if the namespace is valid
	nsMatches := virtualServiceNamespaceValidForGateway(virtualServiceNamespaces, virtualService)
	return nsMatches, nil
}
func virtualServiceNamespaceValidForGateway(virtualServiceNamespaces []string, virtualService *v1.VirtualService) bool {
	if len(virtualServiceNamespaces) > 0 {
		for _, ns := range virtualServiceNamespaces {
			if ns == "*" || virtualService.Metadata.Namespace == ns {
				return true
			}
		}
		return false
	}

	// by default, virtual services will be discovered in all namespaces
	return true
}

// Asserts that the virtual service labels matches all of the expression requirements (logical AND).
func virtualServiceLabelsMatchesExpressionRequirements(requirements labels.Requirements, vsLabels labels.Set) bool {
	for _, r := range requirements {
		if !r.Matches(vsLabels) {
			return false
		}
	}
	return true
}
func hasSsl(vs *v1.VirtualService) bool {
	return vs.SslConfig != nil
}

func (t *HttpTranslator) desiredListenerForHttp(gateway *v1.Gateway, virtualServicesForGateway v1.VirtualServiceList, snapshot *v1.ApiSnapshot, reports reporter.ResourceReports) *gloov1.Listener {
	var (
		virtualHosts []*gloov1.VirtualHost
		sslConfigs   []*gloov1.SslConfig
	)

	for _, virtualService := range virtualServicesForGateway.Sort() {
		if virtualService.VirtualHost == nil {
			virtualService.VirtualHost = &v1.VirtualHost{}
		}
		vh, err := t.virtualServiceToVirtualHost(virtualService, snapshot, reports)
		if err != nil {
			reports.AddError(virtualService, err)
			continue
		}
		virtualHosts = append(virtualHosts, vh)
		if virtualService.SslConfig != nil {
			sslConfigs = append(sslConfigs, virtualService.SslConfig)
		}
	}

	var httpPlugins *gloov1.HttpListenerOptions
	if httpGateway := gateway.GetHttpGateway(); httpGateway != nil {
		httpPlugins = httpGateway.Options
	}
	listener := makeListener(gateway)
	listener.ListenerType = &gloov1.Listener_HttpListener{
		HttpListener: &gloov1.HttpListener{
			VirtualHosts: virtualHosts,
			Options:      httpPlugins,
		},
	}
	listener.SslConfigurations = sslConfigs

	if err := appendSource(listener, gateway); err != nil {
		// should never happen
		reports.AddError(gateway, err)
	}

	return listener
}

func (t *HttpTranslator) virtualServiceToVirtualHost(vs *v1.VirtualService, snapshot *v1.ApiSnapshot, reports reporter.ResourceReports) (*gloov1.VirtualHost, error) {
	converter := NewRouteConverter(NewRouteTableSelector(snapshot.RouteTables), NewRouteTableIndexer())
	t.mergeDelegatedVirtualHostOptions(vs, snapshot.VirtualHostOptions, reports)

	routes, err := converter.ConvertVirtualService(vs, snapshot, reports)
	if err != nil {
		// internal error, should never happen
		return nil, err
	}

	vh := &gloov1.VirtualHost{
		Name:    VirtualHostName(vs),
		Domains: vs.VirtualHost.Domains,
		Routes:  routes,
		Options: vs.VirtualHost.Options,
	}

	validateRoutes(vs, vh, reports)

	if t.WarnOnRouteShortCircuiting {
		validateRouteShortCircuiting(vs, vh, reports)
	}

	if err := appendSource(vh, vs); err != nil {
		// should never happen
		return nil, err
	}

	return vh, nil
}

// finds delegated VirtualHostOption Objects and merges the options into the virtual service
func (t *HttpTranslator) mergeDelegatedVirtualHostOptions(vs *v1.VirtualService, options v1.VirtualHostOptionList, reports reporter.ResourceReports) {
	optionRefs := vs.GetVirtualHost().GetOptionsConfigRefs().GetDelegateOptions()
	for _, optionRef := range optionRefs {
		vhOption, err := options.Find(optionRef.GetNamespace(), optionRef.GetName())
		if err != nil {
			reports.AddError(vs, err)
			continue
		}
		if vs.GetVirtualHost().GetOptions() == nil {
			vs.GetVirtualHost().Options = vhOption.GetOptions()
			continue
		}
		vs.GetVirtualHost().Options, err = mergeVirtualHostOptions(vs.GetVirtualHost().GetOptions(), vhOption.GetOptions())
		if err != nil {
			reports.AddError(vs, err)
		}
	}
}

func VirtualHostName(vs *v1.VirtualService) string {
	return fmt.Sprintf("%v.%v", vs.Metadata.Namespace, vs.Metadata.Name)
}

// this function is written with the assumption that the routes will not be modified afterwards,
// and are in their final sorted form
func validateRoutes(vs *v1.VirtualService, vh *gloov1.VirtualHost, reports reporter.ResourceReports) {
	for _, rt := range vh.Routes {
		for _, matcher := range rt.Matchers {
			_, err := regexp.Compile(matcher.GetRegex())
			if matcher.GetRegex() != "" && err != nil {
				reports.AddError(vs, InvalidRegexErr(vs.Metadata.Ref().Key(), err.Error()))
			}
		}
	}
}

// this function is written with the assumption that the routes will not be modified afterwards,
// and are in their final sorted form
func validateRouteShortCircuiting(vs *v1.VirtualService, vh *gloov1.VirtualHost, reports reporter.ResourceReports) {
	validateAnyDuplicateMatchers(vs, vh, reports)
	validatePrefixHijacking(vs, vh, reports)
	validateRegexHijacking(vs, vh, reports)
}

func validateAnyDuplicateMatchers(vs *v1.VirtualService, vh *gloov1.VirtualHost, reports reporter.ResourceReports) {
	// warn on duplicate matchers
	seenMatchers := make(map[uint64]bool)
	for _, rt := range vh.Routes {
		for _, matcher := range rt.Matchers {
			hash := hashutils.MustHash(matcher)
			if _, ok := seenMatchers[hash]; ok == true {
				reports.AddWarning(vs, ConflictingMatcherErr(vh.GetName(), matcher).Error())
			} else {
				seenMatchers[hash] = true
			}
		}
	}
}

func validatePrefixHijacking(vs *v1.VirtualService, vh *gloov1.VirtualHost, reports reporter.ResourceReports) {
	// warn on early prefix matchers that short-circuit later routes

	var seenPrefixMatchers []*matchers.Matcher
	for _, rt := range vh.Routes {
		for _, matcher := range rt.Matchers {
			// make sure the current matcher doesn't match any previously defined prefix.
			// this code is written with the assumption that the routes are already in their final order;
			// we are trying to help users avoid misconfiguration and short-circuiting errors
			for _, prefix := range seenPrefixMatchers {
				if prefixShortCircuits(matcher, prefix) && nonPathEarlyMatcherShortCircuitsLateMatcher(matcher, prefix) {
					reports.AddWarning(vs, UnorderedPrefixErr(vh.GetName(), prefix.GetPrefix(), matcher).Error())
				}
			}
			if matcher.GetPrefix() != "" {
				seenPrefixMatchers = append(seenPrefixMatchers, matcher)
			}
		}
	}
}

func prefixShortCircuits(laterMatcher, earlierMatcher *matchers.Matcher) bool {
	laterPath := utils.PathAsString(laterMatcher)
	return strings.HasPrefix(laterPath, earlierMatcher.GetPrefix()) && laterMatcher.CaseSensitive == earlierMatcher.CaseSensitive
}

func validateRegexHijacking(vs *v1.VirtualService, vh *gloov1.VirtualHost, reports reporter.ResourceReports) {
	// warn on early regex matchers that short-circuit later routes

	var seenRegexMatchers []*matchers.Matcher
	for _, rt := range vh.Routes {
		for _, matcher := range rt.Matchers {
			if matcher.GetRegex() != "" {
				seenRegexMatchers = append(seenRegexMatchers, matcher)
			} else {
				// make sure the current matcher doesn't match any previously defined regex.
				// this code is written with the assumption that the routes are already in their final order;
				// we are trying to help users avoid misconfiguration and short-circuiting errors
				for _, regex := range seenRegexMatchers {
					if regexShortCircuits(matcher, regex) && nonPathEarlyMatcherShortCircuitsLateMatcher(matcher, regex) {
						reports.AddWarning(vs, UnorderedRegexErr(vh.GetName(), regex.GetRegex(), matcher).Error())
					}
				}
			}
		}
	}
}

func regexShortCircuits(laterMatcher, earlierMatcher *matchers.Matcher) bool {
	laterPath := utils.PathAsString(laterMatcher)
	re, err := regexp.Compile(earlierMatcher.GetRegex())
	if err != nil {
		// invalid regex should already be reported on the virtual service
		return false
	}
	foundIndex := re.FindStringIndex(laterPath)
	// later matcher is always non-regex. to validate against the regex, we need to ensure that it's either
	// unset or set to false
	return foundIndex != nil && laterMatcher.CaseSensitive == nil || laterMatcher.CaseSensitive.GetValue() == false
}

// As future matcher APIs get added, this validation will need to be updated as well.
// If it gets too complex, consider modeling as a constraint satisfaction problem.
func nonPathEarlyMatcherShortCircuitsLateMatcher(laterMatcher, earlierMatcher *matchers.Matcher) bool {

	// we play a trick here to validate the methods by writing them as header
	// matchers and just reusing the header matcher logic
	earlyMatcher := *earlierMatcher
	if len(earlyMatcher.Methods) > 0 {
		earlyMatcher.Headers = append(earlyMatcher.Headers, &matchers.HeaderMatcher{
			Name:  ":method",
			Value: fmt.Sprintf("(%s)", strings.Join(earlyMatcher.Methods, "|")),
			Regex: true,
		})
	}

	lateMatcher := *laterMatcher
	if len(lateMatcher.Methods) > 0 {
		lateMatcher.Headers = append(lateMatcher.Headers, &matchers.HeaderMatcher{
			Name:  ":method",
			Value: fmt.Sprintf("(%s)", strings.Join(lateMatcher.Methods, "|")),
			Regex: true,
		})
	}

	queryParamsShortCircuited := earlyQueryParametersShortCircuitedLaterOnes(lateMatcher, earlyMatcher)
	headersShortCircuited := earlyHeaderMatchersShortCircuitLaterOnes(lateMatcher, earlyMatcher)
	return queryParamsShortCircuited && headersShortCircuited
}

// returns true if the query parameter matcher conditions (or lack thereof) on the early matcher can completely
// short-circuit the query parameter matcher conditions of the latter.
func earlyQueryParametersShortCircuitedLaterOnes(laterMatcher, earlyMatcher matchers.Matcher) bool {
	for _, earlyQpm := range earlyMatcher.QueryParameters {

		foundOverlappingCondition := false
		for _, laterQpm := range laterMatcher.QueryParameters {
			if earlyQpm.Name == laterQpm.Name {
				// we found an overlapping condition
				foundOverlappingCondition = true

				// let's check if the early condition overlaps the later one
				if earlyQpm.Regex && !laterQpm.Regex {
					re, err := regexp.Compile(earlyQpm.Value)
					if err != nil {
						// invalid regex should already be reported on the virtual service
						return false
					}
					foundIndex := re.FindStringIndex(laterQpm.Value)
					if foundIndex == nil {
						// early regex doesn't capture the later matcher
						return false
					}
				} else if !earlyQpm.Regex && !laterQpm.Regex {
					matches := earlyQpm.Value == laterQpm.Value || earlyQpm.Value == ""
					if !matches {
						// early and late have non-compatible conditions on the same query parameter matcher
						return false
					}
				} else {
					// either:
					//   - early header match is regex and late header match is regex
					//   - or early header match is not regex but late header match is regex
					// in both cases, we can't validate the constraint properly, so we mark
					// the route as not short-circuited to avoid reporting flawed warnings.
					return false
				}
			}
		}

		if !foundOverlappingCondition {
			// by default, this matcher cannot short-circuit because it is more specific
			return false
		}
	}

	// every single qpm matcher defined on the later matcher was short-circuited
	return true
}

// returns true if the header matcher conditions (or lack thereof) on the early matcher can completely short-circuit
// the header matcher conditions of the latter.
func earlyHeaderMatchersShortCircuitLaterOnes(laterMatcher, earlyMatcher matchers.Matcher) bool {
	for _, earlyHeaderMatcher := range earlyMatcher.Headers {

		foundOverlappingCondition := false

		for _, laterHeaderMatcher := range laterMatcher.Headers {
			if earlyHeaderMatcher.Name == laterHeaderMatcher.Name {
				// we found an overlapping condition
				foundOverlappingCondition = true

				// let's check if the early condition overlaps the later one
				if earlyHeaderMatcher.Regex && !laterHeaderMatcher.Regex {
					re, err := regexp.Compile(earlyHeaderMatcher.Value)
					if err != nil {
						// invalid regex should already be reported on the virtual service
						return false
					}
					foundIndex := re.FindStringIndex(laterHeaderMatcher.Value)
					if foundIndex == nil && !earlyHeaderMatcher.InvertMatch {
						// early regex doesn't capture the later matcher
						return false
					} else if foundIndex != nil && earlyHeaderMatcher.InvertMatch {
						// early regex captures the later matcher, but we invert the result.
						// so there are conflicting conditions here on the same matcher
						return false
					}
				} else if !earlyHeaderMatcher.Regex && !laterHeaderMatcher.Regex {
					matches := earlyHeaderMatcher.Value == laterHeaderMatcher.Value || earlyHeaderMatcher.Value == ""
					if !matches && !earlyHeaderMatcher.InvertMatch {
						// early and late have non-compatible conditions on the same header matcher
						return false
					} else if matches && earlyHeaderMatcher.InvertMatch {
						// early and late have compatible conditions on the same header matcher, but we invert
						// the result. so there are conflicting conditions here on the same matcher
						return false
					}
				} else {
					// either:
					//   - early header match is regex and late header match is regex
					//   - or early header match is not regex but late header match is regex
					// in both cases, we can't validate the constraint properly, so we mark
					// the route as not short-circuited to avoid reporting flawed warnings.

					if laterOrRegexPartiallyShortCircuited(laterHeaderMatcher, earlyHeaderMatcher) {
						continue
					}

					return false
				}
			}
		}

		if !foundOverlappingCondition {
			// by default, this matcher cannot short-circuit because it is more specific
			return false
		}
	}

	// every single header matcher defined on the later matcher was short-circuited
	return true
}

// special case to catch the following:
//	- matchers:
//	  - prefix: /foo
//      headers:
//	    - name: :method
//        value: GET
//        invertMatch: true
//    directResponseAction:
//      status: 405
//      body: 'Invalid HTTP Method'
//	...
//	- matchers:
//	  - methods:
//	    - GET
//	    - POST # this one cannot be reached
//      prefix: /foo
//    routeAction:
//	    ....
func laterOrRegexPartiallyShortCircuited(laterHeaderMatcher, earlyHeaderMatcher *matchers.HeaderMatcher) bool {

	// regex matches simple OR regex, e.g. (GET|POST|...)
	re, err := regexp.Compile("^\\([\\w]+([|[\\w]+)+\\)$")
	if err != nil {
		// invalid regex should already be reported on the virtual service
		return false
	}
	foundIndex := re.FindStringIndex(laterHeaderMatcher.Value)
	if foundIndex != nil {

		// regex matches, is a simple OR. we can try to do some additional validation

		matches := strings.Split(laterHeaderMatcher.Value[1:len(laterHeaderMatcher.Value)-1], "|")
		shortCircuitedMatchExists := false

		for _, match := range matches {
			if earlyHeaderMatcher.Regex {
				re, err := regexp.Compile(earlyHeaderMatcher.Value)
				if err != nil {
					// invalid regex should already be reported on the virtual service
					return false
				}
				foundIndex := re.FindStringIndex(match)
				if foundIndex != nil && !earlyHeaderMatcher.InvertMatch ||
					foundIndex == nil && earlyHeaderMatcher.InvertMatch {
					// one of the OR'ed conditions cannot be reached, likely an error!
					shortCircuitedMatchExists = true
				}
			} else {
				if match == earlyHeaderMatcher.Value && !earlyHeaderMatcher.InvertMatch ||
					match != earlyHeaderMatcher.Value && earlyHeaderMatcher.InvertMatch {
					// one of the OR'ed conditions cannot be reached, likely an error!
					shortCircuitedMatchExists = true
				}
			}
		}

		if shortCircuitedMatchExists {
			return true
		}
	}
	return false
}

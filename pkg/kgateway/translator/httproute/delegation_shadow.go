package httproute

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	apiconditions "github.com/kgateway-dev/kgateway/v2/api/conditions"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/query"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/translator/routeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	reports "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	statusreports "github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

// reportShadowedDelegatedMatches reports the rules of a delegation tree that can never
// receive traffic because another rule in the same tree resolves to an identical match
// and takes precedence. All routes of one tree share the parent's hostnames, so they
// land in the same virtual hosts, where Envoy only ever uses the first of them.
//
// Only collisions involving a delegated (child) rule are reported; duplicates between
// a parent's own rules follow the Gateway API tie-breaking rules and are left alone.
// The shadowed rule stays programmed and reports kgateway.dev/Programmed=True with
// Reason=RouteRuleShadowed, naming the rule that wins.
func reportShadowedDelegatedMatches(
	topLevel *query.RouteInfo,
	routes []ir.HttpRouteRuleMatchIR,
	baseReporter reports.Reporter,
) {
	// Group the leaf routes by their effective match. Delegating rules produce no route.
	groups := map[string]routeutils.SortableRoutes{}
	var delegated bool
	for i, r := range routes {
		if r.Delegates || r.Parent == nil || r.Parent.SourceObject == nil {
			continue
		}
		delegated = delegated || r.DelegatingParent != nil
		key := matchKey(r.Match)
		// The same comparison the listener sorts the virtual host's routes with, so
		// the winner reported here is the one Envoy serves.
		groups[key] = append(groups[key], &routeutils.SortableRoute{
			Route:       r,
			RouteObject: topLevel.Object.GetSourceObject(),
			Idx:         i,
		})
	}
	if !delegated {
		return
	}

	type reportKey struct {
		route     string
		parentRef string
	}
	msgs := map[reportKey]sets.Set[string]{}
	for _, group := range groups {
		if len(group) < 2 {
			continue
		}
		slices.SortStableFunc(group, func(a, b *routeutils.SortableRoute) int { return a.CompareTo(b) })
		winner := group[0].Route
		for _, s := range group[1:] {
			loser := s.Route
			if winner.DelegatingParent == nil && loser.DelegatingParent == nil {
				continue
			}
			src := loser.Parent.SourceObject
			key := reportKey{
				route:     src.GetNamespace() + "/" + src.GetName(),
				parentRef: statusreports.ParentString(loser.ParentRef),
			}
			if msgs[key] == nil {
				msgs[key] = sets.New[string]()
			}
			msgs[key].Insert(fmt.Sprintf("rule %s (%s) is shadowed by HTTPRoute %s/%s rule %s, which has an identical effective match and takes precedence",
				ruleLabel(loser), describeMatch(loser.Match),
				winner.Parent.Namespace, winner.Parent.Name, ruleLabel(winner)))

			parentReport := baseReporter.Route(src).ParentRef(&loser.ParentRef)
			// A dropped or replaced rule reports Programmed=False; never hide it.
			if prr, ok := parentReport.(*statusreports.ParentRefReport); ok {
				if cond := meta.FindStatusCondition(prr.Conditions, apiconditions.KgatewayConditionProgrammed); cond != nil &&
					cond.Status == metav1.ConditionFalse {
					continue
				}
			}
			parentReport.SetCondition(reports.RouteCondition{
				Type:    gwv1.RouteConditionType(apiconditions.KgatewayConditionProgrammed),
				Status:  metav1.ConditionTrue,
				Reason:  gwv1.RouteConditionReason(apiconditions.KgatewayReasonRouteRuleShadowed),
				Message: strings.Join(sets.List(msgs[key]), "; "),
			})
		}
	}
}

// matchKey returns a canonical form of an HTTPRouteMatch: two matches with the same key
// match exactly the same requests. Header names are case-insensitive, and the order of
// header and query param matches does not matter.
func matchKey(m gwv1.HTTPRouteMatch) string {
	var b strings.Builder
	pathType, pathValue := routeutils.ParsePath(m.Path)
	fmt.Fprintf(&b, "path=%s:%q", pathType, pathValue)
	if m.Method != nil {
		fmt.Fprintf(&b, ";method=%s", *m.Method)
	}
	headers := make([]string, 0, len(m.Headers))
	for _, h := range m.Headers {
		t := gwv1.HeaderMatchExact
		if h.Type != nil {
			t = *h.Type
		}
		headers = append(headers, fmt.Sprintf("%s:%q=%q", t, strings.ToLower(string(h.Name)), h.Value))
	}
	slices.Sort(headers)
	params := make([]string, 0, len(m.QueryParams))
	for _, q := range m.QueryParams {
		t := gwv1.QueryParamMatchExact
		if q.Type != nil {
			t = *q.Type
		}
		params = append(params, fmt.Sprintf("%s:%q=%q", t, q.Name, q.Value))
	}
	slices.Sort(params)
	fmt.Fprintf(&b, ";headers=[%s];query=[%s]", strings.Join(headers, ","), strings.Join(params, ","))
	return b.String()
}

func describeMatch(m gwv1.HTTPRouteMatch) string {
	pathType, pathValue := routeutils.ParsePath(m.Path)
	desc := fmt.Sprintf("%s %s", pathType, pathValue)
	if m.Method != nil || len(m.Headers) > 0 || len(m.QueryParams) > 0 {
		desc += " with method/header/query matches"
	}
	return desc
}

func ruleLabel(r ir.HttpRouteRuleMatchIR) string {
	if r.RuleName != "" {
		return r.RuleName
	}
	return strconv.Itoa(r.RuleIndex)
}

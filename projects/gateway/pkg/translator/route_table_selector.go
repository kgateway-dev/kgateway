package translator

import (
	errors "github.com/rotisserie/eris"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
)

// Reserved value for route table namespace selection.
// If a selector contains this value in its 'namespace' field, we match route tables from any namespace
const allNamespaceRouteTableSelector = "*"

var (
	RouteTableMissingWarning = func(ref core.ResourceRef) error {
		return errors.Errorf("route table %v.%v missing", ref.GetNamespace(), ref.GetName())
	}
	NoMatchingRouteTablesWarning = errors.New("no route table matches the given selector")
	MissingRefAndSelectorWarning = errors.New("cannot determine delegation target: you must specify a route table " +
		"either via a resource reference or a selector")
	RouteTableSelectorExpressionsAndLabelsWarning = errors.New("cannot use both labels and expressions within the " +
		"same selector")
	RouteTableSelectorInvalidExpressionWarning = errors.New("the route table selector expression is invalid")

	// Map connecting Gloo Route Tables expression operator values and Kubernetes expression operator string values.
	RouteTableExpressionOperatorValues = map[gatewayv1.RouteTableSelector_Expression_Operator]selection.Operator{
		gatewayv1.RouteTableSelector_Expression_Equals:       selection.Equals,
		gatewayv1.RouteTableSelector_Expression_DoubleEquals: selection.DoubleEquals,
		gatewayv1.RouteTableSelector_Expression_NotEquals:    selection.NotEquals,
		gatewayv1.RouteTableSelector_Expression_In:           selection.In,
		gatewayv1.RouteTableSelector_Expression_NotIn:        selection.NotIn,
		gatewayv1.RouteTableSelector_Expression_Exists:       selection.Exists,
		gatewayv1.RouteTableSelector_Expression_DoesNotExist: selection.DoesNotExist,
		gatewayv1.RouteTableSelector_Expression_GreaterThan:  selection.GreaterThan,
		gatewayv1.RouteTableSelector_Expression_LessThan:     selection.LessThan,
	}
)

type RouteTableSelector interface {
	SelectRouteTables(action *gatewayv1.DelegateAction, parentNamespace string) (gatewayv1.RouteTableList, error)
}

func NewRouteTableSelector(allRouteTables gatewayv1.RouteTableList) RouteTableSelector {
	return &selector{
		toSearch: allRouteTables,
	}
}

type selector struct {
	toSearch gatewayv1.RouteTableList
}

// When an error is returned, the returned list is empty
func (s *selector) SelectRouteTables(action *gatewayv1.DelegateAction, parentNamespace string) (gatewayv1.RouteTableList, error) {
	var routeTables gatewayv1.RouteTableList
	var err error

	if routeTableRef := getRouteTableRef(action); routeTableRef != nil {
		// missing refs should only result in a warning
		// this allows resources to be applied asynchronously if the validation webhook is configured to allow warnings
		routeTable, err := s.toSearch.Find((*routeTableRef).Strings())
		if err != nil {
			return nil, RouteTableMissingWarning(*routeTableRef)
		}
		routeTables = gatewayv1.RouteTableList{routeTable}

	} else if rtSelector := action.GetSelector(); rtSelector != nil {
		routeTables, err = RouteTablesForSelector(s.toSearch, rtSelector, parentNamespace)
		if err != nil {
			return nil, err
		}
		if len(routeTables) == 0 {
			return nil, NoMatchingRouteTablesWarning
		}
	} else {
		return nil, MissingRefAndSelectorWarning
	}
	return routeTables, nil
}

// Returns the subset of `routeTables` that matches the given `selector`.
// Search will be restricted to the `ownerNamespace` if the selector does not specify any namespaces.
func RouteTablesForSelector(routeTables gatewayv1.RouteTableList, selector *gatewayv1.RouteTableSelector, ownerNamespace string) (gatewayv1.RouteTableList, error) {
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
	if len(selector.GetNamespaces()) > 0 {
		nsSelector = list
	}
	for _, ns := range selector.GetNamespaces() {
		if ns == allNamespaceRouteTableSelector {
			nsSelector = all
		}
	}

	var labelSelector labels.Selector
	if len(selector.GetLabels()) > 0 {
		// expressions and labels cannot be both specified at the same time
		if len(selector.GetExpressions()) > 0 {
			return nil, RouteTableSelectorExpressionsAndLabelsWarning
		}
		labelSelector = labels.SelectorFromSet(selector.GetLabels())
	}

	var requirements labels.Requirements
	if len(selector.GetExpressions()) > 0 {
		for _, expression := range selector.GetExpressions() {
			r, err := labels.NewRequirement(
				expression.GetKey(),
				RouteTableExpressionOperatorValues[expression.GetOperator()],
				expression.GetValues())
			if err != nil {
				return nil, errors.Wrap(RouteTableSelectorInvalidExpressionWarning, err.Error())
			}
			requirements = append(requirements, *r)
		}
	}

	var matchingRouteTables gatewayv1.RouteTableList

	for _, candidate := range routeTables {
		rtLabels := labels.Set(candidate.GetMetadata().GetLabels())

		// Check whether labels match (strict equality)
		if labelSelector != nil {
			if !labelSelector.Matches(rtLabels) {
				continue
			}
		}

		// Check whether labels match (expression requirements)
		if requirements != nil {
			if !RouteTableLabelsMatchesExpressionRequirements(requirements, rtLabels) {
				continue
			}
		}

		// Check whether namespace matches
		nsMatches := false
		switch nsSelector {
		case all:
			nsMatches = true
		case owner:
			nsMatches = candidate.GetMetadata().GetNamespace() == ownerNamespace
		case list:
			for _, ns := range selector.GetNamespaces() {
				if ns == candidate.GetMetadata().GetNamespace() {
					nsMatches = true
				}
			}
		}

		if nsMatches {
			matchingRouteTables = append(matchingRouteTables, candidate)
		}
	}

	return matchingRouteTables, nil
}

// Asserts that the route table labels matches all of the expression requirements (logical AND).
func RouteTableLabelsMatchesExpressionRequirements(requirements labels.Requirements, rtLabels labels.Set) bool {
	for _, r := range requirements {
		if !r.Matches(rtLabels) {
			return false
		}
	}
	return true
}

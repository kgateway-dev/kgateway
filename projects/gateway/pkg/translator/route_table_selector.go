package translator

import (
	errors "github.com/rotisserie/eris"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"k8s.io/apimachinery/pkg/labels"
)

// Reserved value for route table namespace selection.
// If a selector contains this value in its 'namespace' field, we match route tables from any namespace
const allNamespaceRouteTableSelector = "*"

var (
	RouteTableMissingWarning = func(ref core.ResourceRef) error {
		return errors.Errorf("route table %v.%v missing", ref.Namespace, ref.Name)
	}
	NoMatchingRouteTablesWarning = errors.New("no route table matches the given selector")
	MissingRefAndSelectorWarning = errors.New("cannot determine delegation target: you must specify a route table " +
		"either via a resource reference or a selector")
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

// When an error is returned, the returned ist is empty
func (s *selector) SelectRouteTables(action *gatewayv1.DelegateAction, parentNamespace string) (gatewayv1.RouteTableList, error) {
	var routeTables gatewayv1.RouteTableList

	if routeTableRef := getRouteTableRef(action); routeTableRef != nil {
		// missing refs should only result in a warning
		// this allows resources to be applied asynchronously
		routeTable, err := s.toSearch.Find((*routeTableRef).Strings())
		if err != nil {
			return nil, RouteTableMissingWarning(*routeTableRef)
		}
		routeTables = gatewayv1.RouteTableList{routeTable}

	} else if rtSelector := action.GetSelector(); rtSelector != nil {
		routeTables = RouteTablesForSelector(s.toSearch, rtSelector, parentNamespace)

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
func RouteTablesForSelector(routeTables gatewayv1.RouteTableList, selector *gatewayv1.RouteTableSelector, ownerNamespace string) gatewayv1.RouteTableList {
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

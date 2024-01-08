package utils

import (
	"context"
	"fmt"
	"reflect"

	"github.com/solo-io/gloo/projects/gateway2/query"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// Finds all instances of the supplied filterTypes for the Rule supplied in the RouteContext.
// Should only be used for plugins that support multiple filters as part of a single Rule
func FindAppliedRouteFilters(
	routeCtx *plugins.RouteContext,
	filterTypes ...gwv1.HTTPRouteFilterType,
) []gwv1.HTTPRouteFilter {
	var appliedFilters []gwv1.HTTPRouteFilter
	for _, filter := range routeCtx.Rule.Filters {
		for _, filterType := range filterTypes {
			if filter.Type == filterType {
				appliedFilters = append(appliedFilters, filter)
			}
		}
	}
	return appliedFilters
}

// Finds the first instance of the filterType supplied in the Rule being processed.
// Returns nil if the Rule doesn't contain a filter of the provided Type
func FindAppliedRouteFilter(
	routeCtx *plugins.RouteContext,
	filterType gwv1.HTTPRouteFilterType,
) *gwv1.HTTPRouteFilter {
	// TODO: check full Filter list for duplicates and error?
	for _, filter := range routeCtx.Rule.Filters {
		if filter.Type == filterType {
			return &filter
		}
	}
	return nil
}

func FindExtensionRefFilter(
	routeCtx *plugins.RouteContext,
	gk schema.GroupKind,
) *gwv1.HTTPRouteFilter {
	// TODO: check full Filter list for duplicates and error?
	for _, filter := range routeCtx.Rule.Filters {
		if filter.Type == gwv1.HTTPRouteFilterExtensionRef {
			if filter.ExtensionRef.Group == gwv1.Group(gk.Group) && filter.ExtensionRef.Kind == gwv1.Kind(gk.Kind) {
				return &filter
			}
		}
	}
	return nil
}

func GetExtensionRefObj(
	ctx context.Context,
	routeCtx *plugins.RouteContext,
	queries query.GatewayQueries,
	extensionRef *gwv1.LocalObjectReference,
	obj client.Object,
) error {
	localObj, err := queries.GetLocalObjRef(ctx, queries.ObjToFrom(routeCtx.Route), *extensionRef)
	if err != nil {
		return err
	}
	if reflect.TypeOf(obj) != reflect.TypeOf(localObj) {
		return fmt.Errorf("types not equal")
	}
	elem := reflect.ValueOf(obj).Elem()
	if !elem.CanSet() {
		return fmt.Errorf("can't set value")
	}
	elem.Set(reflect.ValueOf(localObj).Elem())
	return nil
}

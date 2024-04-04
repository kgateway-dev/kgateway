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

// FindAppliedRouteFilters finds all instances of the supplied filterTypes for the Rule supplied in the RouteContext.
// Should only be used for plugins that support multiple filters as part of a single Rule
func FindAppliedRouteFilters(
	routeCtx *plugins.RouteContext,
	filterTypes ...gwv1.HTTPRouteFilterType,
) []gwv1.HTTPRouteFilter {
	if routeCtx.Rule == nil {
		return nil
	}
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

// FindAppliedRouteFilter finds the first instance of the filterType supplied in the Rule being processed.
// Returns nil if the Rule doesn't contain a filter of the provided Type
func FindAppliedRouteFilter(
	routeCtx *plugins.RouteContext,
	filterType gwv1.HTTPRouteFilterType,
) *gwv1.HTTPRouteFilter {
	if routeCtx.Rule == nil {
		return nil
	}
	// TODO: check full Filter list for duplicates and error?
	for _, filter := range routeCtx.Rule.Filters {
		if filter.Type == filterType {
			return &filter
		}
	}
	return nil
}

// FindExtensionRefFilter finds the first instance of an ExtensionRef filter that
// references the supplied GroupKind in the Rule being processed.
// Returns nil if the Rule doesn't contain a matching ExtensionRef filter
func FindExtensionRefFilter(
	routeCtx *plugins.RouteContext,
	gk schema.GroupKind,
) *gwv1.HTTPRouteFilter {
	if routeCtx.Rule == nil {
		return nil
	}
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

var (
	ErrTypesNotEqual = fmt.Errorf("types not equal")
	ErrNotSettable   = fmt.Errorf("can't set value")
)

// GetExtensionRefObj uses the provided query engine to retrieve an ExtensionRef object
// and set the value of `obj` to point to it.
// The type of `obj` must match the type referenced in the extensionRef and must be a pointer.
// An error will be returned if the Get was unsuccessful or if the type passed is not valid.
// A nil error indicates success and `obj` should be usable as normal.
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
		return fmt.Errorf(
			"%w: passed Obj typeOf: '%v' localObj typeOf: '%v'",
			ErrTypesNotEqual,
			reflect.TypeOf(obj),
			reflect.TypeOf(localObj),
		)
	}
	elem := reflect.ValueOf(obj).Elem()
	if !elem.CanSet() {
		return ErrNotSettable
	}
	elem.Set(reflect.ValueOf(localObj).Elem())
	return nil
}

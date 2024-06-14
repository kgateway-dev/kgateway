package utils

import (
	"reflect"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

// ShallowMerge sets dst to the value of src, if src is non-zero and dst is zero-valued or overwrite=true.
// It returns a boolean indicating whether src overwrote dst.
func ShallowMerge(dst, src reflect.Value, overwrite bool) bool {
	if !src.IsValid() {
		return false
	}

	if dst.CanSet() && !isEmptyValue(src) && (overwrite || isEmptyValue(dst)) {
		dst.Set(src)
		return true
	}

	return false
}

// Copied from some previous version of https://github.com/golang/go/blob/68a32ced0f7b1b9abf9fd948db53c668ef6b1c66/src/encoding/json/encode.go#L304
func isEmptyValue(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return v.Len() == 0
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Interface, reflect.Ptr:
		if v.IsNil() {
			return true
		}
		return isEmptyValue(v.Elem())
	case reflect.Func:
		return v.IsNil()
	case reflect.Invalid:
		return true
	}
	return false
}

// ShallowMergeRouteOptions merges the top-level fields of src into dst.
// The fields in dst that have non-zero values will not be overwritten.
// It performs a shallow merge of top-level fields only.
// It returns a boolean indicating whether any fields in src overwrote dst.
func ShallowMergeRouteOptions(dst, src *v1.RouteOptions) (*v1.RouteOptions, bool) {
	if src == nil {
		return dst, false
	}

	if dst == nil {
		return src.Clone().(*v1.RouteOptions), true
	}

	dstValue, srcValue := reflect.ValueOf(dst).Elem(), reflect.ValueOf(src).Elem()

	overwrote := false
	for i := range dstValue.NumField() {
		dstField, srcField := dstValue.Field(i), srcValue.Field(i)
		if srcOverride := ShallowMerge(dstField, srcField, false); srcOverride {
			overwrote = true
		}
	}

	return dst, overwrote
}

// ShallowMergeVirtualHostOptions merges the top-level fields of src into dst.
// The fields in dst that have non-zero values will not be overwritten.
// It performs a shallow merge of top-level fields only.
// It returns a boolean indicating whether any fields in src overwrote dst.
func ShallowMergeVirtualHostOptions(dst, src *v1.VirtualHostOptions) (*v1.VirtualHostOptions, bool) {
	if src == nil {
		return dst, false
	}

	if dst == nil {
		return src.Clone().(*v1.VirtualHostOptions), true
	}

	dstValue, srcValue := reflect.ValueOf(dst).Elem(), reflect.ValueOf(src).Elem()

	overwrote := false
	for i := range dstValue.NumField() {
		dstField, srcField := dstValue.Field(i), srcValue.Field(i)
		if srcOverride := ShallowMerge(dstField, srcField, false); srcOverride {
			overwrote = true
		}
	}

	return dst, overwrote
}

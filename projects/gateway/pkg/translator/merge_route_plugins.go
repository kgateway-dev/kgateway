package translator

import (
	"reflect"

	"github.com/gogo/protobuf/proto"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

func mergeRoutePlugins(dst, src *v1.RoutePlugins) (*v1.RoutePlugins, error) {
	if src == nil {
		return dst, nil
	}
	if dst != nil {
		dstValue, srcValue := reflect.ValueOf(dst).Elem(), reflect.ValueOf(src).Elem()

		for i := 0; i < dstValue.NumField(); i++ {
			dstField, srcField := dstValue.Field(i), srcValue.Field(i)
			shallowMerge(dstField, srcField, false)
		}

		return dst, nil
	}
	return proto.Clone(src).(*v1.RoutePlugins), nil
}

// sets src to dst, if src is non-zero and dest is zero-valued or overwrite=true.
func shallowMerge(dst, src reflect.Value, overwrite bool) {
	if !src.IsValid() {
		return
	}

	if dst.CanSet() && !isEmptyValue(src) && (overwrite || isEmptyValue(dst)) {
		dst.Set(src)
	}

	return
}

// From src/pkg/encoding/json/encode.go.
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

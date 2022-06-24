package translator

import (
	"reflect"

	"github.com/imdario/mergo"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/hcm"

	"github.com/golang/protobuf/proto"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

// Merges the fields of src into dst.
// The fields in dst that have non-zero values will not be overwritten.
func mergeRouteOptions(dst, src *v1.RouteOptions) *v1.RouteOptions {
	if src == nil {
		return dst
	}

	if dst == nil {
		return proto.Clone(src).(*v1.RouteOptions)
	}

	dstValue, srcValue := reflect.ValueOf(dst).Elem(), reflect.ValueOf(src).Elem()

	for i := 0; i < dstValue.NumField(); i++ {
		dstField, srcField := dstValue.Field(i), srcValue.Field(i)
		shallowMerge(dstField, srcField, false)
	}

	return dst
}

// Merges the fields of src into dst.
// The fields in dst that have non-zero values will not be overwritten.
func mergeVirtualHostOptions(dst, src *v1.VirtualHostOptions) *v1.VirtualHostOptions {
	if src == nil {
		return dst
	}

	if dst == nil {
		return proto.Clone(src).(*v1.VirtualHostOptions)
	}

	dstValue, srcValue := reflect.ValueOf(dst).Elem(), reflect.ValueOf(src).Elem()

	for i := 0; i < dstValue.NumField(); i++ {
		dstField, srcField := dstValue.Field(i), srcValue.Field(i)
		shallowMerge(dstField, srcField, false)
	}

	return dst
}

// Sets dst to the value of src, if src is non-zero and dest is zero-valued or overwrite=true.
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

func mergeSslConfig(parent, child *v1.SslConfig, preventChildOverrides bool) *v1.SslConfig {
	// Clone to be safe, since we will mutate it
	parentClone := proto.Clone(parent).(*v1.SslConfig)
	childClone := proto.Clone(child).(*v1.SslConfig)

	if childClone == nil {
		// use parent exactly as-is
		return parentClone
	}
	if parent == nil {
		// use child exactly as-is
		return childClone
	}

	if preventChildOverrides {
		// merge, preferring parent
		mergo.Merge(childClone, parentClone, mergo.WithOverride)
	} else {
		// merge, preferring child
		mergo.Merge(childClone, parentClone)
	}

	return childClone
}

func mergeHCMSettings(parent, child *hcm.HttpConnectionManagerSettings, preventChildOverrides bool) *hcm.HttpConnectionManagerSettings {
	// Clone to be safe, since we will mutate it
	parentClone := proto.Clone(parent).(*hcm.HttpConnectionManagerSettings)
	childClone := proto.Clone(child).(*hcm.HttpConnectionManagerSettings)

	if childClone == nil {
		// use parent exactly as-is
		return parentClone
	}
	if parentClone == nil {
		// use child exactly as-is
		return childClone
	}

	if preventChildOverrides {
		// merge, preferring parent
		mergo.Merge(childClone, parentClone, mergo.WithOverride)
	} else {
		// merge, preferring child
		mergo.Merge(childClone, parentClone)
	}

	return childClone
}

package serviceconverter

import (
	"reflect"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/utils/protoutils"
	"google.golang.org/protobuf/proto"
	kubev1 "k8s.io/api/core/v1"
)

const GlooAnnotationPrefix = "gloo.solo.io/UpstreamConfig"

type GeneralServiceConverter struct{}

func (s *GeneralServiceConverter) ConvertService(svc *kubev1.Service, port kubev1.ServicePort, us *v1.Upstream) error {
	upstreamConfigJson, ok := svc.Annotations[GlooAnnotationPrefix]
	if !ok {
		return nil
	}

	var spec v1.Upstream
	if err := protoutils.UnmarshalResource([]byte(upstreamConfigJson), &spec); err != nil {
		return err
	}

	mergeUpstreams(&spec, us)

	return nil
}

// Merges the fields of src into dst.
func mergeUpstreams(src, dst *v1.Upstream) (*v1.Upstream, error) {
	if src == nil {
		return dst, nil
	}

	if dst == nil {
		return proto.Clone(src).(*v1.Upstream), nil
	}

	dstValue, srcValue := reflect.ValueOf(dst).Elem(), reflect.ValueOf(src).Elem()

	for i := 0; i < dstValue.NumField(); i++ {
		dstField, srcField := dstValue.Field(i), srcValue.Field(i)

		if srcField.IsValid() && dstField.CanSet() && !isEmptyValue(srcField) {
			dstField.Set(srcField)
		}
	}

	return dst, nil
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

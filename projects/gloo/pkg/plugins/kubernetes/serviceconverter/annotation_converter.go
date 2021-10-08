package serviceconverter

import (
	"reflect"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/utils/protoutils"
	kubev1 "k8s.io/api/core/v1"
)

func init() {
	AdditionalServiceConverters = append(AdditionalServiceConverters, &GeneralServiceConverter{})
}

const GlooAnnotationPrefix = "gloo.solo.io/UpstreamConfig"

var ExcludedFields = map[string]bool{
	"NamespacedStatuses": true,
	"Metadata":           true,
	"DiscoveryMetadata":  true,
}

type GeneralServiceConverter struct{}

func (s *GeneralServiceConverter) ConvertService(svc *kubev1.Service, port kubev1.ServicePort, us *v1.Upstream) error {
	if upstreamConfigJson, ok := svc.Annotations[GlooAnnotationPrefix]; ok {
		var spec v1.Upstream

		if err := protoutils.UnmarshalResource([]byte(upstreamConfigJson), &spec); err != nil {
			return err
		}

		// iterate over fields in upstream spec
		specType := reflect.TypeOf(spec)
		numFields := specType.NumField()
		for i := 0; i < numFields; i++ {
			field := specType.Field(i)
			// if field is exported and not explicitly excluded, consider setting it on the upstream
			if field.PkgPath == "" && !ExcludedFields[field.Name] {
				fieldValue := getAttr(&spec, field.Name)
				currentValue := getAttr(us, field.Name)
				if fieldValue.IsValid() && currentValue != fieldValue {
					currentValue.Set(fieldValue)
				}
			}
		}
	}

	return nil
}

func getAttr(obj interface{}, fieldName string) reflect.Value {
	pointToStruct := reflect.ValueOf(obj) // addressable
	curStruct := pointToStruct.Elem()
	if curStruct.Kind() != reflect.Struct {
		panic("not struct")
	}
	curField := curStruct.FieldByName(fieldName) // type: reflect.Value
	if !curField.IsValid() {
		panic("not found:" + fieldName)
	}
	return curField
}

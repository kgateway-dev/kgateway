package serviceconverter

import (
	"encoding/json"
	"reflect"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/utils/protoutils"
	"google.golang.org/protobuf/proto"
	kubev1 "k8s.io/api/core/v1"
)

const GlooAnnotationPrefix = "gloo.solo.io/upstream_config"

type GeneralServiceConverter struct{}

func (s *GeneralServiceConverter) ConvertService(svc *kubev1.Service, port kubev1.ServicePort, us *v1.Upstream) error {
	upstreamConfigJson, ok := svc.Annotations[GlooAnnotationPrefix]
	if !ok {
		return nil
	}

	var upstreamConfigMap map[string]interface{}
	if err := json.Unmarshal([]byte(upstreamConfigJson), &upstreamConfigMap); err != nil {
		return err
	}

	upstreamConfig := v1.Upstream{}
	if err := protoutils.UnmarshalMap(upstreamConfigMap, &upstreamConfig); err != nil {
		return err
	}

	mergeUpstreams(&upstreamConfig, us)
	return nil
}

// Merges the fields of src into dst.
func mergeUpstreams(src, dst *v1.Upstream) {
	if src == nil {
		return
	}

	if dst == nil {
		dst = proto.Clone(src).(*v1.Upstream)
		return
	}

	dstValue, srcValue := reflect.ValueOf(dst).Elem(), reflect.ValueOf(src).Elem()

	for i := 0; i < dstValue.NumField(); i++ {
		dstField, srcField := dstValue.Field(i), srcValue.Field(i)

		if srcField.IsValid() && dstField.CanSet() && !srcField.IsZero() {
			fieldName := reflect.Indirect(reflect.ValueOf(dst)).Type().Field(i).Name
			// Information critical to proper UDS operation is contained in these fields,
			// so do not allow this serviceconverter to overwrite them.
			if fieldName != "Metadata" && fieldName != "DiscoveryMetadata" && fieldName != "NamespacedStatuses" {
				dstField.Set(srcField)
			}
		}
	}
}

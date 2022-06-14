package serviceconverter

import (
	"context"
	"encoding/json"
	"reflect"

	"github.com/solo-io/gloo/pkg/utils/settingsutil"

	"github.com/imdario/mergo"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/utils/protoutils"
	"google.golang.org/protobuf/proto"
	kubev1 "k8s.io/api/core/v1"
)

const GlooAnnotationPrefix = "gloo.solo.io/upstream_config"
const DeepMergeAnnotationPrefix = "gloo.solo.io/upstream_config.deep_merge"

type GeneralServiceConverter struct{}

func (s *GeneralServiceConverter) ConvertService(ctx context.Context, svc *kubev1.Service, port kubev1.ServicePort, us *v1.Upstream) error {
	// Global upstream configuration settings
	if globalAnnotations := settingsutil.MaybeFromContext(ctx).GetUpstreamOptions().GetGlobalAnnotations(); globalAnnotations != nil {
		if err := applyAnnotations(globalAnnotations, us); err != nil {
			return err
		}
	}

	// Service-specific annotation are applied afterwards to override global annotations
	return applyAnnotations(svc.Annotations, us)
}

// Applies annotations to the created upstream, overriding configuration provided in earlier calls.
// Will only return an error if annotations are provided but cannot be applied
// (typically a marshalling error due to an incorrect setting key)
func applyAnnotations(annotations map[string]string, us *v1.Upstream) error {
	upstreamConfigJson, ok := annotations[GlooAnnotationPrefix]
	if !ok {
		return nil
	}

	deepMerge, ok := annotations[DeepMergeAnnotationPrefix]
	performShallowMerge := !ok || deepMerge != "true"

	if err := applyUpstreamConfig(upstreamConfigJson, performShallowMerge, us); err != nil {
		return err
	}
	return nil
}
func applyUpstreamConfig(upstreamConfigJson string, performShallowMerge bool, us *v1.Upstream) error {
	var upstreamConfigMap map[string]interface{}
	if err := json.Unmarshal([]byte(upstreamConfigJson), &upstreamConfigMap); err != nil {
		return err
	}

	upstreamConfig := v1.Upstream{}
	if err := protoutils.UnmarshalMap(upstreamConfigMap, &upstreamConfig); err != nil {
		return err
	}

	var err error
	if performShallowMerge {
		err = shallowMergeUpstreams(&upstreamConfig, us)
	} else {
		err = deepMergeUpstreams(&upstreamConfig, us)
	}

	if err != nil {
		return err
	}
	return nil
}

// Merges the fields of src into dst.
func shallowMergeUpstreams(src, dst *v1.Upstream) error {
	if src == nil {
		return nil
	}

	if dst == nil {
		dst = proto.Clone(src).(*v1.Upstream)
		return nil
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
	return nil
}

// Deep merges the fields of src into dst.
func deepMergeUpstreams(src, dst *v1.Upstream) error {
	if err := mergo.Merge(src, dst); err != nil {
		return err
	}

	*dst = *src
	return nil
}

package translator

import (
	"reflect"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/duration"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/imdario/mergo"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/hcm"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/ssl"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

func mergeSslConfig(parent, child *ssl.SslConfig, preventChildOverrides bool) *ssl.SslConfig {
	if child == nil {
		// use parent exactly as-is
		return proto.Clone(parent).(*ssl.SslConfig)
	}
	if parent == nil {
		// use child exactly as-is
		return proto.Clone(child).(*ssl.SslConfig)
	}

	// Clone child to be safe, since we will mutate it
	childClone := proto.Clone(child).(*ssl.SslConfig)
	mergo.Merge(childClone, parent, mergo.WithTransformers(wrapperTransformer{preventChildOverrides}))
	return childClone
}

func mergeHCMSettings(parent, child *hcm.HttpConnectionManagerSettings, preventChildOverrides bool) *hcm.HttpConnectionManagerSettings {
	// Clone to be safe, since we will mutate it
	if child == nil {
		// use parent exactly as-is
		return proto.Clone(parent).(*hcm.HttpConnectionManagerSettings)
	}
	if parent == nil {
		// use child exactly as-is
		return proto.Clone(child).(*hcm.HttpConnectionManagerSettings)
	}

	// Clone child to be safe, since we will mutate it
	childClone := proto.Clone(child).(*hcm.HttpConnectionManagerSettings)
	mergo.Merge(childClone, parent, mergo.WithTransformers(wrapperTransformer{preventChildOverrides}))
	return childClone
}

type wrapperTransformer struct {
	preventChildOverrides bool
}

func (t wrapperTransformer) Transformer(typ reflect.Type) func(dst, src reflect.Value) error {
	if typ == reflect.TypeOf(wrappers.BoolValue{}) ||
		typ == reflect.TypeOf(wrappers.StringValue{}) ||
		typ == reflect.TypeOf(wrappers.UInt32Value{}) ||
		typ == reflect.TypeOf(duration.Duration{}) ||
		typ == reflect.TypeOf(core.ResourceRef{}) {
		return func(dst, src reflect.Value) error {
			if t.preventChildOverrides {
				dst.Set(src)
			}
			return nil
		}
	}
	return nil
}

package api_conversion

import (
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_type_v3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	envoycore_sk "github.com/solo-io/solo-kit/pkg/api/external/envoy/api/v2/core"
	envoytype_sk "github.com/solo-io/solo-kit/pkg/api/external/envoy/type"
)

// Converts between Envoy and Gloo/solokit versions of envoy protos
// This is required because go-control-plane dropped gogoproto in favor of goproto
// in v0.9.0, but solokit depends on gogoproto (and the generated deep equals it creates).
//
// we should work to remove that assumption from solokit and delete this code:
// https://github.com/solo-io/gloo/issues/1793

func ToGlooInt64RangeList(int64Range []*envoy_type_v3.Int64Range) []*envoytype_sk.Int64Range {
	result := make([]*envoytype_sk.Int64Range, len(int64Range))
	for i, v := range int64Range {
		result[i] = ToGlooInt64Range(v)
	}
	return result
}

func ToGlooInt64Range(int64Range *envoy_type_v3.Int64Range) *envoytype_sk.Int64Range {
	return &envoytype_sk.Int64Range{
		Start: int64Range.Start,
		End:   int64Range.End,
	}
}

func ToEnvoyInt64RangeList(int64Range []*envoytype_sk.Int64Range) []*envoy_type_v3.Int64Range {
	result := make([]*envoy_type_v3.Int64Range, len(int64Range))
	for i, v := range int64Range {
		result[i] = ToEnvoyInt64Range(v)
	}
	return result
}

func ToEnvoyInt64Range(int64Range *envoytype_sk.Int64Range) *envoy_type_v3.Int64Range {
	return &envoy_type_v3.Int64Range{
		Start: int64Range.Start,
		End:   int64Range.End,
	}
}

func ToEnvoyHeaderValueOptionList(option []*envoycore_sk.HeaderValueOption, secrets *v1.SecretList) ([]*envoy_config_core_v3.HeaderValueOption, error) {
	result := make([]*envoy_config_core_v3.HeaderValueOption, 0)
	var err error
	var opts []*envoy_config_core_v3.HeaderValueOption
	for _, v := range option {
		opts, err = ToEnvoyHeaderValueOptions(v, secrets)
		if err != nil {
			return nil, err
		}
		result = append(result, opts...)
	}
	return result, nil
}

func ToEnvoyHeaderValueOptions(option *envoycore_sk.HeaderValueOption, secrets *v1.SecretList) ([]*envoy_config_core_v3.HeaderValueOption, error) {
	return []*envoy_config_core_v3.HeaderValueOption{
		{
			Header: &envoy_config_core_v3.HeaderValue{
				Key:   option.Header.GetKey(),
				Value: option.Header.GetValue(),
			},
			Append: option.GetAppend(),
		},
	}, nil
}

func ToGlooHeaderValueOptionList(option []*envoy_config_core_v3.HeaderValueOption) []*envoycore_sk.HeaderValueOption {
	result := make([]*envoycore_sk.HeaderValueOption, len(option))
	for i, v := range option {
		result[i] = ToGlooHeaderValueOption(v)
	}
	return result
}

func ToGlooHeaderValueOption(option *envoy_config_core_v3.HeaderValueOption) *envoycore_sk.HeaderValueOption {
	return &envoycore_sk.HeaderValueOption{
		Header: &envoycore_sk.HeaderValue{
			Key:   option.GetHeader().GetKey(),
			Value: option.GetHeader().GetValue(),
		},
		Append: option.GetAppend(),
	}
}

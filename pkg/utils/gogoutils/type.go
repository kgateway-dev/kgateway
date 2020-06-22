package gogoutils

import (
	envoycore "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoytype "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	envoytype_gloo "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/type"
	envoycore_sk "github.com/solo-io/solo-kit/pkg/api/external/envoy/api/v2/core"
)

// Converts between Envoy and Gloo/solokit versions of envoy protos
// This is required because go-control-plane dropped gogoproto in favor of goproto
// in v0.9.0, but solokit depends on gogoproto (and the generated deep equals it creates).
//
// we should work to remove that assumption from solokit and delete this code:
// https://github.com/solo-io/gloo/issues/1793

func ToGlooInt64RangeList(int64Range []*envoytype.Int64Range) []*envoytype_gloo.Int64Range {
	result := make([]*envoytype_gloo.Int64Range, len(int64Range))
	for i, v := range int64Range {
		result[i] = ToGlooInt64Range(v)
	}
	return result
}

func ToGlooInt64Range(int64Range *envoytype.Int64Range) *envoytype_gloo.Int64Range {
	return &envoytype_gloo.Int64Range{
		Start: int64Range.Start,
		End:   int64Range.End,
	}
}

func ToEnvoyInt64RangeList(int64Range []*envoytype_gloo.Int64Range) []*envoytype.Int64Range {
	result := make([]*envoytype.Int64Range, len(int64Range))
	for i, v := range int64Range {
		result[i] = ToEnvoyInt64Range(v)
	}
	return result
}

func ToEnvoyInt64Range(int64Range *envoytype_gloo.Int64Range) *envoytype.Int64Range {
	return &envoytype.Int64Range{
		Start: int64Range.Start,
		End:   int64Range.End,
	}
}

func ToEnvoyHeaderValueOptionList(option []*envoycore_sk.HeaderValueOption) []*envoycore.HeaderValueOption {
	result := make([]*envoycore.HeaderValueOption, len(option))
	for i, v := range option {
		result[i] = ToEnvoyHeaderValueOption(v)
	}
	return result
}

func ToEnvoyHeaderValueOption(option *envoycore_sk.HeaderValueOption) *envoycore.HeaderValueOption {
	return &envoycore.HeaderValueOption{
		Header: &envoycore.HeaderValue{
			Key:   option.GetHeader().GetKey(),
			Value: option.GetHeader().GetValue(),
		},
		Append: BoolGogoToProto(option.GetAppend()),
	}
}

func ToGlooHeaderValueOptionList(option []*envoycore.HeaderValueOption) []*envoycore_sk.HeaderValueOption {
	result := make([]*envoycore_sk.HeaderValueOption, len(option))
	for i, v := range option {
		result[i] = ToGlooHeaderValueOption(v)
	}
	return result
}

func ToGlooHeaderValueOption(option *envoycore.HeaderValueOption) *envoycore_sk.HeaderValueOption {
	return &envoycore_sk.HeaderValueOption{
		Header: &envoycore_sk.HeaderValue{
			Key:   option.GetHeader().GetKey(),
			Value: option.GetHeader().GetValue(),
		},
		Append: BoolProtoToGogo(option.GetAppend()),
	}
}

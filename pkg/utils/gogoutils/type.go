package gogoutils

import (
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoy_type "github.com/envoyproxy/go-control-plane/envoy/type"
	_type "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/type"
	"github.com/solo-io/solo-kit/pkg/api/external/envoy/api/v2/core"
)

func ToGlooInt64RangeList(int64Range []*envoy_type.Int64Range) []*_type.Int64Range {
	result := make([]*_type.Int64Range, len(int64Range))
	for i, v := range int64Range {
		result[i] = ToGlooInt64Range(v)
	}
	return result
}

func ToGlooInt64Range(int64Range *envoy_type.Int64Range) *_type.Int64Range {
	return &_type.Int64Range{
		Start: int64Range.Start,
		End:   int64Range.End,
	}
}

func ToEnvoyInt64RangeList(int64Range []*_type.Int64Range) []*envoy_type.Int64Range {
	result := make([]*envoy_type.Int64Range, len(int64Range))
	for i, v := range int64Range {
		result[i] = ToEnvoyInt64Range(v)
	}
	return result
}

func ToEnvoyInt64Range(int64Range *_type.Int64Range) *envoy_type.Int64Range {
	return &envoy_type.Int64Range{
		Start: int64Range.Start,
		End:   int64Range.End,
	}
}

func ToEnvoyHeaderValueOptionList(option []*core.HeaderValueOption) []*envoycore.HeaderValueOption {
	result := make([]*envoycore.HeaderValueOption, len(option))
	for i, v := range option {
		result[i] = ToEnvoyHeaderValueOption(v)
	}
	return result
}

func ToEnvoyHeaderValueOption(option *core.HeaderValueOption) *envoycore.HeaderValueOption {
	return &envoycore.HeaderValueOption{
		Header: &envoycore.HeaderValue{
			Key:   option.GetHeader().GetKey(),
			Value: option.GetHeader().GetValue(),
		},
		Append: BoolGogoToProto(option.GetAppend()),
	}
}

func ToGlooHeaderValueOptionList(option []*envoycore.HeaderValueOption) []*core.HeaderValueOption {
	result := make([]*core.HeaderValueOption, len(option))
	for i, v := range option {
		result[i] = ToGlooHeaderValueOption(v)
	}
	return result
}

func ToGlooHeaderValueOption(option *envoycore.HeaderValueOption) *core.HeaderValueOption {
	return &core.HeaderValueOption{
		Header: &core.HeaderValue{
			Key:   option.GetHeader().GetKey(),
			Value: option.GetHeader().GetValue(),
		},
		Append: BoolProtoToGogo(option.GetAppend()),
	}
}

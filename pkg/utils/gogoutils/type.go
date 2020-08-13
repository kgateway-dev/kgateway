package gogoutils

import (
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoytype "github.com/envoyproxy/go-control-plane/envoy/type"
	envoytype_gloo "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/type"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	envoycore_sk "github.com/solo-io/solo-kit/pkg/api/external/envoy/api/v2/core"
	"github.com/solo-io/solo-kit/pkg/errors"
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

func ToEnvoyHeaderValueOptionList(option []*envoycore_sk.HeaderValueOption, secrets *v1.SecretList) ([]*envoycore.HeaderValueOption, error) {
	result := make([]*envoycore.HeaderValueOption, 0)
	var err error
	var opts []*envoycore.HeaderValueOption
	for _, v := range option {
		opts, err = ToEnvoyHeaderValueOptions(v, secrets)
		if err != nil {
			return nil, err
		}
		result = append(result, opts...)
	}
	return result, nil
}

func ToEnvoyHeaderValueOptions(option *envoycore_sk.HeaderValueOption, secrets *v1.SecretList) ([]*envoycore.HeaderValueOption, error) {
	switch typedOption := option.HeaderOption.(type) {
	case *envoycore_sk.HeaderValueOption_Header:
		return []*envoycore.HeaderValueOption{
			{
				Header: &envoycore.HeaderValue{
					Key:   typedOption.Header.GetKey(),
					Value: typedOption.Header.GetValue(),
				},
				Append: BoolGogoToProto(option.GetAppend()),
			},
		}, nil
	case *envoycore_sk.HeaderValueOption_HeaderSecretRef:
		secret, err := secrets.Find(typedOption.HeaderSecretRef.GetNamespace(), typedOption.HeaderSecretRef.GetName())
		if err != nil {
			return nil, err
		}

		headerSecrets, ok := secret.Kind.(*v1.Secret_Header)
		if !ok {
			return nil, errors.Errorf("Secret %v.%v was not a Header secret", typedOption.HeaderSecretRef.GetNamespace(), typedOption.HeaderSecretRef.GetName())
		}

		result := make([]*envoycore.HeaderValueOption, 0)
		for key, value := range headerSecrets.Header.GetHeaders() {
			result = append(result, &envoycore.HeaderValueOption{
				Header: &envoycore.HeaderValue{
					Key:   key,
					Value: value,
				},
				Append: BoolGogoToProto(option.GetAppend()),
			})
		}
		return result, nil
	default:
		return nil, errors.Errorf("Unexpected header option type %v", typedOption)
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
		HeaderOption: &envoycore_sk.HeaderValueOption_Header{
			Header: &envoycore_sk.HeaderValue{
				Key:   option.GetHeader().GetKey(),
				Value: option.GetHeader().GetValue(),
			},
		},
		Append: BoolProtoToGogo(option.GetAppend()),
	}
}

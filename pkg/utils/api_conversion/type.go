package api_conversion

import (
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_type_v3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	envoytype_gloo "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/type"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	envoycore_sk "github.com/solo-io/solo-kit/pkg/api/external/envoy/api/v2/core"
	"github.com/solo-io/solo-kit/pkg/errors"
)

type HeaderSecretOptions struct {
	EnforceNamespaceMatch bool
	UpstreamNamespace     string
}

const MatchingNamespaceEnv = "HEADER_SECRET_REF_NS_MATCHES_US"

// Converts between Envoy and Gloo/solokit versions of envoy protos
// This is required because go-control-plane dropped gogoproto in favor of goproto
// in v0.9.0, but solokit depends on gogoproto (and the generated deep equals it creates).
//
// we should work to remove that assumption from solokit and delete this code:
// https://github.com/solo-io/gloo/issues/1793

func ToGlooInt64RangeList(int64Range []*envoy_type_v3.Int64Range) []*envoytype_gloo.Int64Range {
	result := make([]*envoytype_gloo.Int64Range, len(int64Range))
	for i, v := range int64Range {
		result[i] = ToGlooInt64Range(v)
	}
	return result
}

func ToGlooInt64Range(int64Range *envoy_type_v3.Int64Range) *envoytype_gloo.Int64Range {
	return &envoytype_gloo.Int64Range{
		Start: int64Range.GetStart(),
		End:   int64Range.GetEnd(),
	}
}

func ToEnvoyInt64RangeList(int64Range []*envoytype_gloo.Int64Range) []*envoy_type_v3.Int64Range {
	result := make([]*envoy_type_v3.Int64Range, len(int64Range))
	for i, v := range int64Range {
		result[i] = ToEnvoyInt64Range(v)
	}
	return result
}

func ToEnvoyInt64Range(int64Range *envoytype_gloo.Int64Range) *envoy_type_v3.Int64Range {
	return &envoy_type_v3.Int64Range{
		Start: int64Range.GetStart(),
		End:   int64Range.GetEnd(),
	}
}

func ToEnvoyHeaderValueOptionList(option []*envoycore_sk.HeaderValueOption, secrets *v1.SecretList, secretOptions HeaderSecretOptions) ([]*envoy_config_core_v3.HeaderValueOption, error) {
	result := make([]*envoy_config_core_v3.HeaderValueOption, 0)
	var err error
	var opts []*envoy_config_core_v3.HeaderValueOption
	for _, v := range option {
		opts, err = ToEnvoyHeaderValueOptions(v, secrets, secretOptions)
		if err != nil {
			return nil, err
		}
		result = append(result, opts...)
	}
	return result, nil
}

func ToEnvoyHeaderValueOptions(option *envoycore_sk.HeaderValueOption, secrets *v1.SecretList, secretOptions HeaderSecretOptions) ([]*envoy_config_core_v3.HeaderValueOption, error) {
	switch typedOption := option.GetHeaderOption().(type) {
	case *envoycore_sk.HeaderValueOption_Header:
		return []*envoy_config_core_v3.HeaderValueOption{
			{
				Header: &envoy_config_core_v3.HeaderValue{
					Key:   typedOption.Header.GetKey(),
					Value: typedOption.Header.GetValue(),
				},
				Append: option.GetAppend(),
			},
		}, nil
	case *envoycore_sk.HeaderValueOption_HeaderSecretRef:
		if secretOptions.EnforceNamespaceMatch && secretOptions.UpstreamNamespace != typedOption.HeaderSecretRef.GetNamespace() {
			// The secrets sent to the upstream must come from the same namespace. The error message is copied from secretList.Find() to avoid exposing new information about this setting.
			return nil, errors.Errorf("list did not find secret %v.%v", typedOption.HeaderSecretRef.GetNamespace(), typedOption.HeaderSecretRef.GetName())
		}
		secret, err := secrets.Find(typedOption.HeaderSecretRef.GetNamespace(), typedOption.HeaderSecretRef.GetName())
		if err != nil {
			return nil, err
		}

		headerSecrets, ok := secret.GetKind().(*v1.Secret_Header)
		if !ok {
			return nil, errors.Errorf("Secret %v.%v was not a Header secret", typedOption.HeaderSecretRef.GetNamespace(), typedOption.HeaderSecretRef.GetName())
		}

		result := make([]*envoy_config_core_v3.HeaderValueOption, 0)
		for key, value := range headerSecrets.Header.GetHeaders() {
			result = append(result, &envoy_config_core_v3.HeaderValueOption{
				Header: &envoy_config_core_v3.HeaderValue{
					Key:   key,
					Value: value,
				},
				Append: option.GetAppend(),
			})
		}
		return result, nil
	default:
		return nil, errors.Errorf("Unexpected header option type %v", typedOption)
	}
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
		HeaderOption: &envoycore_sk.HeaderValueOption_Header{
			Header: &envoycore_sk.HeaderValue{
				Key:   option.GetHeader().GetKey(),
				Value: option.GetHeader().GetValue(),
			},
		},
		Append: option.GetAppend(),
	}
}

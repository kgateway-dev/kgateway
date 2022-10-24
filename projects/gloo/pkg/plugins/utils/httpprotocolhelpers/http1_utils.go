package httpprotocolhelpers

import (
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/protocol"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_extensions_http_header_formatters_preserve_case_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/http/header_formatters/preserve_case/v3"
)

const (
	PreserveCasePlugin = "envoy.http.stateful_header_formatters.preserve_case"
)

// ConvertHttp1 is a data marshalling function which converts hpo to an envoy-equivalent Http1ProtocolOptions
func ConvertHttp1(hpo protocol.Http1ProtocolOptions) (*envoy_config_core_v3.Http1ProtocolOptions, error) {
	out := &envoy_config_core_v3.Http1ProtocolOptions{}

	if hpo.GetEnableTrailers() {
		out.EnableTrailers = hpo.GetEnableTrailers()
	}

	out.OverrideStreamErrorOnInvalidHttpMessage = hpo.GetOverrideStreamErrorOnInvalidHttpMessage()

	if hpo.GetProperCaseHeaderKeyFormat() {
		out.HeaderKeyFormat = &envoy_config_core_v3.Http1ProtocolOptions_HeaderKeyFormat{
			HeaderFormat: &envoy_config_core_v3.Http1ProtocolOptions_HeaderKeyFormat_ProperCaseWords_{
				ProperCaseWords: &envoy_config_core_v3.Http1ProtocolOptions_HeaderKeyFormat_ProperCaseWords{},
			},
		}
	} else if hpo.GetPreserveCaseHeaderKeyFormat() {
		typedConfig, err := utils.MessageToAny(&envoy_extensions_http_header_formatters_preserve_case_v3.PreserveCaseFormatterConfig{})
		if err != nil {
			return nil, err
		}
		out.HeaderKeyFormat = &envoy_config_core_v3.Http1ProtocolOptions_HeaderKeyFormat{
			HeaderFormat: &envoy_config_core_v3.Http1ProtocolOptions_HeaderKeyFormat_StatefulFormatter{
				StatefulFormatter: &envoy_config_core_v3.TypedExtensionConfig{
					Name:        PreserveCasePlugin,
					TypedConfig: typedConfig,
				},
			},
		}
	}

	return out, nil
}

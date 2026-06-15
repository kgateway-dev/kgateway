package listenerpolicy

import (
	"fmt"

	envoyaccesslogv3 "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_hcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
)

// localReplyBodyOperator is the Envoy command operator that expands to the response body set on
// the mapper. Using it as the body format lets us set a Content-Type while returning the body verbatim.
const localReplyBodyOperator = "%LOCAL_REPLY_BODY%"

// convertLocalReplyConfig translates a LocalReplyConfig into its Envoy representation. Mappers match
// on the response status code and may override the status code, body and Content-Type.
func convertLocalReplyConfig(cfg *kgateway.LocalReplyConfig) (*envoy_hcm.LocalReplyConfig, error) {
	if cfg == nil || len(cfg.Mappers) == 0 {
		return nil, nil
	}

	mappers := make([]*envoy_hcm.ResponseMapper, 0, len(cfg.Mappers))
	for i := range cfg.Mappers {
		m := &cfg.Mappers[i]

		op, err := toEnvoyComparisonOpType(m.StatusCodeMatch.Op)
		if err != nil {
			return nil, fmt.Errorf("localReplyConfig.mappers[%d].statusCodeMatch.op: %w", i, err)
		}

		mapper := &envoy_hcm.ResponseMapper{
			Filter: &envoyaccesslogv3.AccessLogFilter{
				FilterSpecifier: &envoyaccesslogv3.AccessLogFilter_StatusCodeFilter{
					StatusCodeFilter: &envoyaccesslogv3.StatusCodeFilter{
						Comparison: &envoyaccesslogv3.ComparisonFilter{
							Op: op,
							Value: &envoycorev3.RuntimeUInt32{
								DefaultValue: uint32(m.StatusCodeMatch.Value), //nolint:gosec // G115: kubebuilder validation ensures 100 <= value <= 599, safe for uint32
							},
						},
					},
				},
			},
		}

		if m.StatusCode != nil {
			mapper.StatusCode = wrapperspb.UInt32(uint32(*m.StatusCode)) //nolint:gosec // G115: kubebuilder validation ensures 200 <= value <= 599, safe for uint32
		}

		if m.Body != nil {
			mapper.Body = &envoycorev3.DataSource{
				Specifier: &envoycorev3.DataSource_InlineString{
					InlineString: *m.Body,
				},
			}
		}

		// Setting a Content-Type requires a body_format_override in Envoy. We render the body
		// verbatim via the %LOCAL_REPLY_BODY% operator so the configured Content-Type is applied
		// without reinterpreting the body as a format string.
		if m.ContentType != nil {
			mapper.BodyFormatOverride = &envoycorev3.SubstitutionFormatString{
				Format: &envoycorev3.SubstitutionFormatString_TextFormatSource{
					TextFormatSource: &envoycorev3.DataSource{
						Specifier: &envoycorev3.DataSource_InlineString{
							InlineString: localReplyBodyOperator,
						},
					},
				},
				ContentType: *m.ContentType,
			}
		}

		mappers = append(mappers, mapper)
	}

	return &envoy_hcm.LocalReplyConfig{Mappers: mappers}, nil
}

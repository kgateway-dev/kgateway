package listenerpolicy

import (
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_hcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/extensions2/pluginutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func convertLocalReplyConfig(
	policy *kgateway.HTTPSettings,
	commoncol *collections.CommonCollections,
	krtctx krt.HandlerContext,
	parentSrc ir.ObjectSource,
) (*envoy_hcm.LocalReplyConfig, error) {
	config := policy.LocalReplies

	if config == nil {
		return nil, nil
	}

	envoyConfig := &envoy_hcm.LocalReplyConfig{}

	from := krtcollections.From{
		GroupKind: parentSrc.GetGroupKind(),
		Namespace: parentSrc.Namespace,
	}
	for _, mapper := range config.Mappers {
		envoyMapper, err := translateLocalReplyBodyMapper(krtctx, from, commoncol.Secrets, mapper)
		if err != nil {
			return nil, err
		}
		envoyConfig.Mappers = append(envoyConfig.Mappers, envoyMapper)
	}

	bodyFormat, err := pluginutils.EnvoyBodyFormat(config.DefaultBodyFormat)
	if err != nil {
		return nil, err
	}
	envoyConfig.BodyFormat = bodyFormat

	return envoyConfig, nil
}

func translateLocalReplyBodyMapper(krtctx krt.HandlerContext, from krtcollections.From, secrets *krtcollections.SecretIndex, mapper kgateway.LocalReplyMapper) (*envoy_hcm.ResponseMapper, error) {
	filter, err := convertAccessLogFilter(&mapper.Filter)
	if err != nil {
		return nil, err
	}
	envoyMapper := &envoy_hcm.ResponseMapper{
		Filter: filter,
	}

	if mapper.StatusCode != nil {
		envoyMapper.StatusCode = &wrapperspb.UInt32Value{
			Value: *mapper.StatusCode,
		}
	}

	if mapper.Body != nil {
		envoyMapper.Body = &envoycorev3.DataSource{
			Specifier: &envoycorev3.DataSource_InlineString{
				InlineString: *mapper.Body,
			},
		}
	}

	bodyFormatOverride, err := pluginutils.EnvoyBodyFormat(mapper.BodyFormatOverride)
	if err != nil {
		return nil, err
	}
	envoyMapper.BodyFormatOverride = bodyFormatOverride

	if mapper.Headers != nil {
		gwFilter, err := pluginutils.ConvertHeaderFilter(krtctx, from, secrets, mapper.Headers)
		if err != nil {
			return nil, err
		}
		options, err := pluginutils.ConvertMutationsToOptions(pluginutils.ConvertMutations(gwFilter))
		if err != nil {
			return nil, err
		}
		envoyMapper.HeadersToAdd = options
	}

	return envoyMapper, nil
}

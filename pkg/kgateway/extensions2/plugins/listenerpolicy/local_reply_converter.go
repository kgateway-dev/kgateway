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
) (*envoy_hcm.LocalReplyConfig, []string, error) {
	config := policy.LocalReplies

	if config == nil {
		return nil, nil, nil
	}

	envoyConfig := &envoy_hcm.LocalReplyConfig{}

	from := krtcollections.From{
		GroupKind: parentSrc.GetGroupKind(),
		Namespace: parentSrc.Namespace,
	}
	var dropped []string
	for _, mapper := range config.Mappers {
		envoyMapper, mapperDropped, err := translateLocalReplyBodyMapper(krtctx, from, commoncol.Secrets, mapper)
		if err != nil {
			return nil, nil, err
		}
		dropped = append(dropped, mapperDropped...)
		envoyConfig.Mappers = append(envoyConfig.Mappers, envoyMapper)
	}

	bodyFormat, err := pluginutils.EnvoyBodyFormat(config.DefaultBodyFormat)
	if err != nil {
		return nil, nil, err
	}
	envoyConfig.BodyFormat = bodyFormat

	return envoyConfig, dropped, nil
}

func translateLocalReplyBodyMapper(krtctx krt.HandlerContext, from krtcollections.From, secrets *krtcollections.SecretIndex, mapper kgateway.LocalReplyMapper) (*envoy_hcm.ResponseMapper, []string, error) {
	filter, err := convertAccessLogFilter(&mapper.Filter)
	if err != nil {
		return nil, nil, err
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
		return nil, nil, err
	}
	envoyMapper.BodyFormatOverride = bodyFormatOverride

	var dropped []string
	if mapper.Headers != nil {
		gwFilter, err := pluginutils.ConvertHeaderFilter(krtctx, from, secrets, mapper.Headers)
		if err != nil {
			return nil, nil, err
		}
		options, err := pluginutils.ConvertMutationsToOptions(pluginutils.ConvertMutations(gwFilter))
		if err != nil {
			return nil, nil, err
		}
		envoyMapper.HeadersToAdd = options
		dropped = pluginutils.RestrictedHeaderNames(gwFilter)
	}

	return envoyMapper, dropped, nil
}

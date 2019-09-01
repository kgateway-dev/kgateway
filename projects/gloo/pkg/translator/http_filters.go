package translator

import (
	"sort"

	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoylistener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	envoyutil "github.com/envoyproxy/go-control-plane/pkg/util"
	types "github.com/gogo/protobuf/types"
	"github.com/pkg/errors"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/go-utils/log"
)

const webSocketUpgradeType = "websocket"

func NewHttpConnectionManager(httpFilters []*envoyhttp.HttpFilter, rdsName string) *envoyhttp.HttpConnectionManager {
	return &envoyhttp.HttpConnectionManager{
		CodecType:  envoyhttp.AUTO,
		StatPrefix: "http",
		NormalizePath: &types.BoolValue{
			Value: true,
		},
		UpgradeConfigs: []*envoyhttp.HttpConnectionManager_UpgradeConfig{{
			UpgradeType: webSocketUpgradeType,
		}},
		RouteSpecifier: &envoyhttp.HttpConnectionManager_Rds{
			Rds: &envoyhttp.Rds{
				ConfigSource: envoycore.ConfigSource{
					ConfigSourceSpecifier: &envoycore.ConfigSource_Ads{
						Ads: &envoycore.AggregatedConfigSource{},
					},
				},
				RouteConfigName: rdsName,
			},
		},
		HttpFilters: httpFilters,
	}
}

func (t *translator) computeHttpConnectionManagerFilter(params plugins.Params, listener *v1.HttpListener, rdsName string, report reportFunc) envoylistener.Filter {
	httpFilters := t.computeHttpFilters(params, listener, report)
	params.Ctx = contextutils.WithLogger(params.Ctx, "compute_http_connection_manager")

	httpConnMgr := NewHttpConnectionManager(httpFilters, rdsName)

	hcmFilter, err := NewFilterWithConfig(envoyutil.HTTPConnectionManager, httpConnMgr)
	if err != nil {
		panic(errors.Wrap(err, "failed to convert proto message to struct"))
	}
	return hcmFilter
}

func (t *translator) computeHttpFilters(params plugins.Params, listener *v1.HttpListener, report reportFunc) []*envoyhttp.HttpFilter {
	var httpFilters []plugins.StagedHttpFilter
	// run the Http Filter Plugins
	for _, plug := range t.plugins {
		filterPlugin, ok := plug.(plugins.HttpFilterPlugin)
		if !ok {
			continue
		}
		stagedFilters, err := filterPlugin.HttpFilters(params, listener)
		if err != nil {
			report(err, "invalid http listener")
		}
		for _, httpFilter := range stagedFilters {
			if httpFilter.HttpFilter == nil {
				log.Warnf("plugin implements HttpFilters() but returned nil")
				continue
			}
			httpFilters = append(httpFilters, httpFilter)
		}
	}

	// sort filters by stage
	envoyHttpFilters := sortFilters(httpFilters)
	envoyHttpFilters = append(envoyHttpFilters, &envoyhttp.HttpFilter{Name: envoyutil.Router})
	return envoyHttpFilters
}

func sortFilters(filters plugins.StagedHttpFilterList) []*envoyhttp.HttpFilter {
	sort.Sort(filters)
	var sortedFilters []*envoyhttp.HttpFilter
	for _, filter := range filters {
		sortedFilters = append(sortedFilters, filter.HttpFilter)
	}
	return sortedFilters
}

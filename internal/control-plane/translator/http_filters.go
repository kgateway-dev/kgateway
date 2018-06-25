package translator

import (
	"sort"

	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoylistener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	envoyutil "github.com/envoyproxy/go-control-plane/pkg/util"
	"github.com/pkg/errors"

	"github.com/solo-io/gloo/pkg/log"
	"github.com/solo-io/gloo/pkg/plugins"
)

func (t *Translator) computeHttpConnectionManager(rdsName string) envoylistener.Filter {
	httpConnMgr := &envoyhttp.HttpConnectionManager{
		CodecType:  envoyhttp.AUTO,
		StatPrefix: "http",
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
		HttpFilters: t.computeHttpFilters(),
	}

	httpConnMgrCfg, err := envoyutil.MessageToStruct(httpConnMgr)
	if err != nil {
		panic(errors.Wrap(err, "failed to convert proto message to struct"))
	}
	return envoylistener.Filter{
		Name:   connMgrFilter,
		Config: httpConnMgrCfg,
	}
}

func (t *Translator) computeHttpFilters() []*envoyhttp.HttpFilter {
	var httpFilters []plugins.StagedHttpFilter
	for _, plug := range t.plugins {
		filterPlugin, ok := plug.(plugins.HttpFilterPlugin)
		if !ok {
			continue
		}
		params := &plugins.HttpFilterPluginParams{}
		stagedFilters := filterPlugin.HttpFilters(params)
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
	envoyHttpFilters = append(envoyHttpFilters, &envoyhttp.HttpFilter{Name: routerFilter})
	return envoyHttpFilters
}

func sortFilters(filters []plugins.StagedHttpFilter) []*envoyhttp.HttpFilter {
	// sort them first by stage, then by name.
	less := func(i, j int) bool {
		filteri := filters[i]
		filterj := filters[j]
		if filteri.Stage != filterj.Stage {
			return filteri.Stage < filterj.Stage
		}
		return filteri.HttpFilter.Name < filterj.HttpFilter.Name
	}
	sort.SliceStable(filters, less)

	var sortedFilters []*envoyhttp.HttpFilter
	for _, filter := range filters {
		sortedFilters = append(sortedFilters, filter.HttpFilter)
	}

	return sortedFilters
}

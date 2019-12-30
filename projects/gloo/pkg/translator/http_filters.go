package translator

import (
	"sort"

	"github.com/golang/protobuf/ptypes/wrappers"
	validationapi "github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils/validation"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/util"

	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoylistener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	"github.com/pkg/errors"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/go-utils/log"
)

const (
	DefaultHttpStatPrefix = "http"
)

func NewHttpConnectionManager(listener *v1.HttpListener, httpFilters []*envoyhttp.HttpFilter, rdsName string) *envoyhttp.HttpConnectionManager {
	statPrefix := listener.GetStatPrefix()
	if statPrefix == "" {
		statPrefix = DefaultHttpStatPrefix
	}
	return &envoyhttp.HttpConnectionManager{
		CodecType:  envoyhttp.HttpConnectionManager_AUTO,
		StatPrefix: statPrefix,
		NormalizePath: &wrappers.BoolValue{
			Value: true,
		},
		RouteSpecifier: &envoyhttp.HttpConnectionManager_Rds{
			Rds: &envoyhttp.Rds{
				ConfigSource: &envoycore.ConfigSource{
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

func (t *translatorInstance) computeHttpConnectionManagerFilter(params plugins.Params, listener *v1.HttpListener, rdsName string, httpListenerReport *validationapi.HttpListenerReport) *envoylistener.Filter {
	httpFilters := t.computeHttpFilters(params, listener, httpListenerReport)
	params.Ctx = contextutils.WithLogger(params.Ctx, "compute_http_connection_manager")

	httpConnMgr := NewHttpConnectionManager(listener, httpFilters, rdsName)

	hcmFilter, err := NewFilterWithConfig(util.HTTPConnectionManager, httpConnMgr)
	if err != nil {
		panic(errors.Wrap(err, "failed to convert proto message to struct"))
	}
	return hcmFilter
}

func (t *translatorInstance) computeHttpFilters(params plugins.Params, listener *v1.HttpListener, httpListenerReport *validationapi.HttpListenerReport) []*envoyhttp.HttpFilter {
	var httpFilters []plugins.StagedHttpFilter
	// run the Http Filter Plugins
	for _, plug := range t.plugins {
		filterPlugin, ok := plug.(plugins.HttpFilterPlugin)
		if !ok {
			continue
		}
		stagedFilters, err := filterPlugin.HttpFilters(params, listener)
		if err != nil {
			validation.AppendHTTPListenerError(httpListenerReport, validationapi.HttpListenerReport_Error_ProcessingError, err.Error())
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
	envoyHttpFilters = append(envoyHttpFilters, &envoyhttp.HttpFilter{Name: util.Router})
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

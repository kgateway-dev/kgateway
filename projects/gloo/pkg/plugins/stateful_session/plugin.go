package stateful_session

import (
	"fmt"

	envoyv3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	statefulsessionv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/stateful_session/v3"
	cookiev3 "github.com/envoyproxy/go-control-plane/envoy/extensions/http/stateful_session/cookie/v3"
	httpv3 "github.com/envoyproxy/go-control-plane/envoy/type/http/v3"
	"github.com/golang/protobuf/ptypes/duration"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
)

var (
	_ plugins.Plugin           = new(plugin)
	_ plugins.HttpFilterPlugin = new(plugin)
)

const (
	ExtensionName = "envoy.filters.http.stateful_session"
)

var pluginStage = plugins.DuringStage(plugins.RouteStage)

type plugin struct {
	removeUnused bool
}

func (p *plugin) Name() string {
	return ExtensionName
}

func (p *plugin) Init(params plugins.InitParams) {
	fmt.Printf("\n*************\nstateful session Init settings: %v\n", params.Settings)
	p.removeUnused = params.Settings.GetGloo().GetRemoveUnusedFilters().GetValue()
}

func (p *plugin) HttpFilters(params plugins.Params, listener *gloov1.HttpListener) ([]plugins.StagedHttpFilter, error) {
	fmt.Printf("\n*************\nstateful session HttpFilters\n")
	//fmt.Printf("\n*************\nparams.Snapshot: %v\n", params.Snapshot)
	config := &cookiev3.CookieBasedSessionState{
		Cookie: &httpv3.Cookie{
			Name: "stateful_session_cookie",
			// Path: "/not_the_default_path",
			Ttl: &duration.Duration{
				Seconds: 3600,
			},
		},
	}

	marshalledConf, err := utils.MessageToAny(config)
	if err != nil {
		return nil, err
	}

	fmt.Printf("\n*************\nmarshalledConf: %v\n", marshalledConf)
	return []plugins.StagedHttpFilter{plugins.MustNewStagedFilter(
		ExtensionName,
		&statefulsessionv3.StatefulSession{
			SessionState: &envoyv3.TypedExtensionConfig{
				Name:        "envoy.http.stateful_session.cookie",
				TypedConfig: marshalledConf,
			},
		},
		pluginStage,
	)}, nil

	//return []plugins.StagedHttpFilter{}, nil
}

func NewPlugin() *plugin {
	return &plugin{}
}

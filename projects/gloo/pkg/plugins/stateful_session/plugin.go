package stateful_session

import (
	"fmt"

	v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	stateful_session "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/stateful_session/v3"
	cookiev3 "github.com/envoyproxy/go-control-plane/envoy/extensions/http/stateful_session/cookie/v3"
	httpv3 "github.com/envoyproxy/go-control-plane/envoy/type/http/v3"
	"github.com/golang/protobuf/ptypes/duration"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
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

type plugin struct{}

func (p *plugin) Name() string {
	return ExtensionName
}

func (p *plugin) Init(params plugins.InitParams) {

}

func (p *plugin) HttpFilters(params plugins.Params, listener *v1.HttpListener) ([]plugins.StagedHttpFilter, error) {
	fmt.Printf("\n*************\nstateful session HttpFilters\n")
	fmt.Printf("\n*************\nparams: %v\n", params)
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
		&stateful_session.StatefulSession{
			SessionState: &v3.TypedExtensionConfig{
				Name:        "envoy.http.stateful_session.cookie",
				TypedConfig: marshalledConf,
			},
		},
		pluginStage,
	)}, nil
}

func NewPlugin() *plugin {
	return &plugin{}
}

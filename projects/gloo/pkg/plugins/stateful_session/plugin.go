package stateful_session

import (
	"fmt"

	envoyv3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	statefulsessionv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/stateful_session/v3"
	cookiev3 "github.com/envoyproxy/go-control-plane/envoy/extensions/http/stateful_session/cookie/v3"
	httpv3 "github.com/envoyproxy/go-control-plane/envoy/type/http/v3"
	"github.com/golang/protobuf/proto"
	"github.com/rotisserie/eris"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/statefulsession"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
)

var (
	_ plugins.Plugin           = new(plugin)
	_ plugins.HttpFilterPlugin = new(plugin)
)

const (
	ExtensionName       = "envoy.filters.http.stateful_session"
	ExtensionTypeCookie = "envoy.http.stateful_session.cookie"
)

var (
	pluginStage            = plugins.DuringStage(plugins.RouteStage)
	ErrNoCookie            = eris.Errorf("cookie must be provided")
	ErrNoCookieName        = eris.Errorf("cookie name must be provided")
	ErrNoCookieBasedConfig = eris.Errorf("cookiesBasedConfig must be provided")
)

type plugin struct {
	removeUnused bool
}

func (p *plugin) Name() string {
	return ExtensionName
}

func (p *plugin) Init(params plugins.InitParams) {
	p.removeUnused = params.Settings.GetGloo().GetRemoveUnusedFilters().GetValue()
}

func (p *plugin) HttpFilters(params plugins.Params, listener *gloov1.HttpListener) ([]plugins.StagedHttpFilter, error) {
	sessionConf := listener.GetOptions().GetStatefulSession()

	if sessionConf == nil {
		return []plugins.StagedHttpFilter{}, nil
	}

	var config proto.Message
	var err error
	switch conf := sessionConf.GetSessionState().(type) {
	case *statefulsession.StatefulSession_CookieBased:
		config, err = translateCookieBased(conf)
		if err != nil {
			return nil, err

		}
	}

	marshalledConf, err := utils.MessageToAny(config)
	if err != nil {
		return nil, err
	}

	return []plugins.StagedHttpFilter{plugins.MustNewStagedFilter(
		ExtensionName,
		&statefulsessionv3.StatefulSession{
			SessionState: &envoyv3.TypedExtensionConfig{
				Name:        ExtensionTypeCookie,
				TypedConfig: marshalledConf,
			},
			Strict: sessionConf.GetStrict(),
		},
		pluginStage,
	)}, nil

}

func translateCookieBased(conf *statefulsession.StatefulSession_CookieBased) (*cookiev3.CookieBasedSessionState, error) {
	if conf.CookieBased == nil {
		return nil, ErrNoCookieBasedConfig
	}

	fmt.Printf("cookie: %v\n", conf.CookieBased.GetCookie())
	if conf.CookieBased.GetCookie() == nil {
		return nil, ErrNoCookie
	}

	if conf.CookieBased.GetCookie().GetName() == "" {
		return nil, ErrNoCookieName
	}

	cookieName := conf.CookieBased.GetCookie().GetName()

	cookiePath := conf.CookieBased.GetCookie().GetPath() // (for now) pass through empty string to use Envoy default
	ttl := conf.CookieBased.GetCookie().GetTtl()

	return &cookiev3.CookieBasedSessionState{
		Cookie: &httpv3.Cookie{
			Name: cookieName,
			Path: cookiePath,
			Ttl:  ttl,
		},
	}, nil
}

func NewPlugin() *plugin {
	return &plugin{}
}

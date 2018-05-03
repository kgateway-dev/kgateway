package projectfn

import (
	"net/url"
	"path"
	"strings"

	"github.com/pkg/errors"

	"github.com/go-openapi/runtime"

	"github.com/solo-io/gloo/internal/function-discovery/resolver"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/plugins/kubernetes"
	"github.com/solo-io/gloo/pkg/plugins/rest"

	"github.com/solo-io/gloo/internal/function-discovery/updater/projectfn/imported/client"
	"github.com/solo-io/gloo/internal/function-discovery/updater/projectfn/imported/client/apps"
	"github.com/solo-io/gloo/internal/function-discovery/updater/projectfn/imported/client/routes"
	"github.com/solo-io/gloo/internal/function-discovery/updater/projectfn/imported/models"
)

const routePrefix = "/r/"

func IsFnUpstream(us *v1.Upstream) bool {

	if us.Type != kubernetes.UpstreamTypeKube {
		return false
	}

	spec, err := kubernetes.DecodeUpstreamSpec(us.Spec)
	if err != nil {
		return false
	}

	return strings.HasSuffix(spec.ServiceName, "-fn-api")
}

func GetFuncs(resolve resolver.Resolver, us *v1.Upstream) ([]*v1.Function, error) {
	return GetFuncsWithTransport(resolve, us, nil)
}

func GetFuncsWithTransport(resolve resolver.Resolver, us *v1.Upstream, transport runtime.ClientTransport) ([]*v1.Function, error) {

	addr, err := resolve.Resolve(us)
	if err != nil {
		return nil, errors.Wrap(err, "error getting fn service")
	}

	fr, err := NewFnRetreiver("http://" + addr + "/v1")
	if err != nil {
		return nil, err
	}
	if transport != nil {
		fr.SetTransport(transport)
	}
	funcs, err := fr.GetFuncs()
	if err != nil {
		return nil, err
	}
	var finalizedroutes []*v1.Function
	for _, funk := range funcs {
		finalizedroutes = append(finalizedroutes, createFunction(funk))
	}
	return finalizedroutes, nil
}

func (fr *FnRetreiver) GetFuncs() ([]Function, error) {
	appnames, err := fr.getAppNames()
	if err != nil {
		return nil, err
	}
	var finalizedroutes []Function
	for _, name := range appnames {

		routes, err := fr.getRoutesForApp(name)
		if err != nil {
			return nil, err
		}

		finalizedroutes = append(finalizedroutes, fr.getFnApiPaths(name, routes)...)
	}

	return finalizedroutes, nil
}

type FnRetreiver struct {
	client *client.Fn
}

func createFunction(funk Function) *v1.Function {
	headersTemplate := map[string]string{":method": "POST"}

	return &v1.Function{
		Name: funk.Appname +":" +funk.Funcname,
		Spec: rest.EncodeFunctionSpec(rest.Template{
			Path:            funk.Route,
			Header:          headersTemplate,
			PassthroughBody: true,
		}),
	}
}

func NewFnRetreiver(serverurl string) (*FnRetreiver, error) {
	url, err := url.Parse(serverurl)
	if err != nil {
		return nil, err
	}

	cfg := &client.TransportConfig{
		BasePath: url.Path,
		Host:     url.Host,
		Schemes:  []string{url.Scheme},
	}
	client := client.NewHTTPClientWithConfig(nil, cfg)

	return &FnRetreiver{
		client: client,
	}, nil
}

func (fr *FnRetreiver) SetTransport(transport runtime.ClientTransport) {
	fr.client.SetTransport(transport)
}

func (fr *FnRetreiver) getAppNames() ([]string, error) {

	var appnames []string
	cursor := ""
	for {
		params := apps.NewGetAppsParams()
		if cursor != "" {
			params.Cursor = &cursor
		}
		apppresp, err := fr.client.Apps.GetApps(params)
		if err != nil {
			return nil, err
		}
		appnames = append(appnames, getnames(apppresp)...)

		cursor = apppresp.Payload.NextCursor
		if cursor == "" {
			return appnames, nil
		}
	}
}

func (fr *FnRetreiver) getRoutesForApp(app string) ([]*models.Route, error) {

	var routeobjs []*models.Route

	cursor := ""
	for {
		routeParams :=routes.NewGetAppsAppRoutesParams()
		
		routeParams.App = app
		if cursor != "" {
			routeParams.Cursor = &cursor
		}
		routeresp, err := fr.client.Routes.GetAppsAppRoutes(routeParams)
		if err != nil {
			return nil, err
		}
		routeobjs = append(routeobjs, routeresp.Payload.Routes...)

		cursor = routeresp.Payload.NextCursor
		if cursor == "" {
			return routeobjs, nil
		}
	}
}

func (fr *FnRetreiver) getFnApiPaths(app string, routeobjs []*models.Route) []Function {

	var finalizedroutes []Function

	for _, routeobj := range routeobjs {
		finalizedroutes = append(finalizedroutes, Function{
			Appname: app, 
			Funcname: strings.Replace(routeobj.Path,"/","",-1),
			Route: path.Join(routePrefix, app, routeobj.Path)})
	}
	return finalizedroutes
}

func getnames(resp *apps.GetAppsOK) []string {

	var appnames []string
	for _, app := range resp.Payload.Apps {
		appnames = append(appnames, app.Name)
	}
	return appnames
}

type Function struct {
	Appname  string
	Funcname string
	Route    string
}

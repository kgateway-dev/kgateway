package headers

import (
	"os"
	"strings"

	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/pluginutils"
	"github.com/solo-io/gloo/projects/gloo/pkg/upstreams"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/pkg/utils/api_conversion"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	v1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/headers"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

var (
	_ plugins.RoutePlugin               = new(plugin)
	_ plugins.VirtualHostPlugin         = new(plugin)
	_ plugins.WeightedDestinationPlugin = new(plugin)
)

const (
	ExtensionName = "headers"
)

var (
	MissingHeaderValueError = eris.New("header section of header value option cannot be nil")
	CantSetHostHeaderError  = eris.New("cannot set Host header in response headers")

	CantSetPseudoHeaderError = func(header string) error {
		return eris.Errorf(":-prefixed headers cannot be set: '%s'", header)
	}
)

// Puts Header Manipulation config on Routes, VirtualHosts, and Weighted Clusters
type plugin struct {
	enforceMatchingNamespaces bool
}

func NewPlugin() *plugin {
	return &plugin{
		enforceMatchingNamespaces: strings.ToLower(os.Getenv(api_conversion.MatchingNamespaceEnv)) == "true",
	}
}

func (p *plugin) Name() string {
	return ExtensionName
}

func (p *plugin) Init(_ plugins.InitParams) {}

func (p *plugin) ProcessWeightedDestination(
	params plugins.RouteParams,
	in *v1.WeightedDestination,
	out *envoy_config_route_v3.WeightedCluster_ClusterWeight,
) error {
	headerManipulation := in.GetOptions().GetHeaderManipulation()
	if headerManipulation == nil {
		return nil
	}
	upstreamNamespace := "sometenant"
	if p.enforceMatchingNamespaces {
		us, err := upstreams.DestinationToUpstreamRef(in.GetDestination())
		if err == nil {
			upstreamNamespace = us.GetNamespace()
		}
	}
	headerSecretOptions := api_conversion.HeaderSecretOptions{EnforceNamespaceMatch: p.enforceMatchingNamespaces, UpstreamNamespace: upstreamNamespace}
	envoyHeader, err := convertHeaderConfig(headerManipulation, getSecretsFromSnapshot(params.Snapshot), headerSecretOptions)
	if err != nil {
		return err
	}

	out.RequestHeadersToAdd = envoyHeader.RequestHeadersToAdd
	out.RequestHeadersToRemove = envoyHeader.RequestHeadersToRemove
	out.ResponseHeadersToAdd = envoyHeader.ResponseHeadersToAdd
	out.ResponseHeadersToRemove = envoyHeader.ResponseHeadersToRemove

	return nil
}

func (p *plugin) ProcessVirtualHost(
	params plugins.VirtualHostParams,
	in *v1.VirtualHost,
	out *envoy_config_route_v3.VirtualHost,
) error {
	headerManipulation := in.GetOptions().GetHeaderManipulation()

	if headerManipulation == nil {
		return nil
	}
	headerSecretOptions := api_conversion.HeaderSecretOptions{
		EnforceNamespaceMatch: p.enforceMatchingNamespaces,
	}
	if p.enforceMatchingNamespaces && len(in.GetRoutes()) > 0 {
		usNamespace := getUpstreamNamespaceForRouteAction(params.Snapshot, in.GetRoutes()[0].GetRouteAction())
		if len(in.GetRoutes()) > 1 {
			for _, r := range in.GetRoutes()[1:] {
				// in order for the namespace match check to make sense, all the upstreams on the virtual host need to be the same
				if getUpstreamNamespaceForRouteAction(params.Snapshot, r.GetRouteAction()) != usNamespace {
					usNamespace = ""
					break
				}
			}
		}
		headerSecretOptions.UpstreamNamespace = usNamespace
	}
	envoyHeader, err := convertHeaderConfig(headerManipulation, getSecretsFromSnapshot(params.Snapshot), headerSecretOptions)
	if err != nil {
		return err
	}

	out.RequestHeadersToAdd = envoyHeader.RequestHeadersToAdd
	out.RequestHeadersToRemove = envoyHeader.RequestHeadersToRemove
	out.ResponseHeadersToAdd = envoyHeader.ResponseHeadersToAdd
	out.ResponseHeadersToRemove = envoyHeader.ResponseHeadersToRemove

	return nil
}

func (p *plugin) ProcessRoute(params plugins.RouteParams, in *v1.Route, out *envoy_config_route_v3.Route) error {
	headerManipulation := in.GetOptions().GetHeaderManipulation()

	if headerManipulation == nil {
		return nil
	}

	headerSecretOptions := api_conversion.HeaderSecretOptions{EnforceNamespaceMatch: p.enforceMatchingNamespaces}
	// If we need to enforce that the namespace of the secret and upstream match, then find the upstream namespace.
	if p.enforceMatchingNamespaces {
		headerSecretOptions.UpstreamNamespace = getUpstreamNamespaceForRouteAction(params.Snapshot, in.GetRouteAction())
	}
	envoyHeader, err := convertHeaderConfig(headerManipulation, getSecretsFromSnapshot(params.Snapshot), headerSecretOptions)
	if err != nil {
		return err
	}

	out.RequestHeadersToAdd = envoyHeader.RequestHeadersToAdd
	out.RequestHeadersToRemove = envoyHeader.RequestHeadersToRemove
	out.ResponseHeadersToAdd = envoyHeader.ResponseHeadersToAdd
	out.ResponseHeadersToRemove = envoyHeader.ResponseHeadersToRemove

	return nil
}

type envoyHeaderManipulation struct {
	RequestHeadersToAdd     []*envoy_config_core_v3.HeaderValueOption
	RequestHeadersToRemove  []string
	ResponseHeadersToAdd    []*envoy_config_core_v3.HeaderValueOption
	ResponseHeadersToRemove []string
}

func getUpstreamNamespaceForRouteAction(snapshot *v1snap.ApiSnapshot, action *v1.RouteAction) string {
	usRefs, err := pluginutils.DestinationUpstreams(snapshot, action)
	if err != nil || len(usRefs) == 0 {
		return ""
	}
	ns := usRefs[0].GetNamespace()
	// verify that all the upstreams in the list are in the same namespace
	// if not, we can't do a meaningful check for matching namespaces, so we will fail if headerSecretRef is set
	// otherwise that's weird but fine
	if len(usRefs) > 1 {
		for _, u := range usRefs[1:] {
			if u.GetNamespace() != ns {
				return ""
			}
		}
	}
	return ns
}
func getSecretsFromSnapshot(snapshot *v1snap.ApiSnapshot) *v1.SecretList {
	var secrets *v1.SecretList
	if snapshot == nil {
		secrets = &v1.SecretList{}
	} else {
		secrets = &snapshot.Secrets
	}
	return secrets
}

func convertHeaderConfig(in *headers.HeaderManipulation, secrets *v1.SecretList, secretOptions api_conversion.HeaderSecretOptions) (*envoyHeaderManipulation, error) {
	// request headers can either be made from a normal key/value pair, or.
	// they can be constructed from a supplied secret. To accomplish this, we use
	// a utility function that was originally created to accomplish this for health check headers.
	requestAdd, err := api_conversion.ToEnvoyHeaderValueOptionList(in.GetRequestHeadersToAdd(), secrets, secretOptions)
	if err != nil {
		return nil, err
	}
	// response headers have no reason to include secrets.
	responseAdd, err := convertResponseHeaderValueOption(in.GetResponseHeadersToAdd())
	if err != nil {
		return nil, err
	}

	return &envoyHeaderManipulation{
		RequestHeadersToAdd:     requestAdd,
		RequestHeadersToRemove:  in.GetRequestHeadersToRemove(),
		ResponseHeadersToAdd:    responseAdd,
		ResponseHeadersToRemove: in.GetResponseHeadersToRemove(),
	}, nil
}

func convertResponseHeaderValueOption(
	in []*headers.HeaderValueOption,
) ([]*envoy_config_core_v3.HeaderValueOption, error) {
	var out []*envoy_config_core_v3.HeaderValueOption
	for _, h := range in {
		header := h.GetHeader()
		if header == nil {
			return nil, MissingHeaderValueError
		}

		if strings.HasPrefix(header.GetKey(), ":") {
			return nil, CantSetPseudoHeaderError(header.GetKey())
		}

		if strings.EqualFold(header.GetKey(), "Host") {
			return nil, CantSetHostHeaderError
		}

		out = append(out, &envoy_config_core_v3.HeaderValueOption{
			Header: &envoy_config_core_v3.HeaderValue{
				Key:   header.GetKey(),
				Value: header.GetValue(),
			},
			Append: h.GetAppend(),
		})
	}
	return out, nil
}

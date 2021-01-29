package extauth

import (
	"strings"
	"time"

	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/solo-io/solo-kit/pkg/utils/prototime"

	envoycore "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyauth "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"
	envoymatcher "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	envoytype "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	extauthv1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var (
	DefaultTimeout = prototime.DurationToProto(200 * time.Millisecond)
	NoServerRefErr = eris.New("no extauth server reference configured")
	ServerNotFound = func(usRef *core.ResourceRef) error {
		return eris.Errorf("extauth server upstream not found %s", usRef.String())
	}
	InvalidStatusOnErrorErr = func(code uint32) error {
		return eris.Errorf("invalid statusOnError code: %d", code)
	}
)

const JWTFilterName = "envoy.filters.http.jwt_authn"

func BuildHttpFilters(
	globalSettings *extauthv1.Settings,
	listener *v1.HttpListener,
	upstreams v1.UpstreamList,
) ([]plugins.StagedHttpFilter, error) {
	var filters []plugins.StagedHttpFilter

	// If no extauth settings are provided, don't configure the ext_authz filter
	settings := listener.GetOptions().GetExtauth()
	if settings == nil {
		settings = globalSettings
	}
	if settings == nil {
		return filters, nil
	}

	upstreamRef := settings.GetExtauthzServerRef()
	if upstreamRef == nil {
		return nil, NoServerRefErr
	}

	// Make sure the server exists
	_, err := upstreams.Find(upstreamRef.Namespace, upstreamRef.Name)
	if err != nil {
		return nil, ServerNotFound(upstreamRef)
	}

	extAuthCfg, err := generateEnvoyConfigForFilter(settings, upstreamRef)
	if err != nil {
		return nil, err
	}

	stagedFilter, err := plugins.NewStagedFilterWithConfig(wellknown.HTTPExternalAuthorization, extAuthCfg, FilterStage)
	if err != nil {
		return nil, err
	}
	filters = append(filters, stagedFilter)
	return filters, nil
}

func generateEnvoyConfigForFilter(settings *extauthv1.Settings, extauthUpstreamRef *core.ResourceRef) (*envoyauth.ExtAuthz, error) {
	cfg := &envoyauth.ExtAuthz{
		MetadataContextNamespaces: []string{JWTFilterName},
	}
	httpService := settings.GetHttpService()
	if httpService == nil {
		svc := &envoycore.GrpcService{
			TargetSpecifier: &envoycore.GrpcService_EnvoyGrpc_{
				EnvoyGrpc: &envoycore.GrpcService_EnvoyGrpc{
					ClusterName: translator.UpstreamToClusterName(extauthUpstreamRef),
				},
			}}

		timeout := settings.GetRequestTimeout()
		if timeout == nil {
			timeout = DefaultTimeout
		}
		svc.Timeout = timeout

		cfg.Services = &envoyauth.ExtAuthz_GrpcService{
			GrpcService: svc,
		}
	} else {
		httpURI := &envoycore.HttpUri{
			// This uri is not used by the filter but is required because of envoy validation.
			Uri:     HttpServerUri,
			Timeout: settings.GetRequestTimeout(),
			HttpUpstreamType: &envoycore.HttpUri_Cluster{
				Cluster: translator.UpstreamToClusterName(extauthUpstreamRef),
			},
		}
		if httpURI.Timeout == nil {
			// Set to the default. This is required by envoy validation.
			httpURI.Timeout = DefaultTimeout
		}

		cfg.Services = &envoyauth.ExtAuthz_HttpService{
			HttpService: &envoyauth.HttpService{
				ServerUri: httpURI,
				// Trim suffix, as request path always starts with /, and we want to avoid a double /
				PathPrefix:            strings.TrimSuffix(httpService.PathPrefix, "/"),
				AuthorizationRequest:  translateRequest(httpService.Request),
				AuthorizationResponse: translateResponse(httpService.Response),
			},
		}
	}

	cfg.FailureModeAllow = settings.FailureModeAllow
	cfg.WithRequestBody = translateRequestBody(settings.RequestBody)
	cfg.ClearRouteCache = settings.ClearRouteCache

	statusOnError, err := translateStatusOnError(settings.StatusOnError)
	if err != nil {
		return nil, err
	}
	cfg.StatusOnError = statusOnError

	// If not set, `TransportApiVersion` defaults to AUTO (which defaults to V2).
	// Both the AUTO and V2 values are [currently deprecated](https://github.com/envoyproxy/envoy/blob/main/api/envoy/config/core/v3/config_source.proto#L33).
	// These fields will be removed in Envoy [by end of Q1 2021](https://www.envoyproxy.io/docs/envoy/latest/faq/api/envoy_v2_support),
	// when V3 will become the default.
	switch settings.GetTransportApiVersion() {
	case extauthv1.Settings_V3:
		cfg.TransportApiVersion = envoycore.ApiVersion_V3
	default:
		// Leave unset so it defaults to AUTO
	}

	return cfg, nil
}

func translateRequest(in *extauthv1.HttpService_Request) *envoyauth.AuthorizationRequest {
	if in == nil {
		return nil
	}

	return &envoyauth.AuthorizationRequest{
		AllowedHeaders: translateListMatcher(in.AllowedHeaders),
		HeadersToAdd:   convertHeadersToAdd(in.HeadersToAdd),
	}
}

func translateResponse(in *extauthv1.HttpService_Response) *envoyauth.AuthorizationResponse {
	if in == nil {
		return nil
	}

	return &envoyauth.AuthorizationResponse{
		AllowedUpstreamHeaders: translateListMatcher(in.AllowedUpstreamHeaders),
		AllowedClientHeaders:   translateListMatcher(in.AllowedClientHeaders),
	}
}

func translateRequestBody(in *extauthv1.BufferSettings) *envoyauth.BufferSettings {
	if in == nil {
		return nil
	}
	maxBytes := in.MaxRequestBytes
	if maxBytes <= 0 {
		maxBytes = 4 * 1024
	}
	return &envoyauth.BufferSettings{
		AllowPartialMessage: in.AllowPartialMessage,
		MaxRequestBytes:     maxBytes,
		PackAsBytes:         in.PackAsBytes,
	}
}

func translateStatusOnError(statusOnError uint32) (*envoytype.HttpStatus, error) {
	if statusOnError == 0 {
		return nil, nil
	}

	// make sure it is allowed:
	if _, ok := envoytype.StatusCode_name[int32(statusOnError)]; !ok {
		return nil, InvalidStatusOnErrorErr(statusOnError)
	}

	return &envoytype.HttpStatus{Code: envoytype.StatusCode(int32(statusOnError))}, nil
}

func translateListMatcher(in []string) *envoymatcher.ListStringMatcher {
	if len(in) == 0 {
		return nil
	}
	var lsm envoymatcher.ListStringMatcher

	for _, pattern := range in {
		lsm.Patterns = append(lsm.Patterns, &envoymatcher.StringMatcher{
			MatchPattern: &envoymatcher.StringMatcher_Exact{
				Exact: pattern,
			},
		})
	}

	return &lsm
}

func convertHeadersToAdd(headersToAddMap map[string]string) []*envoycore.HeaderValue {
	var headersToAdd []*envoycore.HeaderValue
	for k, v := range headersToAddMap {
		headersToAdd = append(headersToAdd, &envoycore.HeaderValue{
			Key:   k,
			Value: v,
		})
	}
	return headersToAdd
}

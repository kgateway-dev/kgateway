package irtranslator

import (
	"context"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_upstreams_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"istio.io/istio/pkg/kube/krt"

	extensionsplug "github.com/solo-io/gloo/projects/gateway2/extensions2/plugin"
	"github.com/solo-io/gloo/projects/gateway2/ir"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type UpstreamTranslator struct {
	ContributedUpstreams map[schema.GroupKind]ir.UpstreamInit
	ContributedPolicies  map[schema.GroupKind]extensionsplug.PolicyPlugin
}

func (t *UpstreamTranslator) TranslateUpstream(kctx krt.HandlerContext, ucc ir.UniqlyConnectedClient, u ir.Upstream) *envoy_config_cluster_v3.Cluster {
	gk := schema.GroupKind{
		Group: u.Group,
		Kind:  u.Kind,
	}
	process, ok := t.ContributedUpstreams[gk]
	if !ok {
		// ERROR!
		panic("TODO: report this error on the status")
	}

	if process.InitUpstream == nil {
		// ERROR!
		panic("TODO: report this error on the status")
	}

	out := initializeCluster(u)

	process.InitUpstream(context.TODO(), u, out)

	// now process upstream policies:
	t.runPlugins(kctx, context.TODO(), ucc, u, out)
	return out
}

func (t *UpstreamTranslator) runPlugins(kctx krt.HandlerContext, ctx context.Context, ucc ir.UniqlyConnectedClient, u ir.Upstream, out *envoy_config_cluster_v3.Cluster) {
	for gk, polImpl := range t.ContributedPolicies {
		if polImpl.PerClientProcessUpstream != nil {
			polImpl.PerClientProcessUpstream(kctx, ctx, ucc, u, out)
		}

		if polImpl.ProcessUpstream == nil {
			continue
		}
		for _, pol := range u.AttachedPolicies.Policies[gk] {
			polImpl.ProcessUpstream(ctx, pol.PolicyIr, u, out)
		}
	}
}

func initializeCluster(u ir.Upstream) *envoy_config_cluster_v3.Cluster {

	// circuitBreakers := t.settings.GetGloo().GetCircuitBreakers()
	out := &envoy_config_cluster_v3.Cluster{
		Name:     u.ClusterName(),
		Metadata: new(envoy_config_core_v3.Metadata),
		//	CircuitBreakers:  getCircuitBreakers(upstream.GetCircuitBreakers(), circuitBreakers),
		//	LbSubsetConfig:   createLbConfig(upstream),
		//	HealthChecks:     hcConfig,
		//		OutlierDetection: detectCfg,
		// defaults to Cluster_USE_CONFIGURED_PROTOCOL
		// ProtocolSelection: envoy_config_cluster_v3.Cluster_ClusterProtocolSelection(upstream.GetProtocolSelection()),
		// this field can be overridden by plugins
		ConnectTimeout: durationpb.New(translator.ClusterConnectionTimeout),
		// Http2ProtocolOptions:      getHttp2options(upstream),
		// IgnoreHealthOnHostRemoval: upstream.GetIgnoreHealthOnHostRemoval().GetValue(),
		//	RespectDnsTtl:             upstream.GetRespectDnsTtl().GetValue(),
		//	DnsRefreshRate:            dnsRefreshRate,
		//	PreconnectPolicy:          preconnect,
	}

	//	if sslConfig := upstream.GetSslConfig(); sslConfig != nil {
	//		applyDefaultsToUpstreamSslConfig(sslConfig, t.settings.GetUpstreamOptions())
	//		cfg, err := utils.NewSslConfigTranslator().ResolveUpstreamSslConfig(*secrets, sslConfig)
	//		if err != nil {
	//			// if we are configured to warn on missing tls secret and we match that error, add a
	//			// warning instead of error to the report.
	//			if t.settings.GetGateway().GetValidation().GetWarnMissingTlsSecret().GetValue() &&
	//				errors.Is(err, utils.SslSecretNotFoundError) {
	//				errorList = append(errorList, &Warning{
	//					Message: err.Error(),
	//				})
	//			} else {
	//				errorList = append(errorList, err)
	//			}
	//		} else {
	//			typedConfig, err := utils.MessageToAny(cfg)
	//			if err != nil {
	//				// TODO: Need to change the upstream to use a direct response action instead of leaving the upstream untouched
	//				// Difficult because direct response is not on the upsrtream but on the virtual host
	//				// The fallback listener would take much more piping as well
	//				panic(err)
	//			} else {
	//				out.TransportSocket = &envoy_config_core_v3.TransportSocket{
	//					Name:       wellknown.TransportSocketTls,
	//					ConfigType: &envoy_config_core_v3.TransportSocket_TypedConfig{TypedConfig: typedConfig},
	//				}
	//			}
	//		}
	//	}
	// proxyprotocol may be wiped by some plugins that transform transport sockets
	// see static and failover at time of writing.
	//	if upstream.GetProxyProtocolVersion() != nil {
	//
	//		tp, err := upstream_proxy_protocol.WrapWithPProtocol(out.GetTransportSocket(), upstream.GetProxyProtocolVersion().GetValue())
	//		if err != nil {
	//			errorList = append(errorList, err)
	//		} else {
	//			out.TransportSocket = tp
	//		}
	//	}
	//
	//	// set Type = EDS if we have endpoints for the upstream
	//	if eds {
	//		xds.SetEdsOnCluster(out, t.settings)
	//	}
	return out
}

func setHttp2options(c *envoy_config_cluster_v3.Cluster) {

	if c.GetTypedExtensionProtocolOptions() == nil {
		c.TypedExtensionProtocolOptions = map[string]*anypb.Any{}
	}
	http2ProtocolOptions := &envoy_config_core_v3.Http2ProtocolOptions{}
	opts := &envoy_upstreams_v3.HttpProtocolOptions{
		UpstreamProtocolOptions: &envoy_upstreams_v3.HttpProtocolOptions_ExplicitHttpConfig_{
			ExplicitHttpConfig: &envoy_upstreams_v3.HttpProtocolOptions_ExplicitHttpConfig{
				ProtocolConfig: &envoy_upstreams_v3.HttpProtocolOptions_ExplicitHttpConfig_Http2ProtocolOptions{
					Http2ProtocolOptions: http2ProtocolOptions,
				},
			},
		},
	}
	a, err := anypb.New(opts)
	if err != nil {
		// TODO return and report error instead of panic
		panic(err)
	}

	c.GetTypedExtensionProtocolOptions()["envoy.extensions.upstreams.http.v3.HttpProtocolOptions"] = a

}

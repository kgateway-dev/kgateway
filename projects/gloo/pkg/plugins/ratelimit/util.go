package ratelimit

import (
	envoycore "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	rlconfig "github.com/envoyproxy/go-control-plane/envoy/config/ratelimit/v3"
	envoyratelimit "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ratelimit/v3"
	"github.com/golang/protobuf/ptypes/duration"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

func GenerateEnvoyConfigForFilterWith(
	upstreamRef *core.ResourceRef,
	domain string,
	stage uint32,
	timeout *duration.Duration,
	denyOnFail bool,
) *envoyratelimit.RateLimit {
	var svc *envoycore.GrpcService
	svc = &envoycore.GrpcService{TargetSpecifier: &envoycore.GrpcService_EnvoyGrpc_{
		EnvoyGrpc: &envoycore.GrpcService_EnvoyGrpc{
			ClusterName: translator.UpstreamToClusterName(upstreamRef),
		},
	}}

	curtimeout := DefaultTimeout
	if timeout != nil {
		curtimeout = timeout
	}
	envoyrl := envoyratelimit.RateLimit{
		Domain:          domain,
		Stage:           stage,
		RequestType:     RequestType,
		Timeout:         curtimeout,
		FailureModeDeny: denyOnFail,

		RateLimitService: &rlconfig.RateLimitServiceConfig{
			TransportApiVersion: envoycore.ApiVersion_V3,
			GrpcService:         svc,
		},
	}
	return &envoyrl
}

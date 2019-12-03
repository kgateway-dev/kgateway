package ratelimit

import (
	"time"

	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoyratelimit "github.com/envoyproxy/go-control-plane/envoy/config/filter/http/rate_limit/v2"
	"github.com/solo-io/gloo/pkg/utils/gogoutils"

	rlconfig "github.com/envoyproxy/go-control-plane/envoy/config/ratelimit/v2"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

func GenerateEnvoyConfigForFilterWith(ref core.ResourceRef, domain string, currentState uint32, timeout *time.Duration, denyOnFail bool) *envoyratelimit.RateLimit {
	var svc *envoycore.GrpcService
	svc = &envoycore.GrpcService{TargetSpecifier: &envoycore.GrpcService_EnvoyGrpc_{
		EnvoyGrpc: &envoycore.GrpcService_EnvoyGrpc{
			ClusterName: translator.UpstreamToClusterName(ref),
		},
	}}

	curtimeout := DefaultTimeout
	if timeout != nil {
		curtimeout = *timeout
	}
	envoyrl := envoyratelimit.RateLimit{
		Domain:          domain,
		Stage:           currentState,
		RequestType:     requestType,
		Timeout:         gogoutils.DurationStdToProto(&curtimeout),
		FailureModeDeny: denyOnFail,

		RateLimitService: &rlconfig.RateLimitServiceConfig{
			GrpcService: svc,
		},
	}
	return &envoyrl
}

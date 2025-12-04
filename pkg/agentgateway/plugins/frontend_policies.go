package plugins

import (
	"errors"

	"github.com/agentgateway/agentgateway/go/api"
	"google.golang.org/protobuf/types/known/durationpb"
	"istio.io/istio/pkg/ptr"
	"istio.io/istio/pkg/slices"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/agentgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
)

const (
	frontendTcpPolicySuffix     = ":frontend-tcp"
	frontendTlsPolicySuffix     = ":frontend-tls"
	frontendHttpPolicySuffix    = ":frontend-http"
	frontendLoggingPolicySuffix = ":frontend-logging"
	frontendTracingPolicySuffix = ":frontend-tracing"
)

func translateFrontendPolicyToAgw(
	policy *agentgateway.AgentgatewayPolicy,
	policyTarget *api.PolicyTarget,
) ([]AgwPolicy, error) {
	frontend := policy.Spec.Frontend
	if frontend == nil {
		return nil, nil
	}
	agwPolicies := make([]AgwPolicy, 0)
	var errs []error

	policyName := getFrontendPolicyName(policy.Namespace, policy.Name)

	if s := frontend.HTTP; s != nil {
		pol := translateFrontendHTTP(policy, policyName, policyTarget)
		agwPolicies = append(agwPolicies, pol...)
	}

	if s := frontend.TLS; s != nil {
		pol := translateFrontendTLS(policy, policyName, policyTarget)
		agwPolicies = append(agwPolicies, pol...)
	}

	if s := frontend.TCP; s != nil {
		pol := translateFrontendTCP(policy, policyName, policyTarget)
		agwPolicies = append(agwPolicies, pol...)
	}

	if s := frontend.AccessLog; s != nil {
		pol := translateFrontendAccessLog(policy, policyName, policyTarget)
		agwPolicies = append(agwPolicies, pol...)
	}

	if s := frontend.Tracing; s != nil {
		pol := translateFrontendTracing(policy, policyName, policyTarget)
		agwPolicies = append(agwPolicies, pol...)
	}

	return agwPolicies, errors.Join(errs...)
}

func translateFrontendTracing(policy *agentgateway.AgentgatewayPolicy, name string, target *api.PolicyTarget) []AgwPolicy {
	tracing := policy.Spec.Frontend.Tracing
	tracingPolicy := &api.Policy{
		Key:    name + frontendTracingPolicySuffix + attachmentName(target),
		Name:   TypedResourceName(wellknown.AgentgatewayPolicyGVK.Kind, policy),
		Target: target,
		Kind: &api.Policy_Frontend{
			Frontend: &api.FrontendPolicySpec{
				// TODO: implement this
				Kind: &api.FrontendPolicySpec_Tracing_{Tracing: &api.FrontendPolicySpec_Tracing{}},
			},
		},
	}
	_ = tracing

	logger.Debug("generated tracing policy",
		"policy", policy.Name,
		"agentgateway_policy", tracingPolicy.Name,
		"target", target)

	return []AgwPolicy{{Policy: tracingPolicy}}
}

func translateFrontendAccessLog(policy *agentgateway.AgentgatewayPolicy, name string, target *api.PolicyTarget) []AgwPolicy {
	logging := policy.Spec.Frontend.AccessLog
	spec := &api.FrontendPolicySpec_Logging{}
	if f := logging.Filter; f != nil {
		spec.Filter = (*string)(f)
	}
	if a := logging.Attributes; a != nil {
		f := &api.FrontendPolicySpec_Logging_Fields{
			Remove: a.Remove,
			Add: slices.Map(a.Add, func(e agentgateway.AgentAttributeAdd) *api.FrontendPolicySpec_Logging_Field {
				return &api.FrontendPolicySpec_Logging_Field{
					Name:       e.Name,
					Expression: string(e.Expression),
				}
			}),
		}
		spec.Fields = f
	}

	loggingPolicy := &api.Policy{
		Key:    name + frontendLoggingPolicySuffix + attachmentName(target),
		Name:   TypedResourceName(wellknown.AgentgatewayPolicyGVK.Kind, policy),
		Target: target,
		Kind: &api.Policy_Frontend{
			Frontend: &api.FrontendPolicySpec{
				Kind: &api.FrontendPolicySpec_Logging_{
					Logging: spec,
				},
			},
		},
	}

	logger.Debug("generated logging policy",
		"policy", policy.Name,
		"agentgateway_policy", loggingPolicy.Name,
		"target", target)

	return []AgwPolicy{{Policy: loggingPolicy}}
}

func translateFrontendTCP(policy *agentgateway.AgentgatewayPolicy, name string, target *api.PolicyTarget) []AgwPolicy {
	tcp := policy.Spec.Frontend.TCP
	spec := &api.FrontendPolicySpec_TCP{}
	if ka := tcp.KeepAlive; ka != nil {
		spec.Keepalives = &api.KeepaliveConfig{}
		if ka.Time != nil {
			spec.Keepalives.Time = durationpb.New(ka.Time.Duration)
		}
		if ka.Interval != nil {
			spec.Keepalives.Interval = durationpb.New(ka.Interval.Duration)
		}
		if ka.Retries != nil {
			spec.Keepalives.Retries = castUint32(ka.Retries) //nolint:gosec // G115: kubebuilder validation ensures safe for uint32
		}
	}

	tcpPolicy := &api.Policy{
		Key:    name + frontendTcpPolicySuffix + attachmentName(target),
		Name:   TypedResourceName(wellknown.AgentgatewayPolicyGVK.Kind, policy),
		Target: target,
		Kind: &api.Policy_Frontend{
			Frontend: &api.FrontendPolicySpec{
				Kind: &api.FrontendPolicySpec_Tcp{
					Tcp: spec,
				},
			},
		},
	}

	logger.Debug("generated tcp policy",
		"policy", policy.Name,
		"agentgateway_policy", tcpPolicy.Name,
		"target", target)

	return []AgwPolicy{{Policy: tcpPolicy}}
}

func castUint32[T ~int32](ka *T) *uint32 {
	return ptr.Of((uint32)(*ka))
}

func translateFrontendTLS(policy *agentgateway.AgentgatewayPolicy, name string, target *api.PolicyTarget) []AgwPolicy {
	tls := policy.Spec.Frontend.TLS
	spec := &api.FrontendPolicySpec_TLS{}
	if ka := tls.HandshakeTimeout; ka != nil {
		spec.TlsHandshakeTimeout = durationpb.New(ka.Duration)
	}

	if tls.AlpnProtocols != nil {
		spec.Alpn = &api.Alpn{Protocols: *tls.AlpnProtocols}
	}

	tlsPolicy := &api.Policy{
		Key:    name + frontendTlsPolicySuffix + attachmentName(target),
		Name:   TypedResourceName(wellknown.AgentgatewayPolicyGVK.Kind, policy),
		Target: target,
		Kind: &api.Policy_Frontend{
			Frontend: &api.FrontendPolicySpec{
				Kind: &api.FrontendPolicySpec_Tls{
					Tls: spec,
				},
			},
		},
	}

	logger.Debug("generated tls policy",
		"policy", policy.Name,
		"agentgateway_policy", tlsPolicy.Name,
		"target", target)

	return []AgwPolicy{{Policy: tlsPolicy}}
}

func translateFrontendHTTP(policy *agentgateway.AgentgatewayPolicy, name string, target *api.PolicyTarget) []AgwPolicy {
	http := policy.Spec.Frontend.HTTP
	spec := &api.FrontendPolicySpec_HTTP{}
	if v := http.MaxBufferSize; v != nil {
		spec.MaxBufferSize = castUint32(v) //nolint:gosec // G115: kubebuilder validation ensures safe for uint32
	}
	if v := http.HTTP1MaxHeaders; v != nil {
		spec.Http1MaxHeaders = castUint32(v) //nolint:gosec // G115: kubebuilder validation ensures safe for uint32
	}
	if v := http.HTTP1IdleTimeout; v != nil {
		spec.Http1IdleTimeout = durationpb.New(v.Duration)
	}
	if v := http.HTTP2WindowSize; v != nil {
		spec.Http2WindowSize = castUint32(v) //nolint:gosec // G115: kubebuilder validation ensures safe for uint32
	}
	if v := http.HTTP2ConnectionWindowSize; v != nil {
		spec.Http2ConnectionWindowSize = castUint32(v) //nolint:gosec // G115: kubebuilder validation ensures safe for uint32
	}
	if v := http.HTTP2FrameSize; v != nil {
		spec.Http2FrameSize = castUint32(v) //nolint:gosec // G115: kubebuilder validation ensures safe for uint32
	}
	if v := http.HTTP2KeepaliveInterval; v != nil {
		spec.Http2KeepaliveInterval = durationpb.New(v.Duration)
	}
	if v := http.HTTP2KeepaliveTimeout; v != nil {
		spec.Http2KeepaliveTimeout = durationpb.New(v.Duration)
	}

	httpPolicy := &api.Policy{
		Key:    name + frontendHttpPolicySuffix + attachmentName(target),
		Name:   TypedResourceName(wellknown.AgentgatewayPolicyGVK.Kind, policy),
		Target: target,
		Kind: &api.Policy_Frontend{
			Frontend: &api.FrontendPolicySpec{
				Kind: &api.FrontendPolicySpec_Http{
					Http: spec,
				},
			},
		},
	}

	logger.Debug("generated http policy",
		"policy", policy.Name,
		"agentgateway_policy", httpPolicy.Name,
		"target", target)

	return []AgwPolicy{{Policy: httpPolicy}}
}

package plugins

import (
	"errors"

	"github.com/agentgateway/agentgateway/go/api"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"istio.io/istio/pkg/slices"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
)

const (
	frontendTcpPolicySuffix     = ":frontend-tcp"
	frontendTlsPolicySuffix     = ":frontend-tls"
	frontendHttpPolicySuffix    = ":frontend-http"
	frontendLoggingPolicySuffix = ":frontend-logging"
	frontendTracingPolicySuffix = ":frontend-tracing"
)

func translateFrontendPolicyToAgw(
	ctx PolicyCtx,
	policy *v1alpha1.AgentgatewayPolicy,
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
		pol, err := translateFrontendHTTP(ctx, policy, policyName, policyTarget)
		if err != nil {
			logger.Error("error processing frontend HTTP", "err", err)
			errs = append(errs, err)
		}
		agwPolicies = append(agwPolicies, pol...)
	}

	if s := frontend.TLS; s != nil {
		pol, err := translateFrontendTLS(ctx, policy, policyName, policyTarget)
		if err != nil {
			logger.Error("error processing frontend TLS", "err", err)
			errs = append(errs, err)
		}
		agwPolicies = append(agwPolicies, pol...)
	}

	if s := frontend.TCP; s != nil {
		pol, err := translateFrontendTCP(ctx, policy, policyName, policyTarget)
		if err != nil {
			logger.Error("error processing frontend TCP", "err", err)
			errs = append(errs, err)
		}
		agwPolicies = append(agwPolicies, pol...)
	}

	if s := frontend.AccessLog; s != nil {
		pol, err := translateFrontendAccessLog(ctx, policy, policyName, policyTarget)
		if err != nil {
			logger.Error("error processing frontend AccessLog", "err", err)
			errs = append(errs, err)
		}
		agwPolicies = append(agwPolicies, pol...)
	}

	if s := frontend.Tracing; s != nil {
		pol, err := translateFrontendTracing(ctx, policy, policyName, policyTarget)
		if err != nil {
			logger.Error("error processing frontend Tracing", "err", err)
			errs = append(errs, err)
		}
		agwPolicies = append(agwPolicies, pol...)
	}

	return agwPolicies, errors.Join(errs...)
}

func translateFrontendTracing(ctx PolicyCtx, policy *v1alpha1.AgentgatewayPolicy, name string, target *api.PolicyTarget) ([]AgwPolicy, error) {
	tracing := policy.Spec.Frontend.Tracing
	tracingPolicy := &api.Policy{
		Name:   name + frontendTracingPolicySuffix + attachmentName(target),
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

	return []AgwPolicy{{Policy: tracingPolicy}}, nil
}

func translateFrontendAccessLog(ctx PolicyCtx, policy *v1alpha1.AgentgatewayPolicy, name string, target *api.PolicyTarget) ([]AgwPolicy, error) {
	logging := policy.Spec.Frontend.AccessLog
	spec := &api.FrontendPolicySpec_Logging{}
	if f := logging.Filter; f != nil {
		spec.Filter = wrapperspb.String(string(*f))
	}
	if a := logging.Attributes; a != nil {
		f := &api.FrontendPolicySpec_Logging_Fields{
			Remove: a.Remove,
			Add: slices.Map(a.Add, func(e v1alpha1.AgentAttributeAdd) *api.FrontendPolicySpec_Logging_Field {
				return &api.FrontendPolicySpec_Logging_Field{
					Name:       e.Name,
					Expression: string(e.Expression),
				}
			}),
		}
		spec.Fields = f
	}

	loggingPolicy := &api.Policy{
		Name:   name + frontendLoggingPolicySuffix + attachmentName(target),
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

	return []AgwPolicy{{Policy: loggingPolicy}}, nil
}

func translateFrontendTCP(ctx PolicyCtx, policy *v1alpha1.AgentgatewayPolicy, name string, target *api.PolicyTarget) ([]AgwPolicy, error) {
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
			spec.Keepalives.Retries = wrapperspb.UInt32(uint32(*ka.Retries)) //nolint:gosec // G115: kubebuilder validation ensures safe for uint32
		}
	}

	tcpPolicy := &api.Policy{
		Name:   name + frontendTcpPolicySuffix + attachmentName(target),
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

	return []AgwPolicy{{Policy: tcpPolicy}}, nil
}

func translateFrontendTLS(ctx PolicyCtx, policy *v1alpha1.AgentgatewayPolicy, name string, target *api.PolicyTarget) ([]AgwPolicy, error) {
	tls := policy.Spec.Frontend.TLS
	spec := &api.FrontendPolicySpec_TLS{}
	if ka := tls.HandshakeTimeout; ka != nil {
		spec.TlsHandshakeTimeout = durationpb.New(ka.Duration)
	}

	tlsPolicy := &api.Policy{
		Name:   name + frontendTlsPolicySuffix + attachmentName(target),
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

	return []AgwPolicy{{Policy: tlsPolicy}}, nil
}

func translateFrontendHTTP(ctx PolicyCtx, policy *v1alpha1.AgentgatewayPolicy, name string, target *api.PolicyTarget) ([]AgwPolicy, error) {
	http := policy.Spec.Frontend.HTTP
	spec := &api.FrontendPolicySpec_HTTP{}
	if v := http.MaxBufferSize; v != nil {
		spec.MaxBufferSize = wrapperspb.UInt32(uint32(*v)) //nolint:gosec // G115: kubebuilder validation ensures safe for uint32
	}
	if v := http.HTTP1MaxHeaders; v != nil {
		spec.Http1MaxHeaders = wrapperspb.UInt32(uint32(*v)) //nolint:gosec // G115: kubebuilder validation ensures safe for uint32
	}
	if v := http.HTTP1IdleTimeout; v != nil {
		spec.Http1IdleTimeout = durationpb.New(v.Duration)
	}
	if v := http.HTTP2WindowSize; v != nil {
		spec.Http2WindowSize = wrapperspb.UInt32(uint32(*v)) //nolint:gosec // G115: kubebuilder validation ensures safe for uint32
	}
	if v := http.HTTP2ConnectionWindowSize; v != nil {
		spec.Http2ConnectionWindowSize = wrapperspb.UInt32(uint32(*v)) //nolint:gosec // G115: kubebuilder validation ensures safe for uint32
	}
	if v := http.HTTP2FrameSize; v != nil {
		spec.Http2FrameSize = wrapperspb.UInt32(uint32(*v)) //nolint:gosec // G115: kubebuilder validation ensures safe for uint32
	}
	if v := http.HTTP2KeepaliveInterval; v != nil {
		spec.Http2KeepaliveInterval = durationpb.New(v.Duration)
	}
	if v := http.HTTP2KeepaliveTimeout; v != nil {
		spec.Http2KeepaliveTimeout = durationpb.New(v.Duration)
	}

	httpPolicy := &api.Policy{
		Name:   name + frontendHttpPolicySuffix + attachmentName(target),
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

	return []AgwPolicy{{Policy: httpPolicy}}, nil
}

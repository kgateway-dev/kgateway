package ir

import "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"

// This file exists to avoid changing all the code import paths in one mega PR.
// this file will be removed over time, new code should not use this file.

type (
	TypedFilterConfigMap  = ir.TypedFilterConfigMap
	BackendInit           = ir.BackendInit
	PolicyRef             = ir.PolicyRef
	AttachedPolicyRef     = ir.AttachedPolicyRef
	PolicyAtt             = ir.PolicyAtt
	PolicyAttachmentOpts  = ir.PolicyAttachmentOpts
	AttachedPolicies      = ir.AttachedPolicies
	BackendRefIR          = ir.BackendRefIR
	HttpBackendOrDelegate = ir.HttpBackendOrDelegate
	HttpRouteRuleIR       = ir.HttpRouteRuleIR
	EndpointsForBackend   = ir.EndpointsForBackend
	EndpointWithMd        = ir.EndpointWithMd
	HttpRouteRuleMatchIR  = ir.HttpRouteRuleMatchIR
	PodLocality           = ir.PodLocality
	UniqlyConnectedClient = ir.UniqlyConnectedClient

	BackendObjectIR                   = ir.BackendObjectIR
	GwTranslationCtx                  = ir.GwTranslationCtx
	ListenerContext                   = ir.ListenerContext
	ObjectSource                      = ir.ObjectSource
	PolicyIR                          = ir.PolicyIR
	PolicyWrapper                     = ir.PolicyWrapper
	ProxyTranslationPass              = ir.ProxyTranslationPass
	UnimplementedProxyTranslationPass = ir.UnimplementedProxyTranslationPass

	Gateway                  = ir.Gateway
	ListenerSet              = ir.ListenerSet
	HcmContext               = ir.HcmContext
	HttpBackend              = ir.HttpBackend
	HttpRouteIR              = ir.HttpRouteIR
	Route                    = ir.Route
	RouteBackendContext      = ir.RouteBackendContext
	RouteContext             = ir.RouteContext
	AgentgatewayRouteContext = ir.AgentgatewayRouteContext
	Secret                   = ir.Secret
	VirtualHostContext       = ir.VirtualHostContext

	EndpointMetadata  = ir.EndpointMetadata
	FilterChainCommon = ir.FilterChainCommon
	GatewayExtension  = ir.GatewayExtension
	Resources         = ir.Resources
	TcpRouteIR        = ir.TcpRouteIR
	TlsRouteIR        = ir.TlsRouteIR

	Listener = ir.Listener

	AppProtocol        = ir.AppProtocol
	FilterChainMatch   = ir.FilterChainMatch
	GatewayIR          = ir.GatewayIR
	HttpFilterChainIR  = ir.HttpFilterChainIR
	ListenerIR         = ir.ListenerIR
	Namespaced         = ir.Namespaced
	RouteConfigContext = ir.RouteConfigContext
	TcpIR              = ir.TcpIR
	TlsBundle          = ir.TlsBundle

	CustomEnvoyFilter = ir.CustomEnvoyFilter
	VirtualHost       = ir.VirtualHost
)

var (
	HTTP2AppProtocol            = ir.HTTP2AppProtocol
	NewEndpointsForBackend      = ir.NewEndpointsForBackend
	BackendResourceName         = ir.BackendResourceName
	NewUniqlyConnectedClient    = ir.NewUniqlyConnectedClient
	WithInheritedPolicyPriority = ir.WithInheritedPolicyPriority
	ErrNotAttachable            = ir.ErrNotAttachable
	ParseAppProtocol            = ir.ParseAppProtocol
)

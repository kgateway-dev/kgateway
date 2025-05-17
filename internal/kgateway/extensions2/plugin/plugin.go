package plugin

import "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"

type (
	Plugin              = pluginsdk.Plugin
	ProcessBackend      = pluginsdk.ProcessBackend
	BackendPlugin       = pluginsdk.BackendPlugin
	KGwTranslator       = pluginsdk.KGwTranslator
	EndpointPlugin      = pluginsdk.EndpointPlugin
	AttachmentPoints    = pluginsdk.AttachmentPoints
	ContributesPolicies = pluginsdk.ContributesPolicies
	PolicyPlugin        = pluginsdk.PolicyPlugin
	PolicyReport        = pluginsdk.PolicyReport
	GetPolicyStatusFn   = pluginsdk.GetPolicyStatusFn
	PatchPolicyStatusFn = pluginsdk.PatchPolicyStatusFn
)

const (
	BackendAttachmentPoint = pluginsdk.BackendAttachmentPoint
	GatewayAttachmentPoint = pluginsdk.GatewayAttachmentPoint
	RouteAttachmentPoint   = pluginsdk.RouteAttachmentPoint
)

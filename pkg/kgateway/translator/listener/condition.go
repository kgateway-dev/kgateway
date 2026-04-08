package listener

import gwv1 "sigs.k8s.io/gateway-api/apis/v1"

const (
	GatewayConditionAttachedListenerSets = "AttachedListenerSets"

	GatewayReasonListenerSetsNotAllowed = "ListenerSetsNotAllowed"
	GatewayReasonListenerSetsAttached   = "ListenerSetsAttached"

	ListenerSetReasonListenersNotValid = "ListenersNotValid"
	RouteReasonConflicted              gwv1.RouteConditionReason = "Conflicted"

	ListenerMessageProtocolConflict = "Found conflicting protocols on listeners, a single port can only contain listeners with compatible protocols"
	ListenerMessageHostnameConflict = "Found conflicting hostnames on listeners, all listeners on a single port must have unique hostnames"
)

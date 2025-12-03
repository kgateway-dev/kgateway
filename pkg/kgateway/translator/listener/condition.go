package listener

const (
	GatewayConditionAttachedListenerSets = "AttachedListenerSets"

	GatewayReasonListenerSetsNotAllowed = "ListenerSetsNotAllowed"
	GatewayReasonListenerSetsAttached   = "ListenerSetsAttached"

	ListenerSetReasonListenersNotValid = "ListenersNotValid"

	ListenerMessageProtocolConflict = "Found conflicting protocols on listeners, a single port can only contain listeners with compatible protocols"
	ListenerMessageHostnameConflict = "Found conflicting hostnames on listeners, all listeners on a single port must have unique hostnames"
)

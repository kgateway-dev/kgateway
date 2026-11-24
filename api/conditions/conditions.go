package conditions

const (

	// This condition indicates whether a route has generated some
	// configuration that will soon be ready in the underlying data plane.
	KgatewayConditionProgrammed = "kgateway.dev/Programmed"

	// This reason is used with the "kgateway.dev/Programmed" condition when
	// the condition is true.
	KgatewayReasonProgrammed = "Programmed"

	// This reason is used with the "kgateway.dev/Programmed" condition when
	// the condition is true, but a route rule's sessionPersistence sets an
	// absoluteTimeout that has no effect: it only applies to Cookie sessions
	// with a Permanent cookie lifetimeType. The route is still programmed,
	// without a session lifetime.
	KgatewayReasonSessionPersistenceAbsoluteTimeoutIgnored = "SessionPersistenceAbsoluteTimeoutIgnored"

	// This reason is used with the "kgateway.dev/Programmed" condition when
	// the condition is true, but a route rule's sessionPersistence is
	// overridden by the sessionPersistence of a BackendConfigPolicy attached
	// to one or more of the rule's backends. The route is still programmed,
	// with the BackendConfigPolicy's session persistence for those backends.
	KgatewayReasonSessionPersistenceOverridden = "SessionPersistenceOverridden"

	// This condition on a Gateway reports whether its bootstrap overlay is in effect on
	// its proxy. It is set only when the GatewayParameters of the Gateway or its
	// GatewayClass have a bootstrap overlay.
	KgatewayConditionBootstrapOverlayApplied = "kgateway.dev/BootstrapOverlayApplied"

	// This reason is used with "kgateway.dev/BootstrapOverlayApplied" when the condition
	// is true and the overlay sets no path kgateway manages.
	KgatewayReasonBootstrapOverlayApplied = "Applied"

	// This reason is used with "kgateway.dev/BootstrapOverlayApplied" when the condition
	// is true, KGW_BOOTSTRAP_OVERLAY_MANAGED_PATHS is WARN, and the overlay sets one or
	// more paths kgateway manages. They are applied as written; the message lists them.
	KgatewayReasonBootstrapManagedPathsOverridden = "ManagedPathsOverridden"

	// This reason is used with "kgateway.dev/BootstrapOverlayApplied" when the condition
	// is true, KGW_BOOTSTRAP_OVERLAY_MANAGED_PATHS is PRUNE, and the overlay sets one or
	// more paths kgateway manages. They are ignored and the rest of the overlay is
	// applied; the message lists them.
	KgatewayReasonBootstrapPathsIgnored = "PathsIgnored"

	// This reason is used with "kgateway.dev/BootstrapOverlayApplied" when the condition
	// is false because KGW_BOOTSTRAP_OVERLAY_MANAGED_PATHS is REJECT and the overlay sets
	// one or more paths kgateway manages. The message lists them.
	KgatewayReasonBootstrapPathsDenied = "PathsDenied"

	// This reason is used with "kgateway.dev/BootstrapOverlayApplied" when the condition
	// is false because the overlay does not parse, breaks a rule on BootstrapOverlay.Value,
	// or repeats a name Envoy requires to be unique; or because the merged bootstrap
	// cannot be deployed: its static runtime layers are too large, or its static
	// listeners use every port the Gateway's listeners use. The message names the problem.
	KgatewayReasonBootstrapInvalidOverlay = "InvalidOverlay"

	// This reason is used with "kgateway.dev/BootstrapOverlayApplied" when the condition
	// is false because Envoy rejected the merged bootstrap.
	KgatewayReasonBootstrapValidationFailed = "ValidationFailed"

	// This reason is used with "kgateway.dev/BootstrapOverlayApplied" when the condition
	// is false because kgateway could not run Envoy to validate the merged bootstrap.
	// The message says whether kgateway retries, or whether the controller's Deployment
	// or settings must change first.
	KgatewayReasonBootstrapValidationUnavailable = "ValidationUnavailable"

	// This reason is used with "kgateway.dev/BootstrapOverlayApplied" when the condition
	// is false because kgateway failed while rendering, parsing, merging or encoding
	// the bootstrap. This is a kgateway bug, not a problem with the overlay.
	KgatewayReasonBootstrapControllerError = "ControllerError"

	// This reason is used with "kgateway.dev/BootstrapOverlayApplied" when the condition
	// is false because the generated bootstrap ConfigMap would exceed the API server's
	// 1 MiB limit: the new bootstrap is very large, or proxy pods and ReplicaSets still
	// use earlier bootstrap revisions the ConfigMap keeps for them. The message gives
	// the size of each. None of the Gateway's objects change until the ConfigMap fits;
	// kgateway tries again when those pods and ReplicaSets change or the overlay changes.
	KgatewayReasonBootstrapConfigMapTooLarge = "BootstrapConfigMapTooLarge"

	// This reason is used with "kgateway.dev/BootstrapOverlayApplied" when the condition
	// is false because the proxy does not run the merged bootstrap: a deploymentOverlay
	// mounts a ConfigMap other than the generated bootstrap ConfigMap, or another
	// volume, at the proxy's /etc/envoy, the proxy container's env sets INPUT_CONF to a
	// file outside /etc/envoy, or a second volume is mounted inside /etc/envoy over the
	// bootstrap file the proxy reads.
	KgatewayReasonBootstrapNotMounted = "BootstrapNotMounted"
)

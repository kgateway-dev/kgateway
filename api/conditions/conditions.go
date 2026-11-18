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

	// This condition on a Gateway reports whether the bootstrap patches in its
	// GatewayParameters are in effect on its proxy. It is set only when the Gateway's
	// GatewayParameters have bootstrap patches.
	KgatewayConditionBootstrapPatchApplied = "kgateway.dev/BootstrapPatchApplied"

	// This reason is used with "kgateway.dev/BootstrapPatchApplied" when the condition
	// is true and the patches set no path the control plane manages.
	KgatewayReasonBootstrapPatchApplied = "Applied"

	// This reason is used with "kgateway.dev/BootstrapPatchApplied" when the condition
	// is true and the patches set one or more paths the control plane manages. They
	// are applied as written; the message lists each path.
	KgatewayReasonBootstrapManagedPathsOverridden = "ManagedPathsOverridden"

	// This reason is used with "kgateway.dev/BootstrapPatchApplied" when the condition
	// is true and the controller dropped one or more paths the control plane manages
	// from the patches before applying the rest. The message lists each dropped path.
	KgatewayReasonBootstrapPathsIgnored = "PathsIgnored"

	// This reason is used with "kgateway.dev/BootstrapPatchApplied" when the condition
	// is false because the controller is configured to refuse patches that set paths
	// the control plane manages. The message lists the paths.
	KgatewayReasonBootstrapPathsDenied = "PathsDenied"

	// This reason is used with "kgateway.dev/BootstrapPatchApplied" when the condition
	// is false because a patch does not parse, contains a template, or uses a name the
	// proxy reserves.
	KgatewayReasonBootstrapInvalidOverlay = "InvalidOverlay"

	// This reason is used with "kgateway.dev/BootstrapPatchApplied" when the condition
	// is false because Envoy rejected the patched bootstrap.
	KgatewayReasonBootstrapValidationFailed = "ValidationFailed"

	// This reason is used with "kgateway.dev/BootstrapPatchApplied" when the condition
	// is false because the controller could not run Envoy to validate the patched
	// bootstrap. The message says whether the controller retries, or whether its
	// Deployment or settings must change first.
	KgatewayReasonBootstrapValidationUnavailable = "ValidationUnavailable"

	// This reason is used with "kgateway.dev/BootstrapPatchApplied" when the condition
	// is false because the controller failed to process the bootstrap it generated or
	// the patched result. This is a defect in kgateway, not in the patches.
	KgatewayReasonBootstrapControllerError = "ControllerError"

	// This reason is used with "kgateway.dev/BootstrapPatchApplied" when the condition
	// is false because an object overlay mounts a ConfigMap other than the generated
	// bootstrap ConfigMap, so the proxy does not run the patched bootstrap.
	KgatewayReasonBootstrapNotMounted = "BootstrapNotMounted"
)

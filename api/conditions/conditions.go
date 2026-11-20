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

	// This condition on a Gateway reports whether the bootstrap patch in its
	// GatewayParameters is in effect on its proxy. It is set only when the Gateway's
	// GatewayParameters have a bootstrap patch.
	KgatewayConditionBootstrapPatchApplied = "kgateway.dev/BootstrapPatchApplied"

	// This reason is used with "kgateway.dev/BootstrapPatchApplied" when the condition
	// is true and the patch sets no path the control plane manages.
	KgatewayReasonBootstrapPatchApplied = "Applied"

	// This reason is used with "kgateway.dev/BootstrapPatchApplied" when the condition
	// is true and the patch sets one or more paths the control plane manages. They
	// are applied as written; the message lists each path.
	KgatewayReasonBootstrapManagedPathsOverridden = "ManagedPathsOverridden"

	// This reason is used with "kgateway.dev/BootstrapPatchApplied" when the condition
	// is true and the controller dropped one or more paths the control plane manages
	// from the patch before applying the rest. The message lists each dropped path.
	KgatewayReasonBootstrapPathsIgnored = "PathsIgnored"

	// This reason is used with "kgateway.dev/BootstrapPatchApplied" when the condition
	// is false because the controller is configured to refuse a patch that sets paths
	// the control plane manages. The message lists the paths.
	KgatewayReasonBootstrapPathsDenied = "PathsDenied"

	// This reason is used with "kgateway.dev/BootstrapPatchApplied" when the condition
	// is false because a patch does not parse, contains a template, uses a name the
	// proxy reserves, adds a runtime layer that is not a static layer, or repeats a name
	// Envoy requires to be unique; or because the patched bootstrap cannot be deployed,
	// such as when its static runtime layers are too large or it listens on every
	// Gateway listener port.
	KgatewayReasonBootstrapInvalidPatch = "InvalidPatch"

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
	// the patched result. This is a defect in kgateway, not in the patch.
	KgatewayReasonBootstrapControllerError = "ControllerError"

	// This reason is used with "kgateway.dev/BootstrapPatchApplied" when the condition
	// is false because the generated bootstrap ConfigMap would exceed the API server's
	// 1 MiB limit: the new bootstrap is very large, or proxy pods and ReplicaSets still
	// use earlier bootstrap revisions the ConfigMap keeps for them. The message gives
	// the size of each. None of the Gateway's objects change until it fits.
	KgatewayReasonBootstrapConfigMapTooLarge = "BootstrapConfigMapTooLarge"

	// This reason is used with "kgateway.dev/BootstrapPatchApplied" when the condition
	// is false because an object overlay mounts a ConfigMap other than the generated
	// bootstrap ConfigMap, or another volume, at the proxy's /etc/envoy, so the proxy
	// does not run the patched bootstrap.
	KgatewayReasonBootstrapNotMounted = "BootstrapNotMounted"

	// This condition on a Gateway reports whether a static cluster in its patched
	// bootstrap has the name of a cluster kgateway translates for a backend. Envoy does
	// not let xDS update a cluster defined in the bootstrap, so it ignores the translated
	// cluster: traffic to the backend uses the static cluster, and changes to the backend
	// do not reach the proxy. It is set only when the Gateway's GatewayParameters have
	// a bootstrap patch.
	KgatewayConditionStaticClusterConflicted = "kgateway.dev/StaticClusterConflicted"

	// This reason is used with "kgateway.dev/StaticClusterConflicted" when the condition
	// is true. The message names each static cluster and the backend whose translated
	// cluster Envoy ignores.
	KgatewayReasonTranslatedClusterShadowed = "TranslatedClusterShadowed"

	// This reason is used with "kgateway.dev/StaticClusterConflicted" when the condition
	// is false: no static cluster has the name of a translated backend cluster.
	KgatewayReasonNoStaticClusterConflicts = "NoConflicts"
)

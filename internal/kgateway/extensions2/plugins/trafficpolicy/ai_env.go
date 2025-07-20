package trafficpolicy

const (
	// AiDebugTransformations Controls the debugging log behavior of the AI backend's Envoy transformation filter.
	// When this variable is enabled, Envoy will record detailed HTTP request/response information processed by the AI Gateway.
	// This is very helpful for understanding data flow, debugging transformation rules.
	// Expected values: "true" to enable, any other value (or unset) to disable.
	AiDebugTransformations = "AI_PLUGIN_DEBUG_TRANSFORMATIONS"

	// AiListenAddr can be used to test the ext-proc filter locally.
	// Expected values: A valid network address string (e.g., "127.0.0.1:9000").
	AiListenAddr = "AI_PLUGIN_LISTEN_ADDR"
)

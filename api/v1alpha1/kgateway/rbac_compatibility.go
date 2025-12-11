package kgateway

// Deleted (moved to another GVK), but still necessary in terms of RBAC for
// backwards compatibility:

// +kubebuilder:rbac:groups=gateway.kgateway.dev,resources=agentgatewaybackends,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.kgateway.dev,resources=agentgatewaybackends/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=gateway.kgateway.dev,resources=agentgatewaypolicies,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.kgateway.dev,resources=agentgatewaypolicies/status,verbs=get;update;patch

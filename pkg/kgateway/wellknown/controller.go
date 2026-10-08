package wellknown

const (
	// DefaultGatewayClassName represents the name of the GatewayClass to watch for
	DefaultGatewayClassName = "kgateway"

	// DefaultWaypointClassName is the GatewayClass name for the waypoint.
	DefaultWaypointClassName = "kgateway-waypoint"

	// DefaultGatewayControllerName is the name of the controller that has implemented the Gateway API
	// It is configured to manage GatewayClasses with the name DefaultGatewayClassName
	DefaultGatewayControllerName = "kgateway.dev/kgateway"

	// DefaultGatewayParametersName is the name of the GatewayParameters which is attached by
	// parametersRef to the GatewayClass.
	DefaultGatewayParametersName = "kgateway"

	// ManagedByLabel is the label key for the tool being used to manage the operation of an application
	ManagedByLabel = "app.kubernetes.io/managed-by"

	// WatchLabel marks an object as one that kgateway should watch. It is only consulted for
	// kinds whose discovery mode is set to LABELED (see Settings.SecretDiscoveryMode and
	// Settings.ConfigMapDiscoveryMode), where it is pushed to the API server as a watch
	// selector so that unlabeled objects are never cached.
	WatchLabel = "kgateway.dev/watch"

	// WatchLabelValue is the only WatchLabel value that selects an object. Any other value
	// (including "false") excludes it, so an object can be dropped from the watch without
	// removing the label.
	WatchLabelValue = "true"

	// GatewayNameLabel is a label on GW pods to indicate the name of the gateway
	// they are associated with. For gateway names > 63 chars, this contains a
	// truncated name with hash suffix. Use GatewayNameAnnotation for the full name.
	// Use kubeutils.SafeGatewayLabelValue to compute the value of this label safely
	GatewayNameLabel = "gateway.networking.k8s.io/gateway-name"
	// GatewayNameAnnotation is an annotation on GW pods containing the full gateway name.
	// This is used when the gateway name exceeds the 63-char label value limit.
	GatewayNameAnnotation = "gateway.kgateway.dev/gateway-full-name"
	// GatewayClassNameLabel is a label on GW pods to indicate the name of the GatewayClass
	// they are associated with.
	GatewayClassNameLabel = "gateway.networking.k8s.io/gateway-class-name"

	// LeaderElectionID is the name of the lease that leader election will use for holding the leader lock.
	LeaderElectionID = "kgateway-envoy"

	// DefaultManagedByValue represents the default value of the app.kubernetes.io/managed-by label
	DefaultManagedByValue = "kgateway"
)

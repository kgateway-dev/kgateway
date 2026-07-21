package downward

type DownwardAPI interface {
	PodName() string
	PodNamespace() string
	PodIp() string
	PodSvcAccount() string
	PodUID() string

	NodeName() string
	NodeIp() string

	// NodeZone returns the topology zone of the node (e.g. from topology.kubernetes.io/zone).
	// Populated from the KGATEWAY_NODE_ZONE environment variable.
	NodeZone() string
	// NodeRegion returns the topology region of the node (e.g. from topology.kubernetes.io/region).
	// Populated from the KGATEWAY_NODE_REGION environment variable.
	NodeRegion() string
	// NodeSubzone returns the topology subzone of the node.
	// Populated from the KGATEWAY_NODE_SUBZONE environment variable.
	NodeSubzone() string

	PodLabels() map[string]string
	PodAnnotations() map[string]string
}

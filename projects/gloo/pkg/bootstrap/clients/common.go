package clients

const (
	DefaultK8sQPS   = 50     // 10x the k8s-recommended default; gloo gets busy writing status updates
	DefaultK8sBurst = 100    // 10x the k8s-recommended default; gloo gets busy writing status updates
	DefaultRootKey  = "gloo" // used for vault and consul key-value storage
)

package clients

import errors "github.com/rotisserie/eris"

const (
	DefaultK8sQPS   = 50     // 10x the k8s-recommended default; gloo gets busy writing status updates
	DefaultK8sBurst = 100    // 10x the k8s-recommended default; gloo gets busy writing status updates
	DefaultRootKey  = "gloo" // used for vault and consul key-value storage
)

var (
	// ErrNotImplemented indicates a call was made to an interface method which has not been implemented
	ErrNotImplemented = errors.New("interface method not implemented")
)

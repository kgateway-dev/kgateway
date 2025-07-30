package endpointpicker

import (
	"fmt"

	"istio.io/istio/pkg/kube/krt"
	"sigs.k8s.io/gateway-api-inference-extension/api/v1alpha2"

	krtcollections "github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
)

// resolvePoolEndpoints returns the slice of <IP:Port> for the given pool
// by looking up only the pods that index to it.
func resolvePoolEndpoints(
	pool *v1alpha2.InferencePool,
	idx krt.Index[string, krtcollections.LocalityPod],
) endpoints {
	key := fmt.Sprintf("%s/%s", pool.Namespace, pool.Name)

	var eps endpoints
	for _, p := range idx.Lookup(key) {
		if ip := p.Address(); ip != "" {
			eps = append(eps, endpoint{address: ip, port: pool.Spec.TargetPortNumber})
		}
	}

	return eps
}

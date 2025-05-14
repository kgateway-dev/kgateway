package serviceentry

import (
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"

	// networking "istio.io/api/networking/v1alpha3"
	// networkingclient "istio.io/client-go/pkg/apis/networking/v1"
	"istio.io/istio/pkg/kube/krt"
)

// getBackendForRef allows two types of reference to ServiceEntry
//  1. Directly via the ServiceEntry name and namespace
//  2. Using the `Hostname` kind, targeting any hostname of the SE.
//     This will pick the _oldest_ created SE if there is name collision.
//     Namespace on the ref is ignored.
//
// Both of these use the `networking.istio.io` group.
func (s *serviceEntryPlugin) getBackendForRef(
	kctx krt.HandlerContext,
	key ir.ObjectSource,
	port int32,
) *ir.BackendObjectIR {
	if key.Group != wellknown.ServiceEntryGVK.Group {
		return nil
	}
	switch key.Kind {
	case wellknown.ServiceEntryGVK.Kind:
		return s.resolveServiceEntryBackendRef(kctx, key, port)
	case wellknown.HostnameGVK.Kind:
		return s.resolveHostnameBackendRef(kctx, key.GetName(), port)
	}
	return nil
}

func (s *serviceEntryPlugin) resolveServiceEntryBackendRef(
	kctx krt.HandlerContext,
	key ir.ObjectSource,
	port int32,
) *ir.BackendObjectIR {
	var out *ir.BackendObjectIR

	// TODO currently a single ServiceEntry resource produces a backend _per hostname_.
	// To avoid this we would need to make BackendObjectIR support multiple CanonicalHostnames.
	// In the meantime, to make sure this function returns a consistent result, return the BackendObjectIR
	// with the first hostname, lexicographically/alphabetically.
	results := krt.Fetch(kctx, s.Backends, krt.FilterIndex(s.backendsIndex, makeSrcObjKey(key)))
	for _, res := range results {
		if out == nil || res.CanonicalHostname < out.CanonicalHostname {
			out = &res
		}
	}

	return out
}

func (s *serviceEntryPlugin) resolveHostnameBackendRef(
	kctx krt.HandlerContext,
	hostname string,
	port int32,
) *ir.BackendObjectIR {
	var out *ir.BackendObjectIR

	// It's possible that multple ServiceEntry in the cluster have the same hostname and port.
	// The istio-style behavior is to use the oldest one to prevent a newer ServiceEntry from
	// hijacking traffic to that hostname.
	// TODO consider preferring namespace-local as the first criterion.
	results := krt.Fetch(kctx, s.Backends, krt.FilterIndex(s.backendsIndex, makeHostPortKey(hostname, port)))
	for _, res := range results {
		if out == nil || res.Obj.GetCreationTimestamp().Time.Before(out.Obj.GetCreationTimestamp().Time) {
			out = &res
		}
	}

	return out
}

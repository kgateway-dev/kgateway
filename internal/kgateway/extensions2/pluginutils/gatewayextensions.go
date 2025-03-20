package pluginutils

import (
	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
)

// GetGatewayExtension retrieves a GatewayExtension resource by name and namespace.
// It returns the extension and any error encountered during retrieval.
func GetGatewayExtension(extensions *krtcollections.GatewayExtensionIndex, krtctx krt.HandlerContext, extensionName, ns string) (*ir.GatewayExtension, error) {
	// from := krtcollections.From{
	// 	GroupKind: schema.GroupKind{
	// 		Group: "gateway.kgateway.dev",
	// 		Kind:  "GatewayPolicy",
	// 	},
	// 	Namespace: ns,
	// }
	return nil, nil
	// extension, err := extensions.GetGatewayExtension(krtctx, from, extensionName)
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to find gateway extension %s: %v", extensionName, err)
	// }
	// return extension, nil
}

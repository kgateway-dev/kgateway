package crds

import (
	_ "embed"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

//go:embed gateway-crds.yaml
var GatewayCrds []byte

// GetRouteItems extracts the list of route items from the provided client.ObjectList.
// Supported route list types are:
//
//   - HTTPRouteList
//   - TCPRouteList
func GetRouteItems(list client.ObjectList) ([]client.Object, error) {
	switch routes := list.(type) {
	case *gwv1.HTTPRouteList:
		var objs []client.Object
		for i := range routes.Items {
			objs = append(objs, &routes.Items[i])
		}
		return objs, nil
	case *gwv1a2.TCPRouteList:
		var objs []client.Object
		for i := range routes.Items {
			objs = append(objs, &routes.Items[i])
		}
		return objs, nil
	default:
		return nil, fmt.Errorf("unsupported route type %T", list)
	}
}

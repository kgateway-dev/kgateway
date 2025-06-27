package deployer

import (
	api "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	common "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

var GetGatewayIR = DefaultGatewayIRGetter

func DefaultGatewayIRGetter(gw *api.Gateway, commonCollections *common.CommonCollections) *ir.Gateway {
	gwKey := ir.ObjectSource{
		Group:     wellknown.GatewayGVK.GroupKind().Group,
		Kind:      wellknown.GatewayGVK.GroupKind().Kind,
		Name:      gw.GetName(),
		Namespace: gw.GetNamespace(),
	}

	irGW := commonCollections.GatewayIndex.Gateways.GetKey(gwKey.ResourceName())
	if irGW == nil {
		irGW = GatewayIRFrom(gw)
	}

	return irGW
}

func GatewayIRFrom(gw *api.Gateway) *ir.Gateway {
	out := &ir.Gateway{
		ObjectSource: ir.ObjectSource{
			Group:     api.SchemeGroupVersion.Group,
			Kind:      wellknown.GatewayKind,
			Namespace: gw.Namespace,
			Name:      gw.Name,
		},
		Obj:       gw,
		Listeners: make([]ir.Listener, 0, len(gw.Spec.Listeners)),
	}

	for _, l := range gw.Spec.Listeners {
		out.Listeners = append(out.Listeners, ir.Listener{
			Listener: l,
			Parent:   gw,
		})
	}
	return out
}

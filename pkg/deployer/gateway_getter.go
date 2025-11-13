package deployer

import (
	"istio.io/istio/pkg/util/smallset"
	"k8s.io/apimachinery/pkg/util/sets"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

var GetGatewayIR = DefaultGatewayIRGetter

func DefaultGatewayIRGetter(gw *gwv1.Gateway, commonCollections *collections.CommonCollections) *ir.GatewayForDeployer {
	gwKey := ir.ObjectSource{
		Group:     wellknown.GatewayGVK.GroupKind().Group,
		Kind:      wellknown.GatewayGVK.GroupKind().Kind,
		Name:      gw.GetName(),
		Namespace: gw.GetNamespace(),
	}

	irGW := commonCollections.GatewayIndex.GatewaysForDeployer.GetKey(gwKey.ResourceName())
	if irGW == nil {
		// If its not in the IR we cannot tell, so need to make a guess.
		controllerNameGuess := commonCollections.ControllerName
		irGW = GatewayIRFrom(gw, controllerNameGuess)
	}

	return irGW
}

func GatewayIRFrom(gw *gwv1.Gateway, controllerNameGuess string) *ir.GatewayForDeployer {
	ports := sets.New[int32]()
	for _, l := range gw.Spec.Listeners {
		ports.Insert(l.Port)
	}
	return &ir.GatewayForDeployer{
		ObjectSource: ir.ObjectSource{
			Group:     gwv1.GroupVersion.Group,
			Kind:      wellknown.GatewayKind,
			Namespace: gw.Namespace,
			Name:      gw.Name,
		},
		ControllerName: controllerNameGuess,
		Ports:          smallset.New(ports.UnsortedList()...),
	}
}

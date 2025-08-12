package agentgatewaysyncer

import (
	"istio.io/istio/pkg/kube/krt"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	krtinternal "github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils/krtutil"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
)

type GatewayClass struct {
	Name       string
	Controller gwv1.GatewayController
}

func (g GatewayClass) ResourceName() string {
	return g.Name
}

func GatewayClassesCollection(
	gatewayClasses krt.Collection[*gwv1.GatewayClass],
	krtopts krtinternal.KrtOptions,
) krt.Collection[GatewayClass] {
	return krt.NewCollection(gatewayClasses, func(ctx krt.HandlerContext, obj *gwv1.GatewayClass) *GatewayClass {
		return &GatewayClass{
			Name:       obj.Name,
			Controller: obj.Spec.ControllerName,
		}
	}, krtopts.ToOptions("GatewayClasses")...)
}

func fetchClass(ctx krt.HandlerContext, gatewayClasses krt.Collection[GatewayClass], gc gwv1.ObjectName) *GatewayClass {
	class := krt.FetchOne(ctx, gatewayClasses, krt.FilterKey(string(gc)))
	if class == nil {
		return &GatewayClass{
			Name:       string(gc),
			Controller: wellknown.DefaultGatewayControllerName, // TODO: make this configurable
		}
	}
	return class
}

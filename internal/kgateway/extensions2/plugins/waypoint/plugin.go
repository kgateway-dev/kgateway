package waypoint

import (
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	extensionsplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
)

func NewPlugin() extensionsplug.Plugin {
	return extensionsplug.Plugin{
		ContributesPolicies: extensionsplug.ContributesPolicies{},
		ContributesGwTranslator: func(gw *gwv1.Gateway) extensionsplug.KGwTranslator {
			if gw.Spec.GatewayClassName != wellknown.WaypointClassName {
				return nil
			}
			return NewTranslator()
		},
		ExtraHasSynced: func() bool {
			panic("TODO")
		},
	}
}

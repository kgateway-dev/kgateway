package v1

import (
	"sort"

	"github.com/solo-io/gloo/pkg/api/types/v1"
)

const (
	// TODO: 's/ingress/gateway/g'
	GatewayRoleName = "ingress"

	insecureGatewayListenerName = "insecure-gateway-listener"
	secureGatewayListenerName   = "secure-gateway-listener"
)

// the gateway role is the default role assigned to the gateway proxy
// the event loop will generate this role if it does not exist in the config store before the translation loop
// the insecure gateway listener creates the HTTP listener for unsecured virtual services
// the secure gateway listener creates the HTTPS listener for ssl-configured virtual services
func GatewayRole(bindAddr string, insecurePort, securePort uint32) *v1.Role {
	return &v1.Role{
		Name: GatewayRoleName,
		Listeners: []*v1.Listener{
			{
				Name:        insecureGatewayListenerName,
				BindAddress: bindAddr,
				BindPort:    insecurePort,
			},
			{
				Name:        secureGatewayListenerName,
				BindAddress: bindAddr,
				BindPort:    securePort,
			},
		},
	}
}

// the gateway role automatically selects virtual services that are marked `enabled_for_gateway`
// updates the gateway listeners with the correct list of virtual services they should connect to
func AssignGatewayVirtualServices(insecureListener, secureListener *v1.Listener, virtualServices []*v1.VirtualService) {
	var (
		insecureVirtualServices []string
		secureVirtualServices   []string
	)
	for _, vs := range virtualServices {
		if !vs.DisableForGateways {
			if vs.SslConfig == nil {
				insecureVirtualServices = append(insecureVirtualServices, vs.Name)
			} else {
				secureVirtualServices = append(secureVirtualServices, vs.Name)
			}
		}
	}
	// sort for idempotence
	sort.SliceStable(insecureVirtualServices, func(i, j int) bool {
		return insecureVirtualServices[i] < insecureVirtualServices[j]
	})
	sort.SliceStable(secureVirtualServices, func(i, j int) bool {
		return secureVirtualServices[i] < secureVirtualServices[j]
	})
	insecureListener.VirtualServices = insecureVirtualServices
	secureListener.VirtualServices = secureVirtualServices
}


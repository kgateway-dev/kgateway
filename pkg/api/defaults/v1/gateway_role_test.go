package v1_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	. "github.com/solo-io/gloo/test/helpers"

	. "github.com/solo-io/gloo/pkg/api/defaults/v1"
)

var _ = Describe("GatewayRole", func() {
	bindAddress := "::"
	port := uint32(8080)
	securePort := uint32(8443)
	vs1insecure := NewTestVirtualService("my-vservice-1", NewTestRoute1(), NewTestRoute2())
	vs2secure := NewTestVirtualService("my-vservice-2", NewTestRoute1(), NewTestRoute2())
	vs2secure.SslConfig = &v1.SSLConfig{SecretRef: "foo"}
	vs3nonGateway := NewTestVirtualService("my-vservice-2", NewTestRoute1(), NewTestRoute2())
	vs3nonGateway.DisableForGateways = true
	virtualServices := []*v1.VirtualService{
		vs1insecure,
		vs2secure,
		vs3nonGateway,
	}

	It("creates the default gateway role", func() {
		gatewayRole := GatewayRole(bindAddress, port, securePort)
		AssignGatewayVirtualServices(gatewayRole.Listeners[0], gatewayRole.Listeners[1], virtualServices)
		Expect(gatewayRole).To(Equal(&v1.Role{
			Name: "ingress",
			Listeners: []*v1.Listener{
				{
					Name:            "insecure-gateway-listener",
					BindAddress:     "::",
					BindPort:        8080,
					VirtualServices: []string{"my-vservice-1"},
					Config:          nil,
				},
				{
					Name:            "secure-gateway-listener",
					BindAddress:     "::",
					BindPort:        8443,
					VirtualServices: []string{"my-vservice-2"},
					Config:          nil,
				},
			},
		}))
	})
})

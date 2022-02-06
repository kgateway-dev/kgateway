package translator_test

import (
	"time"

	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	. "github.com/solo-io/gloo/projects/gateway/pkg/translator"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/tcp"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/utils/prototime"
)

var _ = Describe("Tcp Translator", func() {

	var (
		translator *TcpTranslator
	)

	BeforeEach(func() {
		translator = &TcpTranslator{}
	})

	Context("Tcp Gateway", func() {

		It("translates tcp listener options and tcp hosts", func() {
			tcpListenerOptions := &gloov1.TcpListenerOptions{
				TcpProxySettings: &tcp.TcpProxySettings{
					MaxConnectAttempts: &wrappers.UInt32Value{Value: 10},
					IdleTimeout:        prototime.DurationToProto(5 * time.Second),
					TunnelingConfig:    &tcp.TcpProxySettings_TunnelingConfig{Hostname: "proxyhostname"},
				},
			}
			tcpHost := &gloov1.TcpHost{
				Name: "host-one",
				Destination: &gloov1.TcpHost_TcpAction{
					Destination: &gloov1.TcpHost_TcpAction_UpstreamGroup{
						UpstreamGroup: &core.ResourceRef{
							Namespace: ns,
							Name:      "ug-name",
						},
					},
				},
			}
			gw := &v1.Gateway{
				Metadata: &core.Metadata{Namespace: ns, Name: "name"},
				GatewayType: &v1.Gateway_TcpGateway{
					TcpGateway: &v1.TcpGateway{
						Options:  tcpListenerOptions,
						TcpHosts: []*gloov1.TcpHost{tcpHost},
					},
				},
				BindPort: 2,
			}

			listener := translator.ComputeListener(Params{}, defaults.GatewayProxyName, gw, nil)
			Expect(listener).NotTo(BeNil())

			tcpListener := listener.ListenerType.(*gloov1.Listener_TcpListener).TcpListener
            Expect(tcpListener.Options).To(Equal(tcpListenerOptions))
			Expect(tcpListener.TcpHosts).To(HaveLen(1))
			Expect(tcpListener.TcpHosts[0]).To(Equal(tcpHost))
		})

	})

	Context("Non-Tcp Gateway", func() {

		It("returns nil", func() {
			gw := &v1.Gateway{
				Metadata:    &core.Metadata{Namespace: ns, Name: "name"},
				GatewayType: &v1.Gateway_HttpGateway{},
				BindPort:    2,
			}

			listener := translator.ComputeListener(Params{}, defaults.GatewayProxyName, gw, nil)
			Expect(listener).To(BeNil())
		})

	})

})

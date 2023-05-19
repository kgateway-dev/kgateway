package listener_test

import (
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/listener"
	"github.com/solo-io/solo-kit/pkg/api/external/envoy/api/v2/core"
	"github.com/solo-io/solo-kit/test/matchers"
)

var _ = Describe("Plugin", func() {

	var (
		plugin plugins.ListenerPlugin
		out    *envoy_config_listener_v3.Listener
	)

	BeforeEach(func() {
		out = new(envoy_config_listener_v3.Listener)
		plugin = NewPlugin()
	})

	It("should set perConnectionBufferLimitBytes", func() {

		in := &v1.Listener{
			Options: &v1.ListenerOptions{
				PerConnectionBufferLimitBytes: &wrappers.UInt32Value{
					Value: uint32(4096),
				},
			},
		}
		err := plugin.ProcessListener(plugins.Params{}, in, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.PerConnectionBufferLimitBytes.Value).To(BeEquivalentTo(uint32(4096)))
	})

	It("should set socket options", func() {
		in := &v1.Listener{
			Options: &v1.ListenerOptions{
				SocketOptions: []*core.SocketOption{
					{
						Description: "desc",
						Level:       1,
						Name:        2,
						Value:       &core.SocketOption_IntValue{IntValue: 123},
						State:       core.SocketOption_STATE_LISTENING,
					},
				},
			},
		}
		err := plugin.ProcessListener(plugins.Params{}, in, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.SocketOptions).To(BeEquivalentTo([]*envoy_config_core_v3.SocketOption{
			{
				Description: "desc",
				Level:       1,
				Name:        2,
				Value:       &envoy_config_core_v3.SocketOption_IntValue{IntValue: 123},
				State:       envoy_config_core_v3.SocketOption_STATE_LISTENING,
			},
		}))
	})

	Context("should set connection balance config", func() {
		It("should fail if no balancer set", func() {
			in := &v1.Listener{
				Options: &v1.ListenerOptions{
					ConnectionBalanceConfig: &v1.ConnectionBalanceConfig{},
				},
			}
			err := plugin.ProcessListener(plugins.Params{}, in, out)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("connection balancer does not specify balancer type"))
		})

		It("should set Exact balance", func() {
			in := &v1.Listener{
				Options: &v1.ListenerOptions{
					ConnectionBalanceConfig: &v1.ConnectionBalanceConfig{
						ExactBalance: &v1.ConnectionBalanceConfig_ExactBalance{},
					},
				},
			}
			err := plugin.ProcessListener(plugins.Params{}, in, out)
			Expect(err).NotTo(HaveOccurred())
			Expect(out.ConnectionBalanceConfig).To(matchers.MatchProto(&envoy_config_listener_v3.Listener_ConnectionBalanceConfig{
				BalanceType: &envoy_config_listener_v3.Listener_ConnectionBalanceConfig_ExactBalance_{
					ExactBalance: &envoy_config_listener_v3.Listener_ConnectionBalanceConfig_ExactBalance{},
				},
			}))
		})
	})
})

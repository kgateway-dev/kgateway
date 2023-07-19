package connection_limit

import (
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_connection_limit_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/connection_limit/v3"
	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/connection_limit"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"google.golang.org/protobuf/types/known/durationpb"
)

var _ = Describe("Plugin", func() {
	It("copies the connection limit config from the listener to the filter", func() {
		filters, err := NewPlugin().NetworkFilters(plugins.Params{}, &v1.HttpListener{
			Options: &v1.HttpListenerOptions{
				ConnectionLimit: &connection_limit.ConnectionLimit{
					MaxActiveConnections: &wrappers.UInt64Value{Value: 9},
					DelayBeforeClose:     &durationpb.Duration{Seconds: 10},
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		typedConfig, err := utils.MessageToAny(&envoy_config_connection_limit_v3.ConnectionLimit{
			StatPrefix:     StatPrefix,
			MaxConnections: &wrappers.UInt64Value{Value: 9},
			Delay:          &durationpb.Duration{Seconds: 10},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(filters).To(Equal([]plugins.StagedNetworkFilter{
			{
				NetworkFilter: &envoy_config_listener_v3.Filter{
					Name: ExtensionName,
					ConfigType: &envoy_config_listener_v3.Filter_TypedConfig{
						TypedConfig: typedConfig,
					},
				},
				Stage: pluginStage,
			},
		}))

		By("Ensures that max connections must be greater than or equal to 1")
		_, err = NewPlugin().NetworkFilters(plugins.Params{}, &v1.HttpListener{
			Options: &v1.HttpListenerOptions{
				ConnectionLimit: &connection_limit.ConnectionLimit{
					MaxActiveConnections: &wrappers.UInt64Value{Value: 0},
				},
			},
		})
		Expect(err).To(HaveOccurred())
	})
})

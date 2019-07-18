package conversion_test

import (
	"github.com/gogo/protobuf/types"
	. "github.com/onsi/ginkgo"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/api/v2alpha1"
	"github.com/solo-io/gloo/projects/gateway/pkg/conversion"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/grpc_web"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/hcm"
	. "github.com/solo-io/go-utils/testutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var converter conversion.GatewayConverter

var _ = Describe("Gateway Conversion", func() {
	Describe("FromV1ToV2alpha1", func() {
		BeforeEach(func() {
			converter = conversion.NewGatewayConverter()
		})

		It("works", func() {
			meta := core.Metadata{Namespace: "ns", Name: "n"}
			bindAddress := "test-bindaddress"
			bindPort := uint32(100)
			useProxyProto := &types.BoolValue{Value: true}
			virtualServices := []core.ResourceRef{{
				Namespace: "test-ns",
				Name:      "test-name",
			}}
			plugins := &gloov1.HttpListenerPlugins{
				GrpcWeb:                       &grpc_web.GrpcWeb{Disable: true},
				HttpConnectionManagerSettings: &hcm.HttpConnectionManagerSettings{ServerName: "test"},
			}

			input := &v1.Gateway{
				Metadata:        meta,
				Ssl:             true,
				BindAddress:     bindAddress,
				BindPort:        bindPort,
				UseProxyProto:   useProxyProto,
				VirtualServices: virtualServices,
				Plugins:         plugins,
			}
			expected := &v2alpha1.Gateway{
				Metadata:      meta,
				Ssl:           true,
				BindAddress:   bindAddress,
				BindPort:      bindPort,
				UseProxyProto: useProxyProto,
				GatewayType: &v2alpha1.Gateway_HttpGateway{
					HttpGateway: &v2alpha1.HttpGateway{
						VirtualServices: virtualServices,
						Plugins:         plugins,
					},
				},
			}

			actual := converter.FromV1ToV2alpha1(input)
			ExpectEqualProtoMessages(actual, expected)
		})
	})
})

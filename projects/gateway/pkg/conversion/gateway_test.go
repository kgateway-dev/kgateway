package conversion_test

import (
	"github.com/gogo/protobuf/types"
	. "github.com/onsi/ginkgo"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gatewayv2 "github.com/solo-io/gloo/projects/gateway/pkg/api/v2"
	"github.com/solo-io/gloo/projects/gateway/pkg/conversion"
	"github.com/solo-io/gloo/projects/gateway/pkg/translator"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/grpc_web"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/hcm"
	. "github.com/solo-io/go-utils/testutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var converter conversion.GatewayConverter

var _ = Describe("Gateway Conversion", func() {
	Describe("FromV1ToV2", func() {
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

			input := &gatewayv1.Gateway{
				Metadata:        meta,
				Ssl:             true,
				BindAddress:     bindAddress,
				BindPort:        bindPort,
				UseProxyProto:   useProxyProto,
				VirtualServices: virtualServices,
				Plugins:         plugins,
			}
			expected := &gatewayv2.Gateway{
				Metadata:      meta,
				Ssl:           true,
				BindAddress:   bindAddress,
				BindPort:      bindPort,
				UseProxyProto: useProxyProto,
				GatewayType: &gatewayv2.Gateway_HttpGateway{
					HttpGateway: &gatewayv2.HttpGateway{
						VirtualServices: virtualServices,
						Plugins:         plugins,
					},
				},
				GatewayProxyName: translator.GatewayProxyName,
			}

			actual := converter.FromV1ToV2(input)
			ExpectEqualProtoMessages(actual, expected)
		})
	})
})

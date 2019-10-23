package conversion_test

import (
	"github.com/gogo/protobuf/types"
	. "github.com/onsi/ginkgo"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gatewayv2 "github.com/solo-io/gloo/projects/gateway/pkg/api/v2"
	"github.com/solo-io/gloo/projects/gateway/pkg/conversion"
	defaults2 "github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/grpc_web"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/hcm"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	. "github.com/solo-io/go-utils/testutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var (
	converter       conversion.GatewayConverter
	bindAddress     = "test-bindaddress"
	bindPort        = uint32(100)
	useProxyProto   = &types.BoolValue{Value: true}
	virtualServices = []core.ResourceRef{{
		Namespace: "test-ns",
		Name:      "test-name",
	}}
	plugins = &gloov1.HttpListenerPlugins{
		GrpcWeb:                       &grpc_web.GrpcWeb{Disable: true},
		HttpConnectionManagerSettings: &hcm.HttpConnectionManagerSettings{ServerName: "test"},
	}
)

var _ = Describe("Gateway Conversion", func() {
	Describe("FromV1ToV2", func() {
		getMetadata := func(annotations map[string]string) core.Metadata {
			return core.Metadata{
				Namespace:   "ns",
				Name:        "n",
				Cluster:     "my-cluster",
				Labels:      map[string]string{"test": "labels"},
				Annotations: annotations,
			}
		}

		getV1Gateway := func(metadata core.Metadata) *gatewayv1.Gateway {
			return &gatewayv1.Gateway{
				Metadata:        metadata,
				Ssl:             true,
				BindAddress:     bindAddress,
				BindPort:        bindPort,
				UseProxyProto:   useProxyProto,
				VirtualServices: virtualServices,
				Plugins:         plugins,
			}
		}

		getV2Gateway := func(metadata core.Metadata) *gatewayv2.Gateway {
			return &gatewayv2.Gateway{
				Metadata:      metadata,
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
				GatewayProxyName: defaults2.GatewayProxyName,
			}
		}

		BeforeEach(func() {
			converter = conversion.NewGatewayConverter()
		})

		It("works with and without existing annotations", func() {
			existingAnnotations := map[string]string{"foo": "bar"}
			postConversionAnnotations := map[string]string{"foo": "bar", defaults.OriginKey: defaults.ConvertedValue}

			input := getV1Gateway(getMetadata(existingAnnotations))
			expected := getV2Gateway(getMetadata(postConversionAnnotations))
			actual := converter.FromV1ToV2(input)
			ExpectEqualProtoMessages(actual, expected)

			postConversionAnnotations = map[string]string{defaults.OriginKey: defaults.ConvertedValue}

			input = getV1Gateway(getMetadata(nil))
			expected = getV2Gateway(getMetadata(postConversionAnnotations))
			actual = converter.FromV1ToV2(input)
			ExpectEqualProtoMessages(actual, expected)
		})
	})
})

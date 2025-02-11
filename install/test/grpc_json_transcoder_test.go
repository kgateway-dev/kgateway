//go:build ignore

package test

import (
	"encoding/base64"
	"fmt"
	"os"
	"strconv"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	. "github.com/solo-io/k8s-utils/manifesttestutils"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/kgateway-dev/kgateway/v2/internal/gateway/pkg/defaults"
	glootestutils "github.com/kgateway-dev/kgateway/v2/test/testutils"
)

var _ = Describe("GrpcJsonTranscoder helm test", func() {
	var allTests = func(rendererTestCase renderTestCase) {

		var (
			testManifest    TestManifest
			protoDescriptor = getExampleProtoDescriptor()

			// Create a TestManifest out of the custom resource yaml from the configmap,
			// and then perform assertions on the resources (which are assumed to all be Gateways).
			// `matchers` is a map of Gateway name to the assertion to be made on that Gateway.
			// There must be a matcher for every Gateway that's expected to be in the manifest.
			assertCustomResourceManifest = func(matchers map[string]types.GomegaMatcher) {
				configMap := getConfigMap(testManifest, namespace, customResourceConfigMapName)
				ExpectWithOffset(1, configMap.Data).NotTo(BeNil())
				customResourceYaml := configMap.Data["custom-resources"]
				customResourceManifest := NewTestManifestFromYaml(customResourceYaml)
				// make sure that the number of resources found in the manifest equals the number of
				// matchers passed in, so we can ensure that every resource has an associated matcher
				ExpectWithOffset(1, customResourceManifest.NumResources()).To(Equal(len(matchers)))
				for gwName, matcher := range matchers {
					customResourceManifest.ExpectUnstructured("Gateway", namespace, gwName).To(matcher)
				}
			}
		)
		prepareManifest := func(namespace string, values glootestutils.HelmValues) {
			GinkgoHelper()
			tm, err := rendererTestCase.renderer.RenderManifest(namespace, values)
			Expect(err).NotTo(HaveOccurred(), "Failed to render manifest")
			testManifest = tm
		}

		Context("protoDescriptorBin field", func() {
			BeforeEach(func() {
				prepareManifest(namespace, glootestutils.HelmValues{
					ValuesArgs: []string{
						fmt.Sprintf("gatewayProxies.gatewayProxy.gatewaySettings.customHttpGateway.options.grpcJsonTranscoder.protoDescriptorBin=%s", protoDescriptor),
						fmt.Sprintf("gatewayProxies.gatewayProxy.gatewaySettings.customHttpsGateway.options.grpcJsonTranscoder.protoDescriptorBin=%s", protoDescriptor),
					},
				})
			})
			It("renders with the proto descriptor", func() {
				gw := makeUnstructuredGatewayWithProtoDescriptorBin(namespace, "gateway-proxy", false)
				gwSsl := makeUnstructuredGatewayWithProtoDescriptorBin(namespace, "gateway-proxy", true)
				assertCustomResourceManifest(map[string]types.GomegaMatcher{
					defaults.GatewayProxyName:                    BeEquivalentTo(gw),
					getSslGatewayName(defaults.GatewayProxyName): BeEquivalentTo(gwSsl),
				})
			})
		})
		Context("protoDescriptorConfigMap field", func() {
			BeforeEach(func() {
				prepareManifest(namespace, glootestutils.HelmValues{
					ValuesArgs: []string{
						"gatewayProxies.gatewayProxy.gatewaySettings.customHttpGateway.options.grpcJsonTranscoder.protoDescriptorConfigMap.configMapRef.name=my-config-map",
						"gatewayProxies.gatewayProxy.gatewaySettings.customHttpGateway.options.grpcJsonTranscoder.protoDescriptorConfigMap.configMapRef.namespace=gloo-system",
						"gatewayProxies.gatewayProxy.gatewaySettings.customHttpGateway.options.grpcJsonTranscoder.protoDescriptorConfigMap.key=my-key",
						"gatewayProxies.gatewayProxy.gatewaySettings.customHttpsGateway.options.grpcJsonTranscoder.protoDescriptorConfigMap.configMapRef.name=my-config-map",
						"gatewayProxies.gatewayProxy.gatewaySettings.customHttpsGateway.options.grpcJsonTranscoder.protoDescriptorConfigMap.configMapRef.namespace=gloo-system",
						"gatewayProxies.gatewayProxy.gatewaySettings.customHttpsGateway.options.grpcJsonTranscoder.protoDescriptorConfigMap.key=my-key",
						"global.configMaps[0].name=my-config-map",
						"global.configMaps[0].namespace=gloo-system",
						"global.configMaps[0].data.my-key=" + protoDescriptor,
					},
				})
			})
			It("renders with the proto descriptor", func() {
				gw := makeUnstructuredGatewayWithProtoDescriptorConfigMap(namespace, "gateway-proxy", false)
				gwSsl := makeUnstructuredGatewayWithProtoDescriptorConfigMap(namespace, "gateway-proxy", true)
				assertCustomResourceManifest(map[string]types.GomegaMatcher{
					defaults.GatewayProxyName:                    BeEquivalentTo(gw),
					getSslGatewayName(defaults.GatewayProxyName): BeEquivalentTo(gwSsl),
				})
			})
		})
	}

	runTests(allTests)
})

//nolint:unparam // namespace always receives "gloo-system"
func makeUnstructuredGatewayWithProtoDescriptorBin(namespace string, name string, ssl bool) *unstructured.Unstructured {
	GinkgoHelper()

	port := "8080"
	gwName := name
	if ssl {
		port = "8443"
		gwName = getSslGatewayName(name)
	}

	return makeUnstructured(`
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata:
  labels:
    app: gloo
  name: ` + gwName + `
  namespace: ` + namespace + `
spec:
  bindAddress: '::'
  bindPort: ` + port + `
  httpGateway: 
    options:
      grpcJsonTranscoder: 
        protoDescriptorBin: '` + getExampleProtoDescriptor() + `'
  proxyNames:
  - ` + name + `
  ssl: ` + strconv.FormatBool(ssl) + `
  useProxyProto: false
`)
}

//nolint:unparam // namespace always receives "gloo-system"
func makeUnstructuredGatewayWithProtoDescriptorConfigMap(namespace string, name string, ssl bool) *unstructured.Unstructured {
	GinkgoHelper()

	port := "8080"
	gwName := name
	if ssl {
		port = "8443"
		gwName = getSslGatewayName(name)
	}

	return makeUnstructured(`
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata:
  labels:
    app: gloo
  name: ` + gwName + `
  namespace: ` + namespace + `
spec:
  bindAddress: '::'
  bindPort: ` + port + `
  httpGateway: 
    options:
      grpcJsonTranscoder: 
        protoDescriptorConfigMap: 
          configMapRef:
            name: my-config-map
            namespace: gloo-system
          key: my-key
  proxyNames:
  - ` + name + `
  ssl: ` + strconv.FormatBool(ssl) + `
  useProxyProto: false
`)
}

// return a base64-encoded proto descriptor to use for testing
func getExampleProtoDescriptor() string {
	pathToDescriptors := "../../test/v1helpers/test_grpc_service/descriptors/proto.pb"
	bytes, err := os.ReadFile(pathToDescriptors)
	Expect(err).NotTo(HaveOccurred())
	return base64.StdEncoding.EncodeToString(bytes)
}

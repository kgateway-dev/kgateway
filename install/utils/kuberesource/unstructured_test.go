package kuberesource

import (
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Unstructured", func() {

	DescribeTable("should convert unstructured object to typed object", func(obj *unstructured.Unstructured) {
		structured, err := ConvertUnstructured(obj)
		Expect(err).NotTo(HaveOccurred())
		Expect(structured.GetObjectKind().GroupVersionKind().Kind).To(Equal(obj.GetKind()))
	},
		Entry("GatewayParameters", GatewayParametersUnstructured()),
	)

})

func convertToUnstructured(obj interface{}, res *unstructured.Unstructured) (err error) {
	var rawJson []byte
	fmt.Printf("obj: %v", obj)
	rawJson, err = json.Marshal(obj)
	if err != nil {
		return err
	}
	err = res.UnmarshalJSON(rawJson)
	return nil
}

func GatewayParametersUnstructured() *unstructured.Unstructured {
	var rss []*unstructured.Unstructured
	err := json.Unmarshal(unstructuredGatewayParametersJson, &rss)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	ExpectWithOffset(1, rss).To(HaveLen(1))
	return rss[0]
}

var unstructuredGatewayParametersJson = []byte(`[{"apiVersion":"gateway.gloo.solo.io/v1alpha1","kind":"GatewayParameters","metadata":{"labels":{"gloo":"kube-gateway"},"name":"gloo-gateway","namespace":"gloo-system"},"spec":{"kube":{"aiExtension":{"enabled":false,"image":{"pullPolicy":"IfNotPresent","registry":"quay.io/solo-io","repository":"gloo-ai-extension","tag":"1.0.0-sah"}},"deployment":{"replicas":1},"envoyContainer":{"image":{"pullPolicy":"IfNotPresent","registry":"quay.io/solo-io","repository":"gloo-envoy-wrapper","tag":"1.0.0-sah"},"securityContext":{"allowPrivilegeEscalation":false,"capabilities":{"add":["NET_BIND_SERVICE"],"drop":["ALL"]},"readOnlyRootFilesystem":true,"runAsNonRoot":true,"runAsUser":10101}},"floatingUserId":false,"istio":{"istioProxyContainer":{"image":{"pullPolicy":"IfNotPresent","registry":"docker.io/istio","repository":"proxyv2","tag":"1.22.0"},"istioDiscoveryAddress":"istiod.istio-system.svc:15012","istioMetaClusterId":"Kubernetes","istioMetaMeshId":"cluster.local","logLevel":"warning"}},"podTemplate":{"extraLabels":{"gloo":"kube-gateway"}},"sdsContainer":{"bootstrap":{"logLevel":"info"},"image":{"pullPolicy":"IfNotPresent","registry":"quay.io/solo-io","repository":"sds","tag":"1.0.0-sah"}},"service":{"type":"LoadBalancer"},"stats":{"enableStatsRoute":true,"enabled":true,"routePrefixRewrite":"/stats/prometheus","statsRoutePrefixRewrite":"/stats"}}}}]`)

package kuberesource

import (
	"encoding/json"

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

func GatewayParametersUnstructured() *unstructured.Unstructured {
	var rss []*unstructured.Unstructured
	err := json.Unmarshal(unstructuredGatewayParametersJson, &rss)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	ExpectWithOffset(1, rss).To(HaveLen(1))
	return rss[0]
}

var unstructuredGatewayParametersJson = []byte(`[{"apiVersion":"gateway.gloo.solo.io/v1alpha1","kind":"GatewayParameters","metadata":{"labels":{"gloo":"kube-gateway"},"name":"gloo-gateway","namespace":"gloo-system"},"spec":{}}]`)

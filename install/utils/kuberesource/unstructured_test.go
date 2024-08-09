package kuberesource

import (
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/solo-io/gloo/projects/gateway2/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	gatewayParameters = &v1alpha1.GatewayParameters{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "gateway.gloo.solo.io/v1alpha1",
			Kind:       "GatewayParameters",
		},
	}
)

var _ = Describe("Unstructured", func() {

	DescribeTable("should convert unstructured object to typed object", func(obj client.Object) {
		unstructured := &unstructured.Unstructured{}
		err := convertToUnstructured(obj, unstructured)
		Expect(err).NotTo(HaveOccurred())

		structured, err := ConvertUnstructured(unstructured)
		Expect(err).NotTo(HaveOccurred())
		Expect(structured.GetObjectKind().GroupVersionKind()).NotTo(BeNil())
		Expect(structured.GetObjectKind().GroupVersionKind()).To(Equal(obj.GetObjectKind().GroupVersionKind()))
	},
		Entry("GatewayParameters", gatewayParameters),
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
	Expect(err).NotTo(HaveOccurred())
	return nil
}

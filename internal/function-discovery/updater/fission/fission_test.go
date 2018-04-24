package fission_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "github.com/solo-io/gloo/internal/function-discovery/updater/fission"
	"github.com/solo-io/gloo/internal/function-discovery/updater/fission/imported"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/plugins/kubernetes"
	"github.com/solo-io/gloo/pkg/plugins/rest"
	"github.com/solo-io/gloo/test/helpers"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
)

type fakeFunctionInterface struct {
	functionlist fission_imported.FunctionList
}

func (ffi *fakeFunctionInterface) Create(*fission_imported.Function) (*fission_imported.Function, error) {
	panic("should never be called")
}
func (ffi *fakeFunctionInterface) Get(name string) (*fission_imported.Function, error) {
	panic("should never be called")
}
func (ffi *fakeFunctionInterface) Update(*fission_imported.Function) (*fission_imported.Function, error) {
	panic("should never be called")
}
func (ffi *fakeFunctionInterface) Delete(name string, options *metav1.DeleteOptions) error {
	panic("should never be called")
}
func (ffi *fakeFunctionInterface) List(opts metav1.ListOptions) (*fission_imported.FunctionList, error) {
	return &ffi.functionlist, nil
}
func (ffi *fakeFunctionInterface) Watch(opts metav1.ListOptions) (watch.Interface, error) {
	panic("should never be called")
}

type fakeFissionClient struct {
	ffi fakeFunctionInterface
}

func (ffc *fakeFissionClient) Functions(ns string) fission_imported.FunctionInterface {
	return &ffc.ffi
}

var _ = Describe("Fission", func() {

	var fr *FissionRetreiver
	var mockResolver *helpers.MockResolver
	var client *fakeFissionClient
	BeforeEach(func() {
		mockResolver = &helpers.MockResolver{}
		client = &fakeFissionClient{}
		fr = NewFissionRetreiver(client)
	})

	It("should get list of functions", func() {

		client.ffi.functionlist.Items = append(client.ffi.functionlist.Items, fission_imported.Function{
			Metadata: metav1.ObjectMeta{
				Name: "func1",
			},
		})

		us := getFissionService()
		funcs, err := fr.GetFuncs(mockResolver, us)

		Expect(err).NotTo(HaveOccurred())
		Expect(funcs).To(Not(BeEmpty()))
		Expect(funcs).To(HaveLen(1))

		outfunc := funcs[0]

		outspec, err := rest.DecodeFunctionSpec(outfunc.Spec)
		Expect(err).NotTo(HaveOccurred())

		Expect(outspec.Path).To(Equal("/fission-function/func1"))
		Expect(outspec.Header[":method"]).To(Equal("POST"))
		Expect(outspec.PassthroughBody).To(Equal(true))

	})

})

func getFissionService() *v1.Upstream {
	ns := "fission"
	n := "router"

	return &v1.Upstream{
		Name:     "fission-router-80",
		Type:     kubernetes.UpstreamTypeKube,
		Metadata: &v1.Metadata{Namespace: "gloo-system"},
		Spec: kubernetes.EncodeUpstreamSpec(kubernetes.UpstreamSpec{
			ServiceName:      n,
			ServiceNamespace: ns,
			ServicePort:      80,
		}),
	}
}

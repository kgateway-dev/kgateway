package openfaas

import (
	"bytes"
	"io"
	"io/ioutil"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/plugins/kubernetes"

	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/coreplugins/service"
	"github.com/solo-io/gloo/pkg/plugins/rest"
	"github.com/solo-io/gloo/test/helpers"
)

func dummyListFuncs(gw string) (OpenFaasFunctions, error) {
	funcs := OpenFaasFunctions{
		{
			Name: "test",
		},
	}

	return funcs, nil
}

func getKubeUs(ns, n string) *v1.Upstream {
	if ns == "" {
		ns = "openfaas"
	}
	if n == "" {
		n = "gateway"
	}
	return &v1.Upstream{
		Name:     "openfaas-gateway-8080",
		Type:     kubernetes.UpstreamTypeKube,
		Metadata: &v1.Metadata{Namespace: "gloo-system"},
		Spec: kubernetes.EncodeUpstreamSpec(kubernetes.UpstreamSpec{
			ServiceName:      n,
			ServiceNamespace: ns,
			ServicePort:      8080,
		}),
	}
}

func getServiceUs(ns, n string) *v1.Upstream {
	if ns == "" {
		ns = "openfaas"
	}
	if n == "" {
		n = "gateway"
	}
	return &v1.Upstream{
		Name:     n,
		Type:     service.UpstreamTypeService,
		Metadata: &v1.Metadata{Namespace: ns},
		Spec: service.EncodeUpstreamSpec(service.UpstreamSpec{
			Hosts: []service.Host{{
				Addr: "gateway",
				Port: 80,
			}},
		}),
	}
}

func getServices(ns, n string) <-chan *v1.Upstream {
	c := make(chan *v1.Upstream)
	go func() {
		c <- getKubeUs(ns, n)
		c <- getServiceUs(ns, n)
		close(c)
	}()
	return c
}

var _ = Describe("Faas", func() {

	var mockResolver *helpers.MockResolver
	BeforeEach(func() {
		mockResolver = &helpers.MockResolver{Result: "somehost"}
	})

	It("should get list of functions", func() {
		fr := FaasRetriever{Lister: dummyListFuncs}

		for us := range getServices("", "") {
			funcs, err := fr.GetFuncs(mockResolver, us)

			Expect(err).NotTo(HaveOccurred())
			Expect(funcs).To(Not(BeEmpty()))
			Expect(funcs).To(HaveLen(1))

			outfunc := funcs[0]

			outspec, err := rest.DecodeFunctionSpec(outfunc.Spec)
			Expect(err).NotTo(HaveOccurred())

			Expect(outspec.Path).To(Equal("/function/test"))
			Expect(outspec.Header[":method"]).To(Equal("POST"))
			Expect(outspec.PassthroughBody).To(Equal(true))
		}

	})

	It("should ignore non gateway faas upstreams", func() {
		fr := FaasRetriever{Lister: nil}

		for us := range getServices("", "not-the-gateway") {

			funcs, err := fr.GetFuncs(mockResolver, us)

			Expect(err).NotTo(HaveOccurred())
			Expect(funcs).To(BeEmpty())
		}
	})

	It("should not crash on nil metadata upstream", func() {
		fr := FaasRetriever{Lister: nil}

		for us := range getServices("", "not-the-gateway") {
			us.Metadata = nil
			Expect(func() { fr.GetFuncs(mockResolver, us) }).ShouldNot(Panic())
		}
	})

	It("should ignore non faas upstreams", func() {
		fr := FaasRetriever{Lister: nil}

		for us := range getServices("not-openfaas", "") {

			funcs, err := fr.GetFuncs(mockResolver, us)

			Expect(err).NotTo(HaveOccurred())
			Expect(funcs).To(BeEmpty())
		}
	})

	It("get and http(s) url to list", func() {
		var gw string
		f := func(gwinner string) (OpenFaasFunctions, error) {
			gw = gwinner
			return nil, nil
		}
		fr := FaasRetriever{Lister: f}
		us := getKubeUs("", "")

		fr.GetFuncs(mockResolver, us)
		Expect(gw).To(HavePrefix("http"))
	})

	It("get correct gw functions", func() {

		list := listGatewayFunctions(
			func(string) (io.ReadCloser, error) {
				var b bytes.Buffer
				b.WriteString(`[{"name":"qrcode-go","image":"johnmccabe/qrcode","invocationCount":0,"replicas":1,"envProcess":"","availableReplicas":1,"labels":{"com.openfaas.ui.ext":"png","faas_function":"qrcode-go"}}]`)
				c := ioutil.NopCloser(&b)
				return c, nil
			})

		funcs, err := list("blah")
		Expect(err).NotTo(HaveOccurred())
		Expect(funcs).To(HaveLen(1))

	})
})

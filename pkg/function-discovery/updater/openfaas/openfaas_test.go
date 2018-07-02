package openfaas

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/gogo/protobuf/types"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/plugins/kubernetes"

	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/plugins/rest"
	"github.com/solo-io/gloo/test/helpers"
)

const funcListJSON = `[
  {
    "name": "qrcode-go",
    "image": "johnmccabe/qrcode",
    "invocationCount": 0,
    "replicas": 1,
    "envProcess": "",
    "availableReplicas": 1,
    "labels": {
      "com.openfaas.ui.ext": "png",
      "faas_function": "qrcode-go"
    }
  }
]`

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

var _ = Describe("FaaS", func() {
	var (
		httpMockServer *httptest.Server
		mockResolver   *helpers.MockResolver
	)
	BeforeEach(func() {
		httpMockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/system/functions" {
				http.NotFound(w, r)
				return
			}
			b, _ := json.Marshal([]function{
				{
					Name: "test",
				},
			})
			w.Write(b)
		}))
		mockResolver = &helpers.MockResolver{Result: strings.TrimPrefix(httpMockServer.URL, "http://")}
	})
	AfterEach(func() {
		httpMockServer.Close()
	})

	It("should get list of functions", func() {
		us := getKubeUs("", "")
		funcs, err := GetFuncs(mockResolver, us)

		Expect(err).NotTo(HaveOccurred())
		Expect(funcs).To(Not(BeEmpty()))
		Expect(funcs).To(HaveLen(1))

		outfunc := funcs[0]

		outspec, err := rest.DecodeFunctionSpec(outfunc.Spec)
		Expect(err).NotTo(HaveOccurred())

		Expect(outspec.Path).To(Equal("/function/test"))
		Expect(outspec.Header[":method"]).To(Equal("POST"))
		Expect(outspec.PassthroughBody).To(Equal(true))
	})

	It("should ignore non gateway faas upstreams", func() {
		us := getKubeUs("", "not-the-gateway")
		funcs, err := GetFuncs(mockResolver, us)
		Expect(err).NotTo(HaveOccurred())
		Expect(funcs).To(BeEmpty())
	})

	It("should not crash on nil metadata upstream", func() {
		us := getKubeUs("", "not-the-gateway")
		us.Metadata = nil
		Expect(func() { GetFuncs(mockResolver, us) }).ShouldNot(Panic())
	})

	It("should ignore upstreams not in the openfaas namespace", func() {
		us := getKubeUs("not-openfaas", "")
		funcs, err := GetFuncs(mockResolver, us)
		Expect(err).NotTo(HaveOccurred())
		Expect(funcs).To(BeEmpty())
	})

	It("get correct gw functions", func() {
		mockServerWithList := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/system/functions" {
				http.NotFound(w, r)
				return
			}
			w.Write([]byte(funcListJSON))
		}))
		defer mockServerWithList.Close()
		mockResolver = &helpers.MockResolver{Result: strings.TrimPrefix(mockServerWithList.URL, "http://")}
		us := getKubeUs("", "")
		funcs, err := GetFuncs(mockResolver, us)
		Expect(err).NotTo(HaveOccurred())
		Expect(funcs).To(Equal([]*v1.Function{
			{
				Name: "qrcode-go",
				Spec: &types.Struct{
					Fields: map[string]*types.Value{
						"passthrough_body": {
							Kind: &types.Value_BoolValue{BoolValue: true},
						},
						"path": {
							Kind: &types.Value_StringValue{
								StringValue: "/function/qrcode-go",
							},
						},
						"headers": {
							Kind: &types.Value_StructValue{
								StructValue: &types.Struct{
									Fields: map[string]*types.Value{
										":method": {
											Kind: &types.Value_StringValue{StringValue: "POST"},
										},
									},
								},
							},
						},
						"body": {Kind: &types.Value_NullValue{NullValue: 0}},
					},
				},
			},
		}))
	})
})

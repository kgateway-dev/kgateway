package k8sadmisssion

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/rotisserie/eris"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8syamlutil "sigs.k8s.io/yaml"

	"github.com/solo-io/gloo/projects/gateway/pkg/validation"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/crd"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var _ = Describe("ValidatingAdmissionWebhook", func() {

	var (
		srv *httptest.Server
		mv  *mockValidator
		wh  *gatewayValidationWebhook
	)
	BeforeEach(func() {
		mv = &mockValidator{}
		wh = &gatewayValidationWebhook{ctx: context.TODO(), validator: mv}
		srv = httptest.NewServer(wh)
	})
	AfterEach(func() {
		srv.Close()
	})

	gateway := defaults.DefaultGateway("namespace")
	vs := defaults.DefaultVirtualService("namespace", "vs")

	kubeRes := v1.VirtualServiceCrd.KubeResource(vs)
	bytes, err := json.Marshal(kubeRes)
	if err != nil {
		panic(err)
	}

	mapFromVs := map[string]interface{}{}

	// NOTE: This is not the default golang yaml.Unmarshal, because that implementation
	// does not unmarshal into a map[string]interface{}; it unmarshals the file into a map[interface{}]interface{}
	// https://github.com/go-yaml/yaml/issues/139
	err = k8syamlutil.Unmarshal(bytes, &mapFromVs)
	if err != nil {
		panic(err)
	}

	vsList := unstructured.UnstructuredList{
		Object: map[string]interface{}{
			"kind":    "List",
			"version": "v1",
		},
		Items: []unstructured.Unstructured{
			{
				Object: mapFromVs,
			},
		},
	}

	routeTable := &v1.RouteTable{Metadata: core.Metadata{Namespace: "namespace", Name: "rt"}}

	errMsg := "didn't say the magic word"

	DescribeTable("accepts valid admission requests, rejects bad ones", func(valid bool, crd crd.Crd, gvk schema.GroupVersionKind, resource interface{}) {

		if !valid {
			mv.fValidateList = func(ctx context.Context, ul *unstructured.UnstructuredList) (validation.ProxyReports, error) {
				return proxyReports(), fmt.Errorf(errMsg)
			}
			mv.fValidateGateway = func(ctx context.Context, gw *v1.Gateway) (validation.ProxyReports, error) {
				return proxyReports(), fmt.Errorf(errMsg)
			}
			mv.fValidateVirtualService = func(ctx context.Context, vs *v1.VirtualService) (validation.ProxyReports, error) {
				return proxyReports(), fmt.Errorf(errMsg)
			}
			mv.fValidateRouteTable = func(ctx context.Context, rt *v1.RouteTable) (validation.ProxyReports, error) {
				return proxyReports(), fmt.Errorf(errMsg)
			}
		}
		req, err := makeReviewRequest(srv.URL, crd, gvk, v1beta1.Create, resource)

		res, err := srv.Client().Do(req)
		Expect(err).NotTo(HaveOccurred())

		review, err := parseReviewResponse(res)
		Expect(err).NotTo(HaveOccurred())
		Expect(review.Response).NotTo(BeNil())

		if valid {
			Expect(review.Response.Allowed).To(BeTrue())
			Expect(review.Proxies).To(HaveLen(1))
			Expect(review.Proxies[0]).To(ContainSubstring("listener-::-8080"))
		} else {
			Expect(review.Response.Allowed).To(BeFalse())
			Expect(review.Response.Result).NotTo(BeNil())
			Expect(review.Response.Result.Message).To(ContainSubstring(errMsg))
			Expect(review.Proxies).To(HaveLen(1))
			Expect(review.Proxies[0]).To(ContainSubstring("listener-::-8080"))
		}
	},
		Entry("valid gateway", true, v1.GatewayCrd, v1.GatewayCrd.GroupVersionKind(), gateway),
		Entry("invalid gateway", false, v1.GatewayCrd, v1.GatewayCrd.GroupVersionKind(), gateway),
		Entry("valid virtual service", true, v1.VirtualServiceCrd, v1.VirtualServiceCrd.GroupVersionKind(), vs),
		Entry("invalid virtual service", false, v1.VirtualServiceCrd, v1.VirtualServiceCrd.GroupVersionKind(), vs),
		Entry("valid route table", true, v1.RouteTableCrd, v1.RouteTableCrd.GroupVersionKind(), routeTable),
		Entry("invalid route table", false, v1.RouteTableCrd, v1.RouteTableCrd.GroupVersionKind(), routeTable),
		Entry("valid single vs list", true, nil, ListGVK, vsList),
		Entry("invalid single vs list", false, nil, ListGVK, vsList),
	)

	Context("invalid yaml", func() {
		It("rejects the resource even when alwaysAccept=true", func() {
			wh.alwaysAccept = true

			req, err := makeReviewRequestRaw(srv.URL, v1.RouteTableCrd.GroupVersionKind(), v1beta1.Create, routeTable.Metadata.Name, routeTable.Metadata.Namespace, []byte(`{"metadata": [1, 2, 3]}`))
			Expect(err).NotTo(HaveOccurred())

			res, err := srv.Client().Do(req)
			Expect(err).NotTo(HaveOccurred())

			review, err := parseReviewResponse(res)
			Expect(err).NotTo(HaveOccurred())
			Expect(review.Response).NotTo(BeNil())

			Expect(review.Response.Allowed).To(BeFalse())
			Expect(review.Response.Result).NotTo(BeNil())
			Expect(review.Response.Result.Message).To(ContainSubstring("could not unmarshal raw object: unmarshalling from raw json: json: cannot unmarshal array into Go struct field Resource.metadata of type v1.ObjectMeta"))

		})
	})
})

func makeReviewRequest(url string, crd crd.Crd, gvk schema.GroupVersionKind, operation v1beta1.Operation, resource interface{}) (*http.Request, error) {

	switch typedResource := resource.(type) {
	case unstructured.UnstructuredList:
		jsonBytes, err := typedResource.MarshalJSON()
		Expect(err).To(BeNil())
		return makeReviewRequestRaw(url, gvk, operation, "name", "namespace", jsonBytes)
	case resources.InputResource:
		resourceCrd := crd.KubeResource(typedResource)
		raw, err := json.Marshal(resourceCrd)
		if err != nil {
			return nil, err
		}
		return makeReviewRequestRaw(url, gvk, operation, typedResource.GetMetadata().Name, typedResource.GetMetadata().Namespace, raw)
	default:
		Fail("unknown type")
	}

	return nil, eris.Errorf("unknown type")
}

func makeReviewRequestRaw(url string, gvk schema.GroupVersionKind, operation v1beta1.Operation, name, namespace string, raw []byte) (*http.Request, error) {

	review := v1beta1.AdmissionReview{
		Request: &v1beta1.AdmissionRequest{
			UID: "1234",
			Kind: metav1.GroupVersionKind{
				Group:   gvk.Group,
				Version: gvk.Version,
				Kind:    gvk.Kind,
			},
			Name:      name,
			Namespace: namespace,
			Operation: operation,
			Object: runtime.RawExtension{
				Raw: raw,
			},
		},
	}

	body, err := json.Marshal(review)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", url+"/validation", bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-type", "application/json")

	return req, nil
}

func parseReviewResponse(resp *http.Response) (*AdmissionReviewWithProxies, error) {
	var review AdmissionReviewWithProxies
	if err := json.NewDecoder(resp.Body).Decode(&review); err != nil {
		return nil, err
	}
	return &review, nil
}

type mockValidator struct {
	fSync                         func(context.Context, *v1.ApiSnapshot) error
	fValidateList                 func(ctx context.Context, ul *unstructured.UnstructuredList) (validation.ProxyReports, error)
	fValidateGateway              func(ctx context.Context, gw *v1.Gateway) (validation.ProxyReports, error)
	fValidateVirtualService       func(ctx context.Context, vs *v1.VirtualService) (validation.ProxyReports, error)
	fValidateDeleteVirtualService func(ctx context.Context, vs core.ResourceRef) error
	fValidateRouteTable           func(ctx context.Context, rt *v1.RouteTable) (validation.ProxyReports, error)
	fValidateDeleteRouteTable     func(ctx context.Context, rt core.ResourceRef) error
}

func (v *mockValidator) Sync(ctx context.Context, snap *v1.ApiSnapshot) error {
	if v.fSync == nil {
		return nil
	}
	return v.fSync(ctx, snap)
}

func (v *mockValidator) ValidateList(ctx context.Context, ul *unstructured.UnstructuredList) (validation.ProxyReports, error) {
	if v.fValidateList == nil {
		return proxyReports(), nil
	}
	return v.fValidateList(ctx, ul)
}

func (v *mockValidator) ValidateGateway(ctx context.Context, gw *v1.Gateway) (validation.ProxyReports, error) {
	if v.fValidateGateway == nil {
		return proxyReports(), nil
	}
	return v.fValidateGateway(ctx, gw)
}

func (v *mockValidator) ValidateVirtualService(ctx context.Context, vs *v1.VirtualService) (validation.ProxyReports, error) {
	if v.fValidateVirtualService == nil {
		return proxyReports(), nil
	}
	return v.fValidateVirtualService(ctx, vs)
}

func (v *mockValidator) ValidateDeleteVirtualService(ctx context.Context, vs core.ResourceRef) error {
	if v.fValidateDeleteVirtualService == nil {
		return nil
	}
	return v.fValidateDeleteVirtualService(ctx, vs)
}

func (v *mockValidator) ValidateRouteTable(ctx context.Context, rt *v1.RouteTable) (validation.ProxyReports, error) {
	if v.fValidateRouteTable == nil {
		return proxyReports(), nil
	}
	return v.fValidateRouteTable(ctx, rt)
}

func (v *mockValidator) ValidateDeleteRouteTable(ctx context.Context, rt core.ResourceRef) error {
	if v.fValidateDeleteRouteTable == nil {
		return nil
	}
	return v.fValidateDeleteRouteTable(ctx, rt)
}

func proxyReports() validation.ProxyReports {
	return validation.ProxyReports{
		{
			Metadata: core.Metadata{
				Name:      "listener-::-8080",
				Namespace: "gloo-system",
			},
		}: {
			ListenerReports: nil,
		},
	}
}

package k8sadmisssion

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/solo-io/gloo/projects/gateway/pkg/validation"

	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/extensions/table"
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
	routeTable := &v1.RouteTable{Metadata: core.Metadata{Namespace: "namespace", Name: "rt"}}

	errMsg := "didn't say the magic word"

	table.DescribeTable("accepts valid admission requests, rejects bad ones", func(valid bool, resourceCrd crd.Crd, resource resources.InputResource) {
		req, err := makeReviewRequest(srv.URL, resourceCrd, v1beta1.Create, resource)
		// not critical to these tests, but this isn't ever supposed to be null or empty.
		wh.webhookNamespace = routeTable.Metadata.Namespace

		if !valid {
			switch resource.(type) {
			case *v1.Gateway:
				mv.fValidateGateway = func(ctx context.Context, gw *v1.Gateway) (validation.ProxyReports, error) {
					return nil, fmt.Errorf(errMsg)
				}
			case *v1.VirtualService:
				mv.fValidateVirtualService = func(ctx context.Context, vs *v1.VirtualService) (validation.ProxyReports, error) {
					return nil, fmt.Errorf(errMsg)
				}
			case *v1.RouteTable:
				mv.fValidateRouteTable = func(ctx context.Context, rt *v1.RouteTable) (validation.ProxyReports, error) {
					return nil, fmt.Errorf(errMsg)
				}
			}
		}

		res, err := srv.Client().Do(req)
		Expect(err).NotTo(HaveOccurred())

		review, err := parseReviewResponse(res)
		Expect(err).NotTo(HaveOccurred())
		Expect(review.Response).NotTo(BeNil())

		if valid {
			Expect(review.Response.Allowed).To(BeTrue())
		} else {
			Expect(review.Response.Allowed).To(BeFalse())
			Expect(review.Response.Result).NotTo(BeNil())
			Expect(review.Response.Result.Message).To(ContainSubstring(errMsg))
		}
	},
		table.Entry("valid gateway", true, v1.GatewayCrd, gateway),
		table.Entry("invalid gateway", false, v1.GatewayCrd, gateway),
		table.Entry("valid virtual service", true, v1.VirtualServiceCrd, vs),
		table.Entry("invalid virtual service", false, v1.VirtualServiceCrd, vs),
		table.Entry("valid route table", true, v1.RouteTableCrd, routeTable),
		table.Entry("invalid route table", false, v1.RouteTableCrd, routeTable),
	)

	Context("invalid yaml", func() {
		It("rejects the resource even when alwaysAccept=true", func() {
			wh.alwaysAccept = true
			wh.webhookNamespace = routeTable.Metadata.Namespace

			req, err := makeReviewRequestRaw(srv.URL, v1.RouteTableCrd, v1beta1.Create, routeTable.Metadata.Name, routeTable.Metadata.Namespace, []byte(`{"metadata": [1, 2, 3]}`))
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

	Context("namespace scoping", func() {
		It("Does not process the resource if it's not whitelisted by watchNamespaces", func() {
			wh.alwaysAccept = false
			wh.watchNamespaces = []string{routeTable.Metadata.Namespace}
			wh.webhookNamespace = routeTable.Metadata.Namespace

			req, err := makeReviewRequestRaw(srv.URL, v1.RouteTableCrd, v1beta1.Create, routeTable.Metadata.Name, routeTable.Metadata.Namespace+"other", []byte(`{"metadata": [1, 2, 3]}`))
			Expect(err).NotTo(HaveOccurred())

			res, err := srv.Client().Do(req)
			Expect(err).NotTo(HaveOccurred())

			review, err := parseReviewResponse(res)
			Expect(err).NotTo(HaveOccurred())
			Expect(review.Response).NotTo(BeNil())

			Expect(review.Response.Allowed).To(BeTrue())
			Expect(review.Response.Result).To(BeNil())
		})

		It("Does not process other-namespace gateway resources if readGatewaysFromAllNamespaces is false, even if they're from whitelisted namespaces", func() {
			otherNamespace := routeTable.Metadata.Namespace + "other"
			wh.alwaysAccept = false
			wh.watchNamespaces = []string{routeTable.Metadata.Namespace, otherNamespace}
			wh.webhookNamespace = routeTable.Metadata.Namespace
			wh.readGatewaysFromAllNamespaces = false

			req, err := makeReviewRequestRaw(srv.URL, v1.GatewayCrd, v1beta1.Create, routeTable.Metadata.Name, otherNamespace, []byte(`{"metadata": [1, 2, 3]}`))
			Expect(err).NotTo(HaveOccurred())

			res, err := srv.Client().Do(req)
			Expect(err).NotTo(HaveOccurred())

			review, err := parseReviewResponse(res)
			Expect(err).NotTo(HaveOccurred())
			Expect(review.Response).NotTo(BeNil())

			Expect(review.Response.Allowed).To(BeTrue())
			Expect(review.Response.Result).To(BeNil())
		})
	})
})

func makeReviewRequest(url string, crd crd.Crd, operation v1beta1.Operation, resource resources.InputResource) (*http.Request, error) {

	resourceCrd, err := crd.KubeResource(resource)
	if err != nil {
		return nil, err
	}

	raw, err := json.Marshal(resourceCrd)
	if err != nil {
		return nil, err
	}

	return makeReviewRequestRaw(url, crd, operation, resource.GetMetadata().Name, resource.GetMetadata().Namespace, raw)
}

func makeReviewRequestRaw(url string, crd crd.Crd, operation v1beta1.Operation, name, namespace string, raw []byte) (*http.Request, error) {

	review := v1beta1.AdmissionReview{
		Request: &v1beta1.AdmissionRequest{
			UID: "1234",
			Kind: metav1.GroupVersionKind{
				Group:   crd.GroupVersionKind().Group,
				Version: crd.GroupVersionKind().Version,
				Kind:    crd.GroupVersionKind().Kind,
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

func parseReviewResponse(resp *http.Response) (*v1beta1.AdmissionReview, error) {
	var review v1beta1.AdmissionReview
	if err := json.NewDecoder(resp.Body).Decode(&review); err != nil {
		return nil, err
	}
	return &review, nil
}

type mockValidator struct {
	fSync                         func(context.Context, *v1.ApiSnapshot) error
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

func (v *mockValidator) ValidateGateway(ctx context.Context, gw *v1.Gateway) (validation.ProxyReports, error) {
	if v.fValidateGateway == nil {
		return nil, nil
	}
	return v.fValidateGateway(ctx, gw)
}

func (v *mockValidator) ValidateVirtualService(ctx context.Context, vs *v1.VirtualService) (validation.ProxyReports, error) {
	if v.fValidateVirtualService == nil {
		return nil, nil
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
		return nil, nil
	}
	return v.fValidateRouteTable(ctx, rt)
}

func (v *mockValidator) ValidateDeleteRouteTable(ctx context.Context, rt core.ResourceRef) error {
	if v.fValidateDeleteRouteTable == nil {
		return nil
	}
	return v.fValidateDeleteRouteTable(ctx, rt)
}

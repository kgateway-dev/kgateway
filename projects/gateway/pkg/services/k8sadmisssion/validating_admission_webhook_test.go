package k8sadmisssion

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/go-multierror"

	"github.com/ghodss/yaml"

	"net/http"
	"net/http/httptest"

	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/projects/gateway/pkg/validation"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

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

	unstructuredList := unstructured.UnstructuredList{
		Object: map[string]interface{}{
			"kind":    "List",
			"version": "v1",
		},
	}

	routeTable := &v1.RouteTable{Metadata: &core.Metadata{Namespace: "namespace", Name: "rt"}}

	errMsg := "didn't say the magic word"

	DescribeTable("accepts valid admission requests, rejects bad ones", func(valid bool, crd crd.Crd, gvk schema.GroupVersionKind, resource interface{}) {
		// not critical to these tests, but this isn't ever supposed to be null or empty.
		wh.webhookNamespace = routeTable.Metadata.Namespace

		if !valid {
			mv.fValidateList = func(ctx context.Context, ul *unstructured.UnstructuredList, dryRun bool) (validation.ProxyReports, *multierror.Error) {
				return proxyReports(), &multierror.Error{Errors: []error{fmt.Errorf(errMsg)}}
			}
			mv.fValidateGateway = func(ctx context.Context, gw *v1.Gateway, dryRun bool) (validation.ProxyReports, error) {
				return proxyReports(), fmt.Errorf(errMsg)
			}
			mv.fValidateVirtualService = func(ctx context.Context, vs *v1.VirtualService, dryRun bool) (validation.ProxyReports, error) {
				return proxyReports(), fmt.Errorf(errMsg)
			}
			mv.fValidateRouteTable = func(ctx context.Context, rt *v1.RouteTable, dryRun bool) (validation.ProxyReports, error) {
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
			Expect(review.Proxies).To(BeEmpty())
		} else {
			Expect(review.Response.Allowed).To(BeFalse())
			Expect(review.Response.Result).NotTo(BeNil())
			Expect(review.Response.Result.Message).To(ContainSubstring(errMsg))
			Expect(review.Proxies).To(BeEmpty())
		}
	},
		Entry("valid gateway", true, v1.GatewayCrd, v1.GatewayCrd.GroupVersionKind(), gateway),
		Entry("invalid gateway", false, v1.GatewayCrd, v1.GatewayCrd.GroupVersionKind(), gateway),
		Entry("valid virtual service", true, v1.VirtualServiceCrd, v1.VirtualServiceCrd.GroupVersionKind(), vs),
		Entry("invalid virtual service", false, v1.VirtualServiceCrd, v1.VirtualServiceCrd.GroupVersionKind(), vs),
		Entry("valid route table", true, v1.RouteTableCrd, v1.RouteTableCrd.GroupVersionKind(), routeTable),
		Entry("invalid route table", false, v1.RouteTableCrd, v1.RouteTableCrd.GroupVersionKind(), routeTable),
		Entry("valid unstructured list", true, nil, ListGVK, unstructuredList),
		Entry("invalid unstructured list", false, nil, ListGVK, unstructuredList),
	)

	Context("invalid yaml", func() {

		invalidYamlTests := func(useYamlEncoding bool) {
			It("rejects the resource even when alwaysAccept=true", func() {
				wh.alwaysAccept = true
				wh.webhookNamespace = routeTable.Metadata.Namespace

				req, err := makeReviewRequestRaw(srv.URL, v1.RouteTableCrd.GroupVersionKind(), v1beta1.Create, routeTable.Metadata.Name, routeTable.Metadata.Namespace, []byte(`{"metadata": [1, 2, 3]}`), useYamlEncoding, false)
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
		}

		Context("json encoded request to validation server", func() {
			invalidYamlTests(false)
		})
		Context("yaml encoded request to validation server", func() {
			invalidYamlTests(true)
		})
	})

	Context("returns proxies", func() {
		It("returns proxy if requested", func() {
			mv.fValidateGateway = func(ctx context.Context, gw *v1.Gateway, dryRun bool) (validation.ProxyReports, error) {
				return proxyReports(), fmt.Errorf(errMsg)
			}

			req, err := makeReviewRequestWithProxies(srv.URL, v1.GatewayCrd, gateway.GroupVersionKind(), v1beta1.Create, gateway, true)
			Expect(err).NotTo(HaveOccurred())

			res, err := srv.Client().Do(req)
			Expect(err).NotTo(HaveOccurred())

			review, err := parseReviewResponse(res)
			Expect(err).NotTo(HaveOccurred())
			Expect(review.Response).NotTo(BeNil())

			Expect(review.Response.Allowed).To(BeFalse())
			Expect(review.Response.Result).ToNot(BeNil())
			Expect(review.Proxies).To(HaveLen(1))
			Expect(review.Proxies[0]).To(ContainSubstring("listener-::-8080"))
		})
	})

	Context("namespace scoping", func() {
		It("does not process the resource if it's not whitelisted by watchNamespaces", func() {
			wh.alwaysAccept = false
			wh.watchNamespaces = []string{routeTable.Metadata.Namespace}
			wh.webhookNamespace = routeTable.Metadata.Namespace

			req, err := makeReviewRequestRawJsonEncoded(srv.URL, v1.RouteTableCrd.GroupVersionKind(), v1beta1.Create, routeTable.Metadata.Name, routeTable.Metadata.Namespace+"other", []byte(`{"metadata": [1, 2, 3]}`), false)
			Expect(err).NotTo(HaveOccurred())

			res, err := srv.Client().Do(req)
			Expect(err).NotTo(HaveOccurred())

			review, err := parseReviewResponse(res)
			Expect(err).NotTo(HaveOccurred())
			Expect(review.Response).NotTo(BeNil())

			Expect(review.Response.Allowed).To(BeTrue())
			Expect(review.Response.Result).To(BeNil())
		})

		It("does not process other-namespace gateway resources if readGatewaysFromAllNamespaces is false, even if they're from whitelisted namespaces", func() {
			otherNamespace := routeTable.Metadata.Namespace + "other"
			wh.alwaysAccept = false
			wh.watchNamespaces = []string{routeTable.Metadata.Namespace, otherNamespace}
			wh.webhookNamespace = routeTable.Metadata.Namespace
			wh.readGatewaysFromAllNamespaces = false

			req, err := makeReviewRequestRawJsonEncoded(srv.URL, v1.GatewayCrd.GroupVersionKind(), v1beta1.Create, routeTable.Metadata.Name, otherNamespace, []byte(`{"metadata": [1, 2, 3]}`), false)
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

	DescribeTable(
		"respects field manager in admission request",
		func(requestFieldManager string, ignoredFieldManagers []string, expectedRequestAllowed bool) {
			wh.ignoredFieldManagers = map[string]struct{}{}
			for _, ignoredFieldManager := range ignoredFieldManagers {
				wh.ignoredFieldManagers[ignoredFieldManager] = struct{}{}
			}

			gvk := v1.GatewayCrd.GroupVersionKind()

			rawUpdateOptions, err := json.Marshal(metav1.UpdateOptions{
				FieldManager: requestFieldManager,
			})
			Expect(err).NotTo(HaveOccurred())

			admissionReview := AdmissionReviewWithProxies{
				AdmissionRequestWithProxies: AdmissionRequestWithProxies{
					AdmissionReview: v1beta1.AdmissionReview{
						Request: &v1beta1.AdmissionRequest{
							UID: "1234",
							Kind: metav1.GroupVersionKind{
								Group:   gvk.Group,
								Version: gvk.Version,
								Kind:    gvk.Kind,
							},
							Name:      "name",
							Namespace: "namespace",
							Operation: v1beta1.Update,
							Options: runtime.RawExtension{
								Raw: rawUpdateOptions,
							},
						},
					},
					ReturnProxies: false,
				},
				AdmissionResponseWithProxies: AdmissionResponseWithProxies{},
			}

			req, err := convertAdmissionReviewIntoRequest(srv.URL, admissionReview, false)
			Expect(err).NotTo(HaveOccurred())

			res, err := srv.Client().Do(req)
			Expect(err).NotTo(HaveOccurred())

			review, err := parseReviewResponse(res)
			Expect(err).NotTo(HaveOccurred())
			Expect(review.Response).NotTo(BeNil())

			Expect(review.Response.Allowed).To(Equal(expectedRequestAllowed))
		},

		Entry("field manager in ignored list",
			"requestManager", []string{"requestManager"}, true),
		Entry("field manager not in ignored list",
			"requestManager", []string{""}, false),
		Entry("no field manager",
			"", []string{"requestManager"}, false),
	)
})

func makeReviewRequest(url string, crd crd.Crd, gvk schema.GroupVersionKind, operation v1beta1.Operation, resource interface{}) (*http.Request, error) {
	return makeReviewRequestWithProxies(url, crd, gvk, operation, resource, false)
}

func makeReviewRequestWithProxies(url string, crd crd.Crd, gvk schema.GroupVersionKind, operation v1beta1.Operation, resource interface{}, returnProxies bool) (*http.Request, error) {

	switch typedResource := resource.(type) {
	case unstructured.UnstructuredList:
		jsonBytes, err := typedResource.MarshalJSON()
		Expect(err).To(BeNil())
		return makeReviewRequestRawJsonEncoded(url, gvk, operation, "name", "namespace", jsonBytes, returnProxies)
	case resources.InputResource:
		resourceCrd, err := crd.KubeResource(typedResource)
		if err != nil {
			return nil, err
		}

		raw, err := json.Marshal(resourceCrd)
		if err != nil {
			return nil, err
		}
		return makeReviewRequestRawJsonEncoded(url, gvk, operation, typedResource.GetMetadata().Name, typedResource.GetMetadata().Namespace, raw, returnProxies)
	default:
		Fail("unknown type")
	}

	return nil, eris.Errorf("unknown type")
}

func makeReviewRequestRawJsonEncoded(url string, gvk schema.GroupVersionKind, operation v1beta1.Operation, name, namespace string, raw []byte, returnProxies bool) (*http.Request, error) {
	return makeReviewRequestRaw(url, gvk, operation, name, namespace, raw, false, returnProxies)
}

func makeReviewRequestRaw(url string, gvk schema.GroupVersionKind, operation v1beta1.Operation, name, namespace string, raw []byte, useYamlEncoding, returnProxies bool) (*http.Request, error) {

	review := AdmissionReviewWithProxies{
		AdmissionRequestWithProxies: AdmissionRequestWithProxies{
			AdmissionReview: v1beta1.AdmissionReview{
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
			},
			ReturnProxies: returnProxies,
		},
		AdmissionResponseWithProxies: AdmissionResponseWithProxies{},
	}

	return convertAdmissionReviewIntoRequest(url, review, useYamlEncoding)
}

func convertAdmissionReviewIntoRequest(url string, review AdmissionReviewWithProxies, useYamlEncoding bool) (*http.Request, error) {
	var (
		contentType string
		body        []byte
		err         error
	)
	if useYamlEncoding {
		contentType = ApplicationYaml
		body, err = yaml.Marshal(review)
	} else {
		contentType = ApplicationJson
		body, err = json.Marshal(review)
	}
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", url+"/validation", bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-type", contentType)

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
	fValidateList                 func(ctx context.Context, ul *unstructured.UnstructuredList, dryRun bool) (validation.ProxyReports, *multierror.Error)
	fValidateGateway              func(ctx context.Context, gw *v1.Gateway, dryRun bool) (validation.ProxyReports, error)
	fValidateVirtualService       func(ctx context.Context, vs *v1.VirtualService, dryRun bool) (validation.ProxyReports, error)
	fValidateDeleteVirtualService func(ctx context.Context, vs *core.ResourceRef, dryRun bool) error
	fValidateRouteTable           func(ctx context.Context, rt *v1.RouteTable, dryRun bool) (validation.ProxyReports, error)
	fValidateDeleteRouteTable     func(ctx context.Context, rt *core.ResourceRef, dryRun bool) error
}

func (v *mockValidator) Sync(ctx context.Context, snap *v1.ApiSnapshot) error {
	if v.fSync == nil {
		return nil
	}
	return v.fSync(ctx, snap)
}

func (v *mockValidator) ValidateList(ctx context.Context, ul *unstructured.UnstructuredList, dryRun bool) (validation.ProxyReports, *multierror.Error) {
	if v.fValidateList == nil {
		return proxyReports(), nil
	}
	return v.fValidateList(ctx, ul, dryRun)
}

func (v *mockValidator) ValidateGateway(ctx context.Context, gw *v1.Gateway, dryRun bool) (validation.ProxyReports, error) {
	if v.fValidateGateway == nil {
		return proxyReports(), nil
	}
	return v.fValidateGateway(ctx, gw, dryRun)
}

func (v *mockValidator) ValidateVirtualService(ctx context.Context, vs *v1.VirtualService, dryRun bool) (validation.ProxyReports, error) {
	if v.fValidateVirtualService == nil {
		return proxyReports(), nil
	}
	return v.fValidateVirtualService(ctx, vs, dryRun)
}

func (v *mockValidator) ValidateDeleteVirtualService(ctx context.Context, vs *core.ResourceRef, dryRun bool) error {
	if v.fValidateDeleteVirtualService == nil {
		return nil
	}
	return v.fValidateDeleteVirtualService(ctx, vs, dryRun)
}

func (v *mockValidator) ValidateRouteTable(ctx context.Context, rt *v1.RouteTable, dryRun bool) (validation.ProxyReports, error) {
	if v.fValidateRouteTable == nil {
		return proxyReports(), nil
	}
	return v.fValidateRouteTable(ctx, rt, dryRun)
}

func (v *mockValidator) ValidateDeleteRouteTable(ctx context.Context, rt *core.ResourceRef, dryRun bool) error {
	if v.fValidateDeleteRouteTable == nil {
		return nil
	}
	return v.fValidateDeleteRouteTable(ctx, rt, dryRun)
}

func proxyReports() validation.ProxyReports {
	return validation.ProxyReports{
		{
			Metadata: &core.Metadata{
				Name:      "listener-::-8080",
				Namespace: "gloo-system",
			},
		}: {
			ListenerReports: nil,
		},
	}
}

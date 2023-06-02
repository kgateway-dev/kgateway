package k8sadmission

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"

	"github.com/ghodss/yaml"
	"github.com/hashicorp/go-multierror"
	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/projects/gateway/pkg/validation"
	validation2 "github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/static"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	extauth "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
	gloov1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/crd"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var _ = Describe("ValidatingAdmissionWebhook", func() {

	var (
		srv    *httptest.Server
		mv     *mockValidator
		wh     *gatewayValidationWebhook
		errMsg = "didn't say the magic word"
	)

	BeforeEach(func() {
		mv = &mockValidator{}
		wh = &gatewayValidationWebhook{
			webhookNamespace: "namespace",
			ctx:              context.TODO(),
			validator:        mv,
		}
		srv = httptest.NewServer(wh)

	})

	setMockFunctions := func() {
		mv.fValidateList = func(ctx context.Context, ul *unstructured.UnstructuredList, dryRun bool) (*validation.Reports, *multierror.Error) {
			return reports(), &multierror.Error{Errors: []error{fmt.Errorf(errMsg)}}
		}
		mv.fValidateModifiedGvk = func(ctx context.Context, gvk schema.GroupVersionKind, resource resources.Resource, dryRun bool) (*validation.Reports, error) {
			return reports(), fmt.Errorf(errMsg)
		}
		mv.fValidateDeleteGvk = func(ctx context.Context, gvk schema.GroupVersionKind, resource resources.Resource, dryRun bool) error {
			return fmt.Errorf(errMsg)
		}
	}

	AfterEach(func() {
		srv.Close()
	})

	gateway := defaults.DefaultGateway("namespace")
	vs := defaults.DefaultVirtualService("namespace", "vs")
	upstream := &gloov1.Upstream{
		Metadata: &core.Metadata{
			Name:      "us",
			Namespace: "namespace",
		},
		UpstreamType: &gloov1.Upstream_Static{
			Static: &static.UpstreamSpec{
				Hosts: []*static.Host{
					{
						Addr: "localhost",
						Port: 12345,
					},
				},
			},
		},
	}
	secret := &gloov1.Secret{
		Metadata: &core.Metadata{
			Name:      "secret",
			Namespace: "namespace",
		},
		Kind: &gloov1.Secret_Oauth{
			Oauth: &extauth.OauthSecret{
				ClientSecret: "thisisasecret",
			},
		},
	}

	unstructuredList := unstructured.UnstructuredList{
		Object: map[string]interface{}{
			"kind":    "List",
			"version": "v1",
		},
	}

	routeTable := &v1.RouteTable{Metadata: &core.Metadata{Namespace: "namespace", Name: "rt"}}

	DescribeTable("processes admission requests with auto-accept validator", func(crd crd.Crd, gvk schema.GroupVersionKind, op v1beta1.Operation, resourceOrRef interface{}) {
		reviewRequest := makeReviewRequest(srv.URL, crd, gvk, op, resourceOrRef)
		res, err := srv.Client().Do(reviewRequest)
		Expect(err).NotTo(HaveOccurred())

		review, err := parseReviewResponse(res)
		Expect(err).NotTo(HaveOccurred())
		Expect(review.Response).NotTo(BeNil())

		Expect(review.Response.Allowed).To(BeTrue())
		Expect(review.Proxies).To(BeEmpty())
	},
		Entry("gateway, accepted", v1.GatewayCrd, v1.GatewayCrd.GroupVersionKind(), v1beta1.Create, gateway),
		Entry("virtual service, accepted", v1.VirtualServiceCrd, v1.VirtualServiceCrd.GroupVersionKind(), v1beta1.Create, vs),
		Entry("virtual service deletion, accepted", v1.VirtualServiceCrd, v1.VirtualServiceCrd.GroupVersionKind(), v1beta1.Delete, vs.GetMetadata().Ref()),
		Entry("route table, accepted", v1.RouteTableCrd, v1.RouteTableCrd.GroupVersionKind(), v1beta1.Create, routeTable),
		Entry("route table deletion, accepted", v1.RouteTableCrd, v1.RouteTableCrd.GroupVersionKind(), v1beta1.Delete, routeTable.GetMetadata().Ref()),
		Entry("unstructured list, accepted", nil, ListGVK, v1beta1.Create, unstructuredList),
		Entry("upstream, accepted", gloov1.UpstreamCrd, gloov1.UpstreamCrd.GroupVersionKind(), v1beta1.Create, upstream),
		Entry("upstream deletion, accepted", gloov1.UpstreamCrd, gloov1.UpstreamCrd.GroupVersionKind(), v1beta1.Delete, upstream.GetMetadata().Ref()),
		Entry("secret deletion, accepted", gloov1.SecretCrd, gloov1.SecretCrd.GroupVersionKind(), v1beta1.Delete, secret.GetMetadata().Ref()),
	)

	DescribeTable("processes admission requests with auto-fail validator", func(crd crd.Crd, gvk schema.GroupVersionKind, op v1beta1.Operation, resourceOrRef interface{}) {
		setMockFunctions()
		req := makeReviewRequest(srv.URL, crd, gvk, op, resourceOrRef)

		res, err := srv.Client().Do(req)
		Expect(err).NotTo(HaveOccurred())

		review, err := parseReviewResponse(res)
		Expect(err).NotTo(HaveOccurred())
		Expect(review.Response).NotTo(BeNil())

		Expect(review.Response.Allowed).To(BeFalse())
		Expect(review.Response.Result).NotTo(BeNil())
		Expect(review.Response.Result.Message).To(ContainSubstring(errMsg))
		Expect(review.Proxies).To(BeEmpty())

	},
		Entry("gateway, rejected", v1.GatewayCrd, v1.GatewayCrd.GroupVersionKind(), v1beta1.Create, gateway),
		Entry("virtual service, rejected", v1.VirtualServiceCrd, v1.VirtualServiceCrd.GroupVersionKind(), v1beta1.Create, vs),
		Entry("virtual service deletion, rejected", v1.VirtualServiceCrd, v1.VirtualServiceCrd.GroupVersionKind(), v1beta1.Delete, vs.GetMetadata().Ref()),
		Entry("route table, rejected", v1.RouteTableCrd, v1.RouteTableCrd.GroupVersionKind(), v1beta1.Create, routeTable),
		Entry("route table deletion, rejected", v1.RouteTableCrd, v1.RouteTableCrd.GroupVersionKind(), v1beta1.Delete, routeTable.GetMetadata().Ref()),
		Entry("unstructured list, rejected", nil, ListGVK, v1beta1.Create, unstructuredList),
		Entry("upstream, rejected", gloov1.UpstreamCrd, gloov1.UpstreamCrd.GroupVersionKind(), v1beta1.Create, upstream),
		Entry("upstream deletion, rejected", gloov1.UpstreamCrd, gloov1.UpstreamCrd.GroupVersionKind(), v1beta1.Delete, upstream.GetMetadata().Ref()),
		Entry("secret deletion, rejected", gloov1.SecretCrd, gloov1.SecretCrd.GroupVersionKind(), v1beta1.Delete, secret.GetMetadata().Ref()),
	)

	DescribeTable("processes status updates with auto-fail validator", func(expectAllowed bool, crd crd.Crd, gvk schema.GroupVersionKind, op v1beta1.Operation, resource resources.InputResource) {
		setMockFunctions()
		resourceCrd, err := crd.KubeResource(resource)
		Expect(err).NotTo(HaveOccurred())
		raw, err := json.Marshal(resourceCrd)
		Expect(err).NotTo(HaveOccurred())

		// Ensure the oldResource only differs by a status change and metadata that shouldn't affect the resource hash
		oldResourceCrd, err := crd.KubeResource(resource)
		Expect(err).NotTo(HaveOccurred())
		oldResourceCrd.Status = map[string]interface{}{
			"namespace": core.Status{
				State: core.Status_Pending,
			},
		}
		oldResourceCrd.Generation = 123
		oldResourceCrd.ResourceVersion = "123"
		oldRaw, err := json.Marshal(oldResourceCrd)
		Expect(err).NotTo(HaveOccurred())

		Expect(oldResourceCrd.Status).NotTo(Equal(resourceCrd.Status))
		Expect(oldResourceCrd.Generation).NotTo(Equal(resourceCrd.Generation))
		Expect(oldResourceCrd.ResourceVersion).NotTo(Equal(resourceCrd.ResourceVersion))

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
						Name:      resource.GetMetadata().GetName(),
						Namespace: resource.GetMetadata().GetNamespace(),
						Operation: op,
						Object: runtime.RawExtension{
							Raw: raw,
						},
						OldObject: runtime.RawExtension{
							Raw: oldRaw,
						},
					},
				},
				ReturnProxies: false,
			},
			AdmissionResponseWithProxies: AdmissionResponseWithProxies{},
		}
		req, err := makeReviewRequestFromAdmissionReview(srv.URL, admissionReview, true)
		Expect(err).NotTo(HaveOccurred())

		res, err := srv.Client().Do(req)
		Expect(err).NotTo(HaveOccurred())

		review, err := parseReviewResponse(res)
		Expect(err).NotTo(HaveOccurred())
		Expect(review.Response).NotTo(BeNil())

		if expectAllowed {
			Expect(review.Response.Allowed).To(BeTrue())
			Expect(review.Proxies).To(BeEmpty())
		} else {
			Expect(review.Response.Allowed).To(BeFalse())
			Expect(review.Response.Result).NotTo(BeNil())
			Expect(review.Response.Result.Message).To(ContainSubstring(errMsg))
			Expect(review.Proxies).To(BeEmpty())
		}
	},
		Entry("gateway create, rejected", false, v1.GatewayCrd, v1.GatewayCrd.GroupVersionKind(), v1beta1.Create, gateway),
		Entry("gateway update, accepted", true, v1.GatewayCrd, v1.GatewayCrd.GroupVersionKind(), v1beta1.Update, gateway),
		Entry("virtual service create, rejected", false, v1.VirtualServiceCrd, v1.VirtualServiceCrd.GroupVersionKind(), v1beta1.Create, vs),
		Entry("virtual service update, accepted", true, v1.VirtualServiceCrd, v1.VirtualServiceCrd.GroupVersionKind(), v1beta1.Update, vs),
		Entry("route table create, rejected", false, v1.RouteTableCrd, v1.RouteTableCrd.GroupVersionKind(), v1beta1.Create, routeTable),
		Entry("route table update, accepted", true, v1.RouteTableCrd, v1.RouteTableCrd.GroupVersionKind(), v1beta1.Update, routeTable),
		Entry("upstream create, rejected", false, gloov1.UpstreamCrd, gloov1.UpstreamCrd.GroupVersionKind(), v1beta1.Create, upstream),
		Entry("upstream update, accepted", true, gloov1.UpstreamCrd, gloov1.UpstreamCrd.GroupVersionKind(), v1beta1.Update, upstream),
	)

	DescribeTable("processes metadata updates with auto-fail validator", func(expectAllowed bool, crd crd.Crd, gvk schema.GroupVersionKind, op v1beta1.Operation, resource resources.InputResource) {
		setMockFunctions()
		resourceCrd, err := crd.KubeResource(resource)
		Expect(err).NotTo(HaveOccurred())
		raw, err := json.Marshal(resourceCrd)
		Expect(err).NotTo(HaveOccurred())

		// Ensure the oldResource only differs by a metadata change
		resourceCrd.Labels = map[string]string{
			"label": "old-resource",
		}
		oldRaw, err := json.Marshal(resourceCrd)
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
						Name:      resource.GetMetadata().GetName(),
						Namespace: resource.GetMetadata().GetNamespace(),
						Operation: op,
						Object: runtime.RawExtension{
							Raw: raw,
						},
						OldObject: runtime.RawExtension{
							Raw: oldRaw,
						},
					},
				},
				ReturnProxies: false,
			},
			AdmissionResponseWithProxies: AdmissionResponseWithProxies{},
		}
		req, err := makeReviewRequestFromAdmissionReview(srv.URL, admissionReview, true)
		Expect(err).NotTo(HaveOccurred())

		res, err := srv.Client().Do(req)
		Expect(err).NotTo(HaveOccurred())

		review, err := parseReviewResponse(res)
		Expect(err).NotTo(HaveOccurred())
		Expect(review.Response).NotTo(BeNil())

		if expectAllowed {
			Expect(review.Response.Allowed).To(BeTrue())
			Expect(review.Proxies).To(BeEmpty())
		} else {
			Expect(review.Response.Allowed).To(BeFalse())
			Expect(review.Response.Result).NotTo(BeNil())
			Expect(review.Response.Result.Message).To(ContainSubstring(errMsg))
			Expect(review.Proxies).To(BeEmpty())
		}
	},
		Entry("gateway create, rejected", false, v1.GatewayCrd, v1.GatewayCrd.GroupVersionKind(), v1beta1.Create, gateway),
		Entry("gateway update, rejected", false, v1.GatewayCrd, v1.GatewayCrd.GroupVersionKind(), v1beta1.Update, gateway),
		Entry("virtual service create, rejected", false, v1.VirtualServiceCrd, v1.VirtualServiceCrd.GroupVersionKind(), v1beta1.Create, vs),
		Entry("virtual service update, rejected", false, v1.VirtualServiceCrd, v1.VirtualServiceCrd.GroupVersionKind(), v1beta1.Update, vs),
		Entry("route table create, rejected", false, v1.RouteTableCrd, v1.RouteTableCrd.GroupVersionKind(), v1beta1.Create, routeTable),
		Entry("route table update, rejected", false, v1.RouteTableCrd, v1.RouteTableCrd.GroupVersionKind(), v1beta1.Update, routeTable),
		Entry("upstream create, rejected", false, gloov1.UpstreamCrd, gloov1.UpstreamCrd.GroupVersionKind(), v1beta1.Create, upstream),
		Entry("upstream update, rejected", false, gloov1.UpstreamCrd, gloov1.UpstreamCrd.GroupVersionKind(), v1beta1.Update, upstream),
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
			setMockFunctions()
			req, err := makeReviewRequestWithProxies(srv.URL, v1.GatewayCrd, gateway.GroupVersionKind(), v1beta1.Create, gateway, true)
			Expect(err).NotTo(HaveOccurred())

			res, err := srv.Client().Do(req)
			Expect(err).NotTo(HaveOccurred())

			review, err := parseReviewResponse(res)
			Expect(err).NotTo(HaveOccurred())
			Expect(review.Response).NotTo(BeNil())

			Expect(review.Response.Allowed).To(BeFalse())
			Expect(review.Response.Result).ToNot(BeNil())
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

	Context("custom input resource validation", func() {

		It("Validates RateLimitConfigs", func() {
			// Changes to the ratelimit api can break this test, to generate new ratelimitconfig.json:
			// 1. create a test cluster and ensure LOG_LEVEL="debug" in the gloo deployment env vars
			// 2. `kubectl apply` some new `RateLimitConfig` and ensure it gets accepted
			// 3. run `kubectl logs deploy/gloo -n gloo-system | grep "kube-apiserver-admission"` and collect the new
			//    json from the first bracket after `kube-apiserver-admission` through the end of the line

			content, err := os.ReadFile("testdata/ratelimitconfig.json")
			Expect(err).ToNot(HaveOccurred())
			url, err := url.Parse("somePlaceholderUrl")
			Expect(err).ToNot(HaveOccurred())

			req := http.Request{
				URL:    url,
				Header: map[string][]string{},
				Body:   io.NopCloser(bytes.NewBuffer(content)),
			}
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			wh.ServeHTTP(w, &req)
			data, err := io.ReadAll(w.Result().Body)
			Expect(err).ToNot(HaveOccurred())
			Expect(string(data)).To(ContainSubstring(`"allowed":true`))

			err = w.Result().Body.Close()
			Expect(err).ToNot(HaveOccurred())
		})
	})
})

func makeReviewRequest(url string, crd crd.Crd, gvk schema.GroupVersionKind, operation v1beta1.Operation, resource interface{}) *http.Request {
	req, err := makeReviewRequestWithProxies(url, crd, gvk, operation, resource, false)
	Expect(err).NotTo(HaveOccurred())
	return req
}

func makeReviewRequestWithProxies(url string, crd crd.Crd, gvk schema.GroupVersionKind, operation v1beta1.Operation, resource interface{}, returnProxies bool) (*http.Request, error) {

	if operation == v1beta1.Delete {
		ref := resource.(*core.ResourceRef)
		return makeReviewRequestRawJsonEncoded(url, gvk, operation, ref.GetName(), ref.GetNamespace(), nil, returnProxies)
	}

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

	return makeReviewRequestFromAdmissionReview(url, review, useYamlEncoding)
}

func makeReviewRequestFromAdmissionReview(url string, admissionReview AdmissionReviewWithProxies, useYamlEncoding bool) (*http.Request, error) {
	var (
		contentType string
		body        []byte
		err         error
	)
	if useYamlEncoding {
		contentType = ApplicationYaml
		body, err = yaml.Marshal(admissionReview)
	} else {
		contentType = ApplicationJson
		body, err = json.Marshal(admissionReview)
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

var _ validation.Validator = new(mockValidator)

type mockValidator struct {
	fSync                func(context.Context, *gloov1snap.ApiSnapshot) error
	fValidateList        func(ctx context.Context, ul *unstructured.UnstructuredList, dryRun bool) (*validation.Reports, *multierror.Error)
	fValidateDeleteGvk   func(ctx context.Context, gvk schema.GroupVersionKind, resource resources.Resource, dryRun bool) error
	fValidateModifiedGvk func(ctx context.Context, gvk schema.GroupVersionKind, resource resources.Resource, dryRun bool) (*validation.Reports, error)
}

func (v *mockValidator) Sync(ctx context.Context, snap *gloov1snap.ApiSnapshot) error {
	if v.fSync == nil {
		return nil
	}
	return v.fSync(ctx, snap)
}

func (v *mockValidator) ValidateDeletedGvk(ctx context.Context, gvk schema.GroupVersionKind, resource resources.Resource, dryRun bool) error {
	if v.fValidateDeleteGvk == nil {
		return nil
	}
	return v.fValidateDeleteGvk(ctx, gvk, resource, dryRun)
}

func (v *mockValidator) ValidateModifiedGvk(ctx context.Context, gvk schema.GroupVersionKind, resource resources.Resource, dryRun bool) (*validation.Reports, error) {
	if v.fValidateModifiedGvk == nil {
		return nil, nil
	}
	return v.fValidateModifiedGvk(ctx, gvk, resource, dryRun)
}

func (v *mockValidator) ValidateList(ctx context.Context, ul *unstructured.UnstructuredList, dryRun bool) (*validation.Reports, *multierror.Error) {
	if v.fValidateList == nil {
		return reports(), nil
	}
	return v.fValidateList(ctx, ul, dryRun)
}

func reports() *validation.Reports {
	return &validation.Reports{
		ProxyReports: &validation.ProxyReports{
			&validation2.ProxyReport{
				ListenerReports: nil,
			},
		},
	}
}

package k8sadmission

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/ghodss/yaml"
	"github.com/hashicorp/go-multierror"
	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/projects/gateway/pkg/validation"
	validation2 "github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/static"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
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

	errMsg := "didn't say the magic word"

	DescribeTable("accepts valid admission requests, rejects bad ones", func(valid bool, crd crd.Crd, gvk schema.GroupVersionKind, op v1beta1.Operation, resourceOrRef interface{}) {
		// not critical to these tests, but this isn't ever supposed to be null or empty.
		wh.webhookNamespace = routeTable.Metadata.Namespace

		if !valid {
			mv.fValidateList = func(ctx context.Context, ul *unstructured.UnstructuredList, dryRun bool) (*validation.Reports, *multierror.Error) {
				return reports(), &multierror.Error{Errors: []error{fmt.Errorf(errMsg)}}
			}
			mv.fValidateGateway = func(ctx context.Context, gw *v1.Gateway, dryRun bool) (*validation.Reports, error) {
				return reports(), fmt.Errorf(errMsg)
			}
			mv.fValidateVirtualService = func(ctx context.Context, vs *v1.VirtualService, dryRun bool) (*validation.Reports, error) {
				return reports(), fmt.Errorf(errMsg)
			}
			mv.fValidateDeleteVirtualService = func(ctx context.Context, vs *core.ResourceRef, dryRun bool) error {
				return fmt.Errorf(errMsg)
			}
			mv.fValidateRouteTable = func(ctx context.Context, rt *v1.RouteTable, dryRun bool) (*validation.Reports, error) {
				return reports(), fmt.Errorf(errMsg)
			}
			mv.fValidateDeleteRouteTable = func(ctx context.Context, rt *core.ResourceRef, dryRun bool) error {
				return fmt.Errorf(errMsg)
			}
			mv.fValidateUpstream = func(ctx context.Context, us *gloov1.Upstream, dryRun bool) (*validation.Reports, error) {
				return reports(), fmt.Errorf(errMsg)
			}
			mv.fValidateDeleteUpstream = func(ctx context.Context, us *core.ResourceRef, dryRun bool) error {
				return fmt.Errorf(errMsg)
			}
			mv.fValidateDeleteSecret = func(ctx context.Context, secret *core.ResourceRef, dryRun bool) error {
				return fmt.Errorf(errMsg)
			}
		}
		req, err := makeReviewRequest(srv.URL, crd, gvk, op, resourceOrRef)

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
		Entry("valid gateway", true, v1.GatewayCrd, v1.GatewayCrd.GroupVersionKind(), v1beta1.Create, gateway),
		Entry("invalid gateway", false, v1.GatewayCrd, v1.GatewayCrd.GroupVersionKind(), v1beta1.Create, gateway),
		Entry("valid virtual service", true, v1.VirtualServiceCrd, v1.VirtualServiceCrd.GroupVersionKind(), v1beta1.Create, vs),
		Entry("invalid virtual service", false, v1.VirtualServiceCrd, v1.VirtualServiceCrd.GroupVersionKind(), v1beta1.Create, vs),
		Entry("valid virtual service deletion", true, v1.VirtualServiceCrd, v1.VirtualServiceCrd.GroupVersionKind(), v1beta1.Delete, vs.GetMetadata().Ref()),
		Entry("invalid virtual service deletion", false, v1.VirtualServiceCrd, v1.VirtualServiceCrd.GroupVersionKind(), v1beta1.Delete, vs.GetMetadata().Ref()),
		Entry("valid route table", true, v1.RouteTableCrd, v1.RouteTableCrd.GroupVersionKind(), v1beta1.Create, routeTable),
		Entry("invalid route table", false, v1.RouteTableCrd, v1.RouteTableCrd.GroupVersionKind(), v1beta1.Create, routeTable),
		Entry("valid route table deletion", true, v1.RouteTableCrd, v1.RouteTableCrd.GroupVersionKind(), v1beta1.Delete, routeTable.GetMetadata().Ref()),
		Entry("invalid route table deletion", false, v1.RouteTableCrd, v1.RouteTableCrd.GroupVersionKind(), v1beta1.Delete, routeTable.GetMetadata().Ref()),
		Entry("valid unstructured list", true, nil, ListGVK, v1beta1.Create, unstructuredList),
		Entry("invalid unstructured list", false, nil, ListGVK, v1beta1.Create, unstructuredList),
		Entry("valid upstream", true, gloov1.UpstreamCrd, gloov1.UpstreamCrd.GroupVersionKind(), v1beta1.Create, upstream),
		Entry("invalid upstream", false, gloov1.UpstreamCrd, gloov1.UpstreamCrd.GroupVersionKind(), v1beta1.Create, upstream),
		Entry("valid upstream deletion", true, gloov1.UpstreamCrd, gloov1.UpstreamCrd.GroupVersionKind(), v1beta1.Delete, upstream.GetMetadata().Ref()),
		Entry("invalid upstream deletion", false, gloov1.UpstreamCrd, gloov1.UpstreamCrd.GroupVersionKind(), v1beta1.Delete, upstream.GetMetadata().Ref()),
		Entry("valid secret deletion", true, gloov1.SecretCrd, gloov1.SecretCrd.GroupVersionKind(), v1beta1.Delete, secret.GetMetadata().Ref()),
		Entry("invalid secret deletion", false, gloov1.SecretCrd, gloov1.SecretCrd.GroupVersionKind(), v1beta1.Delete, secret.GetMetadata().Ref()),
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
			mv.fValidateGateway = func(ctx context.Context, gw *v1.Gateway, dryRun bool) (*validation.Reports, error) {
				return reports(), fmt.Errorf(errMsg)
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
})

func makeReviewRequest(url string, crd crd.Crd, gvk schema.GroupVersionKind, operation v1beta1.Operation, resource interface{}) (*http.Request, error) {
	return makeReviewRequestWithProxies(url, crd, gvk, operation, resource, false)
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
func convertSnapshot(glooSnap *gloov1snap.ApiSnapshot) *v1.ApiSnapshot {
	return &v1.ApiSnapshot{
		VirtualServices:    glooSnap.VirtualServices,
		RouteTables:        glooSnap.RouteTables,
		Gateways:           glooSnap.Gateways,
		VirtualHostOptions: glooSnap.VirtualHostOptions,
		RouteOptions:       glooSnap.RouteOptions,
		HttpGateways:       glooSnap.HttpGateways,
	}
}

type mockValidator struct {
	fSync                         func(context.Context, *v1.ApiSnapshot) error
	fValidateList                 func(ctx context.Context, ul *unstructured.UnstructuredList, dryRun bool) (*validation.Reports, *multierror.Error)
	fValidateGateway              func(ctx context.Context, gw *v1.Gateway, dryRun bool) (*validation.Reports, error)
	fValidateVirtualService       func(ctx context.Context, vs *v1.VirtualService, dryRun bool) (*validation.Reports, error)
	fValidateDeleteVirtualService func(ctx context.Context, vs *core.ResourceRef, dryRun bool) error
	fValidateRouteTable           func(ctx context.Context, rt *v1.RouteTable, dryRun bool) (*validation.Reports, error)
	fValidateDeleteRouteTable     func(ctx context.Context, rt *core.ResourceRef, dryRun bool) error
	fValidateUpstream             func(ctx context.Context, us *gloov1.Upstream, dryRun bool) (*validation.Reports, error)
	fValidateDeleteUpstream       func(ctx context.Context, us *core.ResourceRef, dryRun bool) error
	fValidateDeleteSecret         func(ctx context.Context, secret *core.ResourceRef, dryRun bool) error
}

func (v *mockValidator) Sync(ctx context.Context, snap *gloov1snap.ApiSnapshot) error {
	if v.fSync == nil {
		return nil
	}
	gwsnap := convertSnapshot(snap)
	return v.fSync(ctx, gwsnap)
}

func (v *mockValidator) ValidateList(ctx context.Context, ul *unstructured.UnstructuredList, dryRun bool) (*validation.Reports, *multierror.Error) {
	if v.fValidateList == nil {
		return reports(), nil
	}
	return v.fValidateList(ctx, ul, dryRun)
}

func (v *mockValidator) ValidateGateway(ctx context.Context, gw *v1.Gateway, dryRun bool) (*validation.Reports, error) {
	if v.fValidateGateway == nil {
		return reports(), nil
	}
	return v.fValidateGateway(ctx, gw, dryRun)
}

func (v *mockValidator) ValidateVirtualService(ctx context.Context, vs *v1.VirtualService, dryRun bool) (*validation.Reports, error) {
	if v.fValidateVirtualService == nil {
		return reports(), nil
	}
	return v.fValidateVirtualService(ctx, vs, dryRun)
}

func (v *mockValidator) ValidateDeleteVirtualService(ctx context.Context, vs *core.ResourceRef, dryRun bool) error {
	if v.fValidateDeleteVirtualService == nil {
		return nil
	}
	return v.fValidateDeleteVirtualService(ctx, vs, dryRun)
}

func (v *mockValidator) ValidateRouteTable(ctx context.Context, rt *v1.RouteTable, dryRun bool) (*validation.Reports, error) {
	if v.fValidateRouteTable == nil {
		return reports(), nil
	}
	return v.fValidateRouteTable(ctx, rt, dryRun)
}

func (v *mockValidator) ValidateDeleteRouteTable(ctx context.Context, rt *core.ResourceRef, dryRun bool) error {
	if v.fValidateDeleteRouteTable == nil {
		return nil
	}
	return v.fValidateDeleteRouteTable(ctx, rt, dryRun)
}

func (v *mockValidator) ValidateUpstream(ctx context.Context, us *gloov1.Upstream, dryRun bool) (*validation.Reports, error) {
	if v.fValidateUpstream == nil {
		return reports(), nil
	}
	return v.fValidateUpstream(ctx, us, dryRun)
}

func (v *mockValidator) ValidateDeleteUpstream(ctx context.Context, us *core.ResourceRef, dryRun bool) error {
	if v.fValidateDeleteUpstream == nil {
		return nil
	}
	return v.fValidateDeleteUpstream(ctx, us, dryRun)
}

func (v *mockValidator) ValidateDeleteSecret(ctx context.Context, secret *core.ResourceRef, dryRun bool) error {
	if v.fValidateDeleteSecret == nil {
		return nil
	}
	return v.fValidateDeleteSecret(ctx, secret, dryRun)
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

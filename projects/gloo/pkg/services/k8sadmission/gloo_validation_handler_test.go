package k8sadmission

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/crd"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/errors"
	"k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	ApplicationJson = "application/json"
)

var _ = Describe("GlooValidationHandler", func() {

	var (
		server *httptest.Server
	)

	BeforeEach(func() {
		validationHandler := &GlooValidationHandler{ctx: context.TODO()}
		server = httptest.NewServer(validationHandler)
	})

	AfterEach(func() {
		server.Close()
	})

	It("Works", func() {
		req, err := makeReviewRequest(
			server.URL,
			v1.UpstreamCrd,
			v1.UpstreamCrd.GroupVersionKind(),
			v1beta1.Create,
			DefaultUpstream("default"),
		)
		Expect(err).NotTo(HaveOccurred())

		res, err := server.Client().Do(req)
		Expect(err).NotTo(HaveOccurred())

		review, err := parseReviewResponse(res)
		Expect(err).NotTo(HaveOccurred())
		Expect(review.Response).NotTo(BeNil())
		Expect(review.Response.Allowed).To(BeTrue())
	})
})

func DefaultUpstream(namespace string) *v1.Upstream {
	return &v1.Upstream{
		Metadata: &core.Metadata{
			Name:      "upstream",
			Namespace: namespace,
		},
	}
}

func parseReviewResponse(resp *http.Response) (*v1beta1.AdmissionReview, error) {
	var review v1beta1.AdmissionReview
	if err := json.NewDecoder(resp.Body).Decode(&review); err != nil {
		return nil, err
	}
	return &review, nil
}

func makeReviewRequest(url string, crd crd.Crd, gvk schema.GroupVersionKind, operation v1beta1.Operation, resource interface{}) (*http.Request, error) {
	switch typedResource := resource.(type) {
	case unstructured.UnstructuredList:
		jsonBytes, err := typedResource.MarshalJSON()
		Expect(err).To(BeNil())
		return makeReviewRequestRaw(url, gvk, operation, "name", "namespace", jsonBytes)
	case resources.InputResource:
		resourceCrd, err := crd.KubeResource(typedResource)
		if err != nil {
			return nil, err
		}

		raw, err := json.Marshal(resourceCrd)
		if err != nil {
			return nil, err
		}
		return makeReviewRequestRaw(url, gvk, operation, typedResource.GetMetadata().Name, typedResource.GetMetadata().Namespace, raw)
	default:
		Fail("unknown type")
	}

	return nil, errors.Errorf("unknown type")
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

	req.Header.Set("Content-type", ApplicationJson)
	return req, nil
}

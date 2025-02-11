//go:build ignore

package matchers_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	crdv1 "github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/crd/solo.io/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	v1 "github.com/kgateway-dev/kgateway/v2/internal/gloo/pkg/api/v1"
	gloov1 "github.com/kgateway-dev/kgateway/v2/internal/gloo/pkg/api/v1/kube/apis/gloo.solo.io/v1"
	"github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
)

var _ = Describe("CustomResource", func() {

	Context("MatchObjectMeta", func() {

		It("matches with name/namespace", func() {
			objMeta := metav1.ObjectMeta{
				Name:      "name",
				Namespace: "namespace",
			}
			Expect(objMeta).To(matchers.MatchObjectMeta(types.NamespacedName{
				Name:      "name",
				Namespace: "namespace",
			}))
		})

		It("does not match with name/namespace", func() {
			objMeta := metav1.ObjectMeta{
				Name:      "name",
				Namespace: "namespace",
			}
			Expect(objMeta).NotTo(matchers.MatchObjectMeta(types.NamespacedName{
				Name:      "name-mismatch",
				Namespace: "namespace-mismatch",
			}))
		})

		It("supports additional matchers", func() {
			objMeta := metav1.ObjectMeta{
				Name:      "name",
				Namespace: "namespace",
			}
			Expect(objMeta).NotTo(matchers.MatchObjectMeta(types.NamespacedName{
				Name:      "name",
				Namespace: "namespace",
			}, gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
				"ResourceVersion": Equal("resource-version"),
			})))
		})

	})

	Context("HaveNilManagedFields", func() {

		It("matches when ManagedFields are nil", func() {
			objMeta := metav1.ObjectMeta{
				Name:          "name",
				Namespace:     "namespace",
				ManagedFields: nil,
			}
			Expect(objMeta).To(matchers.MatchObjectMeta(types.NamespacedName{
				Name:      "name",
				Namespace: "namespace",
			}, matchers.HaveNilManagedFields()))
		})

		It("does not match when MangedFields are non-nil", func() {
			objMeta := metav1.ObjectMeta{
				Name:      "name",
				Namespace: "namespace",
				ManagedFields: []metav1.ManagedFieldsEntry{
					{
						Manager: "manager",
					},
				},
			}
			Expect(objMeta).NotTo(matchers.MatchObjectMeta(types.NamespacedName{
				Name:      "name",
				Namespace: "namespace",
			}, matchers.HaveNilManagedFields()))
		})
	})

	Context("MatchTypeMeta", func() {

		It("matches gvk", func() {
			typeMeta := metav1.TypeMeta{
				Kind:       "kind",
				APIVersion: "test.solo.io/v1alpha1",
			}
			Expect(typeMeta).To(matchers.MatchTypeMeta(schema.GroupVersionKind{
				Group:   "test.solo.io",
				Version: "v1alpha1",
				Kind:    "kind",
			}))
		})

		It("does not match partial gvk", func() {
			typeMeta := metav1.TypeMeta{
				Kind:       "kind",
				APIVersion: "test.solo.io/v1alpha1",
			}
			Expect(typeMeta).NotTo(matchers.MatchTypeMeta(schema.GroupVersionKind{
				Group:   "test.solo.io",
				Version: "v1alpha1",
				Kind:    "mismatched-kind",
			}))
		})

	})

	Context("MatchCustomResource", func() {

		It("matches entire resource", func() {

			obj := crdv1.Resource{
				TypeMeta: metav1.TypeMeta{
					Kind:       "kind",
					APIVersion: "test.solo.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "name",
					Namespace: "namespace",
				},
				Spec: &crdv1.Spec{
					"key": "value",
				},
			}

			Expect(obj).To(matchers.MatchCustomResource(
				matchers.MatchTypeMeta(schema.GroupVersionKind{
					Group:   "test.solo.io",
					Version: "v1alpha1",
					Kind:    "kind",
				}),
				matchers.MatchObjectMeta(types.NamespacedName{
					Name:      "name",
					Namespace: "namespace",
				}),
				gstruct.PointTo(HaveKeyWithValue("key", "value")),
			))

		})

	})

	Context("HaveNameAndNamespace", func() {

		It("matches resource purely on name and namespace", func() {
			proxy := &gloov1.Proxy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "name",
					Namespace: "namespace",
					// We add extra properties to demonstrate that they do not impact the matching
					ManagedFields: []metav1.ManagedFieldsEntry{
						{
							Manager: "manager",
						},
					},
				},
				Spec: v1.Proxy{
					Listeners: make([]*v1.Listener, 0),
				},
			}
			Expect(proxy).To(matchers.HaveNameAndNamespace("name", "namespace"),
				"only name and namespace are considered for match")
		})
	})

})

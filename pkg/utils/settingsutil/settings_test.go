package settingsutil

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	external_kubernetes_namespace "github.com/solo-io/solo-kit/api/external/kubernetes/namespace"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/common/kubernetes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Settings", func() {

	It("should store settings in ctx", func() {
		settings := &v1.Settings{}
		ctx := context.Background()

		ctx = WithSettings(ctx, settings)
		expectedSettings := MaybeFromContext(ctx)
		Expect(expectedSettings).To(Equal(settings))
	})

	It("should return nil when no settings", func() {
		ctx := context.Background()

		expectedSettings := MaybeFromContext(ctx)
		Expect(expectedSettings).To(BeNil())
	})

	It("should not when no settings with MaybeFromContext", func() {
		ctx := context.Background()
		settings := MaybeFromContext(ctx)
		Expect(settings).To(BeNil())
	})

	It("should return true for IsAllNamespacesFromSettings when WatchNamespaces is empty", func() {
		settings := &v1.Settings{
			WatchNamespaces: []string{},
		}

		Expect(IsAllNamespacesFromSettings(settings)).To(BeTrue())
	})

	It("should return true for IsAllNamespacesFromSettings when WatchNamespaces only has empty namespace", func() {
		settings := &v1.Settings{
			WatchNamespaces: []string{""},
		}

		Expect(IsAllNamespacesFromSettings(settings)).To(BeTrue())
	})

	It("should return false for IsAllNamespacesFromSettings when WatchNamespaces has a namespace", func() {
		settings := &v1.Settings{
			WatchNamespaces: []string{"test"},
		}

		Expect(IsAllNamespacesFromSettings(settings)).To(BeFalse())
	})

	It("should add the discovery namespace to the list of namespaces to watch if not present", func() {
		settings := &v1.Settings{
			WatchNamespaces: []string{"test"},
		}

		Expect(GetNamespacesToWatch(settings)).To(Equal([]string{"test", "gloo-system"}))
	})

	Context("Determining namespaces to watch", func() {
		var settings *v1.Settings
		var matchedNamespace *kubernetes.KubeNamespace
		var skippedNamespace *kubernetes.KubeNamespace
		var namespaces kubernetes.KubeNamespaceList

		BeforeEach(func() {
			matchedNamespace = &kubernetes.KubeNamespace{
				KubeNamespace: external_kubernetes_namespace.KubeNamespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "match-me",
						Labels: map[string]string{
							"match":  "me",
							"common": "label",
						},
					},
				},
			}
			skippedNamespace = &kubernetes.KubeNamespace{
				KubeNamespace: external_kubernetes_namespace.KubeNamespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "skip-me",
						Labels: map[string]string{
							"skip":   "me",
							"common": "label",
						},
					},
				},
			}
			namespaces = kubernetes.KubeNamespaceList{
				matchedNamespace,
				skippedNamespace,
			}

			settings = &v1.Settings{
				WatchNamespaceSelectors: []*v1.LabelSelector{
					{
						MatchLabels: map[string]string{
							"match": "me",
						},
						MatchExpressions: []*v1.LabelSelectorRequirement{
							{
								Key:      "common",
								Operator: "In",
								Values:   []string{"label"},
							},
						},
					},
				},
			}

		})

		When("calling GenerateNamespacesToWatch", func() {
			It("should return watchNamespaces if defined", func() {
				settings.WatchNamespaces = []string{"test"}
				Expect(GenerateNamespacesToWatch(settings, namespaces)).To(Equal([]string{"test", "gloo-system"}))
			})

			It("should return nil if watchNamespaces and watchNamespaceSelector are not defined", func() {
				settings = &v1.Settings{}
				Expect(GenerateNamespacesToWatch(settings, namespaces)).To(BeNil())
			})

			It("should return namespaces that match watchNamespaceSelector", func() {
				Expect(GenerateNamespacesToWatch(settings, namespaces)).To(Equal([]string{"gloo-system", "match-me"}))
			})
		})

		When("calling NamespaceWatched", func() {
			It("should return true if the namespace should be watched", func() {
				Expect(NamespaceWatched(settings, *matchedNamespace)).To(BeTrue())
			})

			It("should return false if the namespace should not be watched", func() {
				Expect(NamespaceWatched(settings, *skippedNamespace)).To(BeFalse())
			})
		})

		When("calling UpdateNamespacesToWatch", func() {
			It("should return the correct value if the namespaces to watch list has changed", func() {
				Expect(UpdateNamespacesToWatch(settings, namespaces)).To(BeTrue())

				// Running it again with the same settings should not result in a change
				Expect(UpdateNamespacesToWatch(settings, namespaces)).To(BeFalse())
			})
		})
	})
})

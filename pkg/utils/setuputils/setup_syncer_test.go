package setuputils_test

import (
	"context"

	"github.com/solo-io/gloo/pkg/bootstrap/leaderelector"
	"github.com/solo-io/gloo/pkg/bootstrap/leaderelector/singlereplica"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	external_kubernetes_namespace "github.com/solo-io/solo-kit/api/external/kubernetes/namespace"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/common/kubernetes"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	. "github.com/solo-io/gloo/pkg/utils/setuputils"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("SetupSyncer", func() {
	It("calls the setup function with the referenced settings crd", func() {
		var actualSettings *v1.Settings
		expectedSettings := &v1.Settings{
			Metadata: &core.Metadata{Name: "hello", Namespace: "goodbye"},
		}
		setupSyncer := NewSetupSyncer(
			expectedSettings.Metadata.Ref(),
			func(
				ctx context.Context,
				kubeCache kube.SharedCache,
				inMemoryCache memory.InMemoryResourceCache,
				settings *v1.Settings,
				identity leaderelector.Identity) error {
				actualSettings = expectedSettings
				return nil
			},
			singlereplica.Identity())
		err := setupSyncer.Sync(context.TODO(), &v1.SetupSnapshot{
			Settings: v1.SettingsList{expectedSettings},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(actualSettings).To(Equal(expectedSettings))
	})

	When("calling ShouldSync", func() {
		var snapshot *v1.SetupSnapshot
		var settings *v1.Settings
		var namespaces kubernetes.KubeNamespaceList
		var setupSyncer *SetupSyncer

		BeforeEach(func() {
			labelSelectors := []*v1.LabelSelector{
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
			}
			settings = &v1.Settings{
				Metadata:                &core.Metadata{Name: "hello", Namespace: "goodbye"},
				WatchNamespaceSelectors: labelSelectors,
				DiscoveryNamespace:      "old",
			}

			matchedNamespace := &kubernetes.KubeNamespace{
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
			skippedNamespace := &kubernetes.KubeNamespace{
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

			snapshot = &v1.SetupSnapshot{
				Settings:       v1.SettingsList{settings},
				Kubenamespaces: namespaces,
			}

			setupSyncer = NewSetupSyncer(
				settings.Metadata.Ref(),
				func(
					ctx context.Context,
					kubeCache kube.SharedCache,
					inMemoryCache memory.InMemoryResourceCache,
					settings *v1.Settings,
					identity leaderelector.Identity) error {
					return nil
				},
				singlereplica.Identity())
		})

		BeforeEach(func() {
			// Handle the initial check where the previous settings hash is nil
			setupSyncer.Sync(context.TODO(), snapshot)
		})

		It("return false if nothing has changed", func() {
			newSnapshot := snapshot.Clone()
			Expect(setupSyncer.ShouldSync(context.TODO(), snapshot, &newSnapshot)).To(BeFalse())
		})

		It("return true if the settings object has changed", func() {
			// Ensure only the settings has changed
			newSnapshot := snapshot.Clone()
			newSnapshot.Settings[0].DiscoveryNamespace = "new"
			Expect(setupSyncer.ShouldSync(context.TODO(), snapshot, &newSnapshot)).To(BeTrue())
		})

		When("The kubenamespaces has changed", func() {
			It("A new namespace to watch has been added", func() {
				newSnapshot := snapshot.Clone()
				newSnapshot.Kubenamespaces = append(newSnapshot.Kubenamespaces, &kubernetes.KubeNamespace{
					KubeNamespace: external_kubernetes_namespace.KubeNamespace{
						ObjectMeta: metav1.ObjectMeta{
							Name: "match-me-again",
							Labels: map[string]string{
								"match":   "me",
								"common":  "label",
								"matched": "again",
							},
						},
					},
				})
				Expect(setupSyncer.ShouldSync(context.TODO(), snapshot, &newSnapshot)).To(BeTrue())
			})

			It("A namespace to watch has been deleted", func() {
				newSnapshot := snapshot.Clone()
				newSnapshot.Kubenamespaces = kubernetes.KubeNamespaceList{newSnapshot.Kubenamespaces[1]}
				Expect(setupSyncer.ShouldSync(context.TODO(), snapshot, &newSnapshot)).To(BeTrue())
			})

			It("A namespace to watch has been modified to unwatch it", func() {
				newSnapshot := snapshot.Clone()
				newSnapshot.Kubenamespaces[0].Labels = nil
				Expect(setupSyncer.ShouldSync(context.TODO(), snapshot, &newSnapshot)).To(BeTrue())
			})

			It("A namespace to watch has been modified that doesn't affect how we watch it", func() {
				newSnapshot := snapshot.Clone()
				newSnapshot.Kubenamespaces[0].Labels["random"] = "label"
				Expect(setupSyncer.ShouldSync(context.TODO(), snapshot, &newSnapshot)).To(BeFalse())
			})

			It("A namespace to has been added but we don't need to watch it", func() {
				newSnapshot := snapshot.Clone()
				newSnapshot.Kubenamespaces = append(newSnapshot.Kubenamespaces, &kubernetes.KubeNamespace{
					KubeNamespace: external_kubernetes_namespace.KubeNamespace{
						ObjectMeta: metav1.ObjectMeta{
							Name: "skip-me-again",
							Labels: map[string]string{
								"skip":    "me",
								"common":  "label",
								"skipped": "again",
							},
						},
					},
				})
				Expect(setupSyncer.ShouldSync(context.TODO(), snapshot, &newSnapshot)).To(BeFalse())
			})

			It("A namespace to has been deleted but we don't watch it", func() {
				newSnapshot := snapshot.Clone()
				newSnapshot.Kubenamespaces = kubernetes.KubeNamespaceList{newSnapshot.Kubenamespaces[0]}
				Expect(setupSyncer.ShouldSync(context.TODO(), snapshot, &newSnapshot)).To(BeFalse())
			})
		})
	})
})

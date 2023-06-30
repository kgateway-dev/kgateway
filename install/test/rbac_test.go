package test

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/format"
	. "github.com/solo-io/k8s-utils/manifesttestutils"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var _ = Describe("RBAC Test", func() {
	format.MaxLength = 10000000
	var allTests = func(testCase renderTestCase) {
		Describe(testCase.rendererName, func() {
			var (
				testManifest    TestManifest
				resourceBuilder ResourceBuilder
			)

			prepareMakefile := func(helmFlags ...string) {
				tm, err := testCase.renderer.RenderManifest(namespace, helmValues{
					valuesArgs: append([]string{}, helmFlags...),
				})
				Expect(err).NotTo(HaveOccurred(), "Should be able to render the manifest in the RBAC unit test")
				testManifest = tm
			}

			ExpectDiscoveryNotInRoleBindingSubjects := func(roleBindingName string) {
				selectedResources := testManifest.SelectResources(func(resource *unstructured.Unstructured) bool {
					return resource.GetKind() == "RoleBinding" && resource.GetName() == roleBindingName
				})
				Expect(selectedResources.NumResources()).ToNot(BeZero(), fmt.Sprintf("could not find expected role binding: %s", roleBindingName))
				selectedResources.ExpectAll(func(resource *unstructured.Unstructured) {
					roleBinding := makeRoleBindingFromUnstructured(resource)
					for _, subject := range roleBinding.Subjects {
						Expect(subject.Name).To(Not(Equal("discovery")), "disabled discovery service should not be bound in %s", roleBinding.Name)
					}
				})
			}

			ExpectDiscoveryNotInClusterRoleBindingSubjects := func(clusterRoleBindingName string) {
				selectedResources := testManifest.SelectResources(func(resource *unstructured.Unstructured) bool {
					return resource.GetKind() == "ClusterRoleBinding" && resource.GetName() == clusterRoleBindingName
				})
				Expect(selectedResources.NumResources()).ToNot(BeZero(), fmt.Sprintf("could not find expected cluster role binding: %s", clusterRoleBindingName))
				selectedResources.ExpectAll(func(resource *unstructured.Unstructured) {
					clusterRoleBinding := makeClusterRoleBindingFromUnstructured(resource)
					for _, subject := range clusterRoleBinding.Subjects {
						Expect(subject.Name).To(Not(Equal("discovery")), "disabled discovery service should not be bound in %s", clusterRoleBinding.Name)
					}
				})
			}

			Context("implementation-agnostic permissions", func() {
				BeforeEach(func() {
					format.MaxLength = 0
				})
				It("correctly assigns permissions for single-namespace gloo", func() {
					prepareMakefile("namespace.create=true", "global.glooRbac.namespaced=true")
					permissions := GetServiceAccountPermissions("gloo-system")
					testManifest.ExpectPermissions(permissions)
				})

				It("correctly assigns permissions for cluster-scoped gloo", func() {
					prepareMakefile("namespace.create=true", "global.glooRbac.namespaced=false")
					permissions := GetServiceAccountPermissions("")
					testManifest.ExpectPermissions(permissions)
				})

				It("creates no permissions when rbac is disabled", func() {
					prepareMakefile("global.glooRbac.create=false")
					testManifest.ExpectAll(func(resource *unstructured.Unstructured) {
						Expect(resource.GetAPIVersion()).NotTo(ContainSubstring("rbac.authorization.k8s.io"), "Should not contain the RBAC API group")
					})
				})
			})

			Context("all cluster-scoped RBAC resources", func() {
				checkSuffix := func(suffix string) {
					rbacResources := testManifest.SelectResources(func(resource *unstructured.Unstructured) bool {
						return resource.GetKind() == "ClusterRole" || resource.GetKind() == "ClusterRoleBinding"
					})

					Expect(rbacResources.NumResources()).NotTo(BeZero())

					rbacResources.ExpectAll(func(resource *unstructured.Unstructured) {
						Expect(resource.GetName()).To(HaveSuffix("-" + suffix))
					})
				}

				It("is all named appropriately when a custom suffix is specified", func() {
					suffix := "test-suffix"
					prepareMakefile("global.glooRbac.nameSuffix=" + suffix)
					checkSuffix(suffix)
				})

				It("is all named appropriately in a non-namespaced install", func() {
					// be sure to pass these flags here so that all RBAC resources are rendered in the template
					prepareMakefile("ingress.enabled=true", "settings.integrations.knative.enabled=true")
					checkSuffix(namespace)
				})
			})

			Context("kube-resource-watcher", func() {
				BeforeEach(func() {
					resourceBuilder = ResourceBuilder{
						Name: "kube-resource-watcher",
						Labels: map[string]string{
							"app":  "gloo",
							"gloo": "rbac",
						},
						Rules: []rbacv1.PolicyRule{
							{
								APIGroups: []string{""},
								Resources: []string{"pods", "services", "secrets", "endpoints", "configmaps", "namespaces"},
								Verbs:     []string{"get", "list", "watch"},
							},
						},
						RoleRef: rbacv1.RoleRef{
							APIGroup: "rbac.authorization.k8s.io",
							Kind:     "ClusterRole",
							Name:     "kube-resource-watcher",
						},
						Subjects: []rbacv1.Subject{{
							Kind:      "ServiceAccount",
							Name:      "gloo",
							Namespace: namespace,
						}, {
							Kind:      "ServiceAccount",
							Name:      "discovery",
							Namespace: namespace,
						}},
					}
				})
				Context("cluster scope", func() {
					It("role", func() {
						resourceBuilder.Name += "-" + namespace
						prepareMakefile("global.glooRbac.namespaced=false")
						testManifest.ExpectClusterRole(resourceBuilder.GetClusterRole())
					})

					It("role binding", func() {
						resourceBuilder.Name += "-binding-" + namespace
						resourceBuilder.RoleRef.Name += "-" + namespace
						prepareMakefile("global.glooRbac.namespaced=false")
						testManifest.ExpectClusterRoleBinding(resourceBuilder.GetClusterRoleBinding())
					})

					It("disabling discovery removes its service account from cluster role binding", func() {
						resourceBuilder.Name += "-binding-" + namespace
						resourceBuilder.RoleRef.Name += "-" + namespace
						prepareMakefile("global.glooRbac.namespaced=false", "discovery.enabled=false")
						ExpectDiscoveryNotInClusterRoleBindingSubjects(resourceBuilder.GetClusterRoleBinding().GetName())
					})
				})
				Context("namespace scope", func() {
					BeforeEach(func() {
						resourceBuilder.RoleRef.Kind = "Role"
						resourceBuilder.Namespace = namespace
					})

					It("role", func() {
						prepareMakefile("global.glooRbac.namespaced=true")
						testManifest.ExpectRole(resourceBuilder.GetRole())
					})

					It("role binding", func() {
						resourceBuilder.Name += "-binding"
						prepareMakefile("global.glooRbac.namespaced=true")
						testManifest.ExpectRoleBinding(resourceBuilder.GetRoleBinding())
					})

					It("disabling discovery removes its service account from role binding", func() {
						resourceBuilder.Name += "-binding"
						prepareMakefile("global.glooRbac.namespaced=true", "discovery.enabled=false")
						ExpectDiscoveryNotInRoleBindingSubjects(resourceBuilder.GetRoleBinding().GetName())
					})
				})
			})

			Context("gloo-upstream-mutator", func() {
				BeforeEach(func() {
					resourceBuilder = ResourceBuilder{
						Name: "gloo-upstream-mutator",
						Labels: map[string]string{
							"app":  "gloo",
							"gloo": "rbac",
						},
						Rules: []rbacv1.PolicyRule{
							{
								APIGroups: []string{"gloo.solo.io"},
								Resources: []string{"upstreams"},
								Verbs:     []string{"get", "list", "watch", "create", "update", "patch", "delete"},
							},
						},
						RoleRef: rbacv1.RoleRef{
							APIGroup: "rbac.authorization.k8s.io",
							Kind:     "ClusterRole",
							Name:     "gloo-upstream-mutator",
						},
						Subjects: []rbacv1.Subject{{
							Kind:      "ServiceAccount",
							Name:      "discovery",
							Namespace: namespace,
						}},
					}
				})
				Context("cluster scope", func() {
					It("role", func() {
						resourceBuilder.Name += "-" + namespace
						prepareMakefile("global.glooRbac.namespaced=false")
						testManifest.ExpectClusterRole(resourceBuilder.GetClusterRole())
					})

					It("role binding", func() {
						resourceBuilder.Name += "-binding-" + namespace
						resourceBuilder.RoleRef.Name += "-" + namespace
						prepareMakefile("global.glooRbac.namespaced=false")
						testManifest.ExpectClusterRoleBinding(resourceBuilder.GetClusterRoleBinding())
					})

					It("disabling discovery removes its service account from cluster role binding", func() {
						resourceBuilder.Name += "-binding-" + namespace
						resourceBuilder.RoleRef.Name += "-" + namespace
						prepareMakefile("global.glooRbac.namespaced=false", "discovery.enabled=false")
						ExpectDiscoveryNotInClusterRoleBindingSubjects(resourceBuilder.GetClusterRoleBinding().GetName())
					})
				})
				Context("namespace scope", func() {
					BeforeEach(func() {
						resourceBuilder.RoleRef.Kind = "Role"
						resourceBuilder.Namespace = namespace
					})

					It("role", func() {
						prepareMakefile("global.glooRbac.namespaced=true")
						testManifest.ExpectRole(resourceBuilder.GetRole())
					})

					It("role binding", func() {
						resourceBuilder.Name += "-binding"
						prepareMakefile("global.glooRbac.namespaced=true")
						testManifest.ExpectRoleBinding(resourceBuilder.GetRoleBinding())
					})

					It("disabling discovery removes its service account from role binding", func() {
						resourceBuilder.Name += "-binding"
						prepareMakefile("global.glooRbac.namespaced=true", "discovery.enabled=false")
						ExpectDiscoveryNotInRoleBindingSubjects(resourceBuilder.GetRoleBinding().GetName())
					})
				})
			})

			Context("gloo-resource-reader", func() {
				BeforeEach(func() {
					resourceBuilder = ResourceBuilder{
						Name: "gloo-resource-reader",
						Labels: map[string]string{
							"app":  "gloo",
							"gloo": "rbac",
						},
						Rules: []rbacv1.PolicyRule{
							{
								APIGroups: []string{"gloo.solo.io"},
								Resources: []string{"upstreams", "upstreamgroups", "proxies"},
								Verbs:     []string{"get", "list", "watch", "patch"},
							},
							{
								APIGroups: []string{"enterprise.gloo.solo.io"},
								Resources: []string{"authconfigs"},
								Verbs:     []string{"get", "list", "watch", "patch"},
							},
							{
								APIGroups: []string{"ratelimit.solo.io"},
								Resources: []string{"ratelimitconfigs", "ratelimitconfigs/status"},
								Verbs:     []string{"get", "list", "watch", "patch", "update"},
							},
							{
								APIGroups: []string{"graphql.gloo.solo.io"},
								Resources: []string{"graphqlapis", "graphqlapis/status"},
								Verbs:     []string{"get", "list", "watch", "patch", "update"},
							},
						},
						RoleRef: rbacv1.RoleRef{
							APIGroup: "rbac.authorization.k8s.io",
							Kind:     "ClusterRole",
							Name:     "gloo-resource-reader",
						},
						Subjects: []rbacv1.Subject{{
							Kind:      "ServiceAccount",
							Name:      "gloo",
							Namespace: namespace,
						}},
					}
				})
				Context("cluster scope", func() {
					It("role", func() {
						resourceBuilder.Name += "-" + namespace
						prepareMakefile("global.glooRbac.namespaced=false")
						testManifest.ExpectClusterRole(resourceBuilder.GetClusterRole())
					})

					It("role binding", func() {
						resourceBuilder.Name += "-binding-" + namespace
						resourceBuilder.RoleRef.Name += "-" + namespace
						prepareMakefile("global.glooRbac.namespaced=false")
						testManifest.ExpectClusterRoleBinding(resourceBuilder.GetClusterRoleBinding())
					})
				})
				Context("namespace scope", func() {
					BeforeEach(func() {
						resourceBuilder.RoleRef.Kind = "Role"
						resourceBuilder.Namespace = namespace
					})

					It("role", func() {
						prepareMakefile("global.glooRbac.namespaced=true")
						testManifest.ExpectRole(resourceBuilder.GetRole())
					})

					It("role binding", func() {
						resourceBuilder.Name += "-binding"
						prepareMakefile("global.glooRbac.namespaced=true")
						testManifest.ExpectRoleBinding(resourceBuilder.GetRoleBinding())
					})
				})
			})

			Context("kube-leader-election", func() {
				BeforeEach(func() {
					resourceBuilder = ResourceBuilder{
						Name: "kube-leader-election",
						Labels: map[string]string{
							"app":  "gloo",
							"gloo": "rbac",
						},
						Rules: []rbacv1.PolicyRule{
							{
								APIGroups: []string{"coordination.k8s.io"},
								Resources: []string{"leases"},
								Verbs:     []string{"*"},
							},
							{
								APIGroups: []string{""},
								Resources: []string{"configmaps"},
								Verbs:     []string{"*"},
							},
						},
						RoleRef: rbacv1.RoleRef{
							APIGroup: "rbac.authorization.k8s.io",
							Kind:     "ClusterRole",
							Name:     "kube-leader-election",
						},
						Subjects: []rbacv1.Subject{
							{
								Kind:      "ServiceAccount",
								Name:      "gloo",
								Namespace: namespace,
							},
							{
								Kind:      "ServiceAccount",
								Name:      "discovery",
								Namespace: namespace,
							},
						},
					}
				})
				Context("cluster scope", func() {
					It("role", func() {
						resourceBuilder.Name += "-" + namespace
						prepareMakefile("global.glooRbac.namespaced=false")
						testManifest.ExpectClusterRole(resourceBuilder.GetClusterRole())
					})

					It("role binding", func() {
						resourceBuilder.Name += "-binding-" + namespace
						resourceBuilder.RoleRef.Name += "-" + namespace
						prepareMakefile("global.glooRbac.namespaced=false")
						testManifest.ExpectClusterRoleBinding(resourceBuilder.GetClusterRoleBinding())
					})
				})
				Context("namespace scope", func() {
					BeforeEach(func() {
						resourceBuilder.RoleRef.Kind = "Role"
						resourceBuilder.Namespace = namespace
					})

					It("role", func() {
						prepareMakefile("global.glooRbac.namespaced=true")
						testManifest.ExpectRole(resourceBuilder.GetRole())
					})

					It("role binding", func() {
						resourceBuilder.Name += "-binding"
						prepareMakefile("global.glooRbac.namespaced=true")
						testManifest.ExpectRoleBinding(resourceBuilder.GetRoleBinding())
					})
				})
			})

			Context("gloo-graphqlapi-mutator", func() {
				BeforeEach(func() {
					resourceBuilder = ResourceBuilder{
						Name: "gloo-graphqlapi-mutator",
						Labels: map[string]string{
							"app":  "gloo",
							"gloo": "rbac",
						},
						Rules: []rbacv1.PolicyRule{
							{
								APIGroups: []string{"graphql.gloo.solo.io"},
								Resources: []string{"graphqlapis", "graphqlapis/status"},
								Verbs:     []string{"get", "list", "watch", "update", "patch", "create"},
							},
						},
						RoleRef: rbacv1.RoleRef{
							APIGroup: "rbac.authorization.k8s.io",
							Kind:     "ClusterRole",
							Name:     "gloo-graphqlapi-mutator",
						},
						Subjects: []rbacv1.Subject{{
							Kind:      "ServiceAccount",
							Name:      "discovery",
							Namespace: namespace,
						}},
					}
				})
				Context("cluster scope", func() {
					It("role", func() {
						resourceBuilder.Name += "-" + namespace
						prepareMakefile("global.glooRbac.namespaced=false")
						testManifest.ExpectClusterRole(resourceBuilder.GetClusterRole())
					})

					It("role binding", func() {
						resourceBuilder.Name += "-binding-" + namespace
						resourceBuilder.RoleRef.Name += "-" + namespace
						prepareMakefile("global.glooRbac.namespaced=false")
						testManifest.ExpectClusterRoleBinding(resourceBuilder.GetClusterRoleBinding())
					})

					It("disabling discovery removes its service account from cluster role binding", func() {
						resourceBuilder.Name += "-binding-" + namespace
						resourceBuilder.RoleRef.Name += "-" + namespace
						prepareMakefile("global.glooRbac.namespaced=false", "discovery.enabled=false")
						ExpectDiscoveryNotInClusterRoleBindingSubjects(resourceBuilder.GetClusterRoleBinding().GetName())
					})
				})
				Context("namespace scope", func() {
					BeforeEach(func() {
						resourceBuilder.RoleRef.Kind = "Role"
						resourceBuilder.Namespace = namespace
					})

					It("role", func() {
						prepareMakefile("global.glooRbac.namespaced=true")
						testManifest.ExpectRole(resourceBuilder.GetRole())
					})

					It("role binding", func() {
						resourceBuilder.Name += "-binding"
						prepareMakefile("global.glooRbac.namespaced=true")
						testManifest.ExpectRoleBinding(resourceBuilder.GetRoleBinding())
					})

					It("disabling discovery removes its service account from role binding", func() {
						resourceBuilder.Name += "-binding"
						prepareMakefile("global.glooRbac.namespaced=true", "discovery.enabled=false")
						ExpectDiscoveryNotInRoleBindingSubjects(resourceBuilder.GetRoleBinding().GetName())
					})
				})
			})

			Context("settings-user", func() {
				BeforeEach(func() {
					resourceBuilder = ResourceBuilder{
						Name: "settings-user",
						Labels: map[string]string{
							"app":  "gloo",
							"gloo": "rbac",
						},
						Rules: []rbacv1.PolicyRule{
							{
								APIGroups: []string{"gloo.solo.io"},
								Resources: []string{"settings"},
								Verbs:     []string{"get", "list", "watch"},
							},
						},
						RoleRef: rbacv1.RoleRef{
							APIGroup: "rbac.authorization.k8s.io",
							Kind:     "ClusterRole",
							Name:     "settings-user",
						},
						Subjects: []rbacv1.Subject{{
							Kind:      "ServiceAccount",
							Name:      "gloo",
							Namespace: namespace,
						}, {
							Kind:      "ServiceAccount",
							Name:      "gateway",
							Namespace: namespace,
						}, {
							Kind:      "ServiceAccount",
							Name:      "discovery",
							Namespace: namespace,
						}},
					}
				})
				Context("cluster scope", func() {
					It("role", func() {
						resourceBuilder.Name += "-" + namespace
						prepareMakefile("global.glooRbac.namespaced=false")
						testManifest.ExpectClusterRole(resourceBuilder.GetClusterRole())
					})

					It("role binding", func() {
						resourceBuilder.Name += "-binding-" + namespace
						resourceBuilder.RoleRef.Name += "-" + namespace
						prepareMakefile("global.glooRbac.namespaced=false")
						testManifest.ExpectClusterRoleBinding(resourceBuilder.GetClusterRoleBinding())
					})

					It("disabling discovery removes its service account from cluster role binding", func() {
						resourceBuilder.Name += "-binding-" + namespace
						resourceBuilder.RoleRef.Name += "-" + namespace
						prepareMakefile("global.glooRbac.namespaced=false", "discovery.enabled=false")
						ExpectDiscoveryNotInClusterRoleBindingSubjects(resourceBuilder.GetClusterRoleBinding().GetName())
					})
				})
				Context("namespace scope", func() {
					BeforeEach(func() {
						resourceBuilder.RoleRef.Kind = "Role"
						resourceBuilder.Namespace = namespace
					})

					It("role", func() {
						prepareMakefile("global.glooRbac.namespaced=true")
						testManifest.ExpectRole(resourceBuilder.GetRole())
					})

					It("role binding", func() {
						resourceBuilder.Name += "-binding"
						prepareMakefile("global.glooRbac.namespaced=true")
						testManifest.ExpectRoleBinding(resourceBuilder.GetRoleBinding())
					})

					It("disabling discovery removes its service account from role binding", func() {
						resourceBuilder.Name += "-binding"
						prepareMakefile("global.glooRbac.namespaced=true", "discovery.enabled=false")
						ExpectDiscoveryNotInRoleBindingSubjects(resourceBuilder.GetRoleBinding().GetName())
					})
				})
			})

			Context("certgen job", func() {
				It("Cluster scope", func() {
					prepareMakefile("global.glooRbac.namespaced=false")
					By("roles", func() {
						testManifest.ExpectClusterRole(&rbacv1.ClusterRole{
							TypeMeta: metav1.TypeMeta{
								Kind:       "ClusterRole",
								APIVersion: "rbac.authorization.k8s.io/v1",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name: "gloo-gateway-secret-create-gloo-system",
								Labels: map[string]string{
									"app":  "gloo",
									"gloo": "rbac",
								},
								Annotations: map[string]string{
									"helm.sh/hook-weight": "5",
									"helm.sh/hook":        "pre-install,pre-upgrade",
								},
							},
							Rules: []rbacv1.PolicyRule{
								{
									Verbs:           []string{"create", "get", "update"},
									APIGroups:       []string{""},
									Resources:       []string{"secrets"},
									ResourceNames:   nil,
									NonResourceURLs: nil,
								}},
							AggregationRule: nil,
						})
						testManifest.ExpectClusterRole(&rbacv1.ClusterRole{
							TypeMeta: metav1.TypeMeta{
								Kind:       "ClusterRole",
								APIVersion: "rbac.authorization.k8s.io/v1",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name: "gloo-gateway-vwc-update-gloo-system",
								Labels: map[string]string{
									"app":  "gloo",
									"gloo": "rbac",
								},
								Annotations: map[string]string{
									"helm.sh/hook-weight": "5",
									"helm.sh/hook":        "pre-install,pre-upgrade",
								},
							},
							Rules: []rbacv1.PolicyRule{
								{
									Verbs:           []string{"get", "update"},
									APIGroups:       []string{"admissionregistration.k8s.io"},
									Resources:       []string{"validatingwebhookconfigurations"},
									ResourceNames:   nil,
									NonResourceURLs: nil,
								}},
							AggregationRule: nil,
						})
					})
					By("role bindings", func() {
						testManifest.ExpectClusterRoleBinding(&rbacv1.ClusterRoleBinding{
							TypeMeta: metav1.TypeMeta{
								Kind:       "ClusterRoleBinding",
								APIVersion: "rbac.authorization.k8s.io/v1",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name: "gloo-gateway-secret-create-gloo-system",
								Labels: map[string]string{
									"app":  "gloo",
									"gloo": "rbac",
								},
								Annotations: map[string]string{
									"helm.sh/hook-weight": "5",
									"helm.sh/hook":        "pre-install,pre-upgrade",
								},
							},
							Subjects: []rbacv1.Subject{{
								Kind:      "ServiceAccount",
								APIGroup:  "",
								Name:      "certgen",
								Namespace: "gloo-system",
							}},
							RoleRef: rbacv1.RoleRef{
								APIGroup: "rbac.authorization.k8s.io",
								Kind:     "ClusterRole",
								Name:     "gloo-gateway-secret-create-gloo-system",
							},
						})
						testManifest.ExpectClusterRoleBinding(&rbacv1.ClusterRoleBinding{
							TypeMeta: metav1.TypeMeta{
								Kind:       "ClusterRoleBinding",
								APIVersion: "rbac.authorization.k8s.io/v1",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name: "gloo-gateway-vwc-update-gloo-system",
								Labels: map[string]string{
									"app":  "gloo",
									"gloo": "rbac",
								},
								Annotations: map[string]string{
									"helm.sh/hook-weight": "5",
									"helm.sh/hook":        "pre-install,pre-upgrade",
								},
							},
							Subjects: []rbacv1.Subject{{
								Kind:      "ServiceAccount",
								APIGroup:  "",
								Name:      "certgen",
								Namespace: "gloo-system",
							}},
							RoleRef: rbacv1.RoleRef{
								APIGroup: "rbac.authorization.k8s.io",
								Kind:     "ClusterRole",
								Name:     "gloo-gateway-vwc-update-gloo-system",
							},
						})
					})
				})
				It("Namespace scope", func() {
					prepareMakefile("global.glooRbac.namespaced=true")
					By("roles", func() {
						testManifest.ExpectRole(&rbacv1.Role{
							TypeMeta: metav1.TypeMeta{
								Kind:       "Role",
								APIVersion: "rbac.authorization.k8s.io/v1",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name:      "gloo-gateway-secret-create",
								Namespace: "gloo-system",
								Labels: map[string]string{
									"app":  "gloo",
									"gloo": "rbac",
								},
								Annotations: map[string]string{
									"helm.sh/hook-weight": "5",
									"helm.sh/hook":        "pre-install,pre-upgrade",
								},
							},
							Rules: []rbacv1.PolicyRule{{
								Verbs:           []string{"create", "get", "update"},
								APIGroups:       []string{""},
								Resources:       []string{"secrets"},
								ResourceNames:   nil,
								NonResourceURLs: nil,
							}},
						})
						testManifest.ExpectClusterRole(&rbacv1.ClusterRole{
							TypeMeta: metav1.TypeMeta{
								Kind:       "ClusterRole",
								APIVersion: "rbac.authorization.k8s.io/v1",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name: "gloo-gateway-vwc-update",
								Labels: map[string]string{
									"app":  "gloo",
									"gloo": "rbac",
								},
								Annotations: map[string]string{
									"helm.sh/hook-weight": "5",
									"helm.sh/hook":        "pre-install,pre-upgrade",
								},
							},
							Rules: []rbacv1.PolicyRule{
								{
									Verbs:           []string{"get", "update"},
									APIGroups:       []string{"admissionregistration.k8s.io"},
									Resources:       []string{"validatingwebhookconfigurations"},
									ResourceNames:   nil,
									NonResourceURLs: nil,
								}},
							AggregationRule: nil,
						})
					})
					By("role bindings", func() {
						testManifest.ExpectRoleBinding(&rbacv1.RoleBinding{
							TypeMeta: metav1.TypeMeta{
								Kind:       "RoleBinding",
								APIVersion: "rbac.authorization.k8s.io/v1",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name:      "gloo-gateway-secret-create",
								Namespace: "gloo-system",
								Labels: map[string]string{
									"app":  "gloo",
									"gloo": "rbac",
								},
								Annotations: map[string]string{
									"helm.sh/hook-weight": "5",
									"helm.sh/hook":        "pre-install,pre-upgrade",
								},
							},
							Subjects: []rbacv1.Subject{{
								Kind:      "ServiceAccount",
								APIGroup:  "",
								Name:      "certgen",
								Namespace: "gloo-system",
							}},
							RoleRef: rbacv1.RoleRef{
								APIGroup: "rbac.authorization.k8s.io",
								Kind:     "Role",
								Name:     "gloo-gateway-secret-create",
							},
						})
						testManifest.ExpectClusterRoleBinding(&rbacv1.ClusterRoleBinding{
							TypeMeta: metav1.TypeMeta{
								Kind:       "ClusterRoleBinding",
								APIVersion: "rbac.authorization.k8s.io/v1",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name: "gloo-gateway-vwc-update",
								Labels: map[string]string{
									"app":  "gloo",
									"gloo": "rbac",
								},
								Annotations: map[string]string{
									"helm.sh/hook-weight": "5",
									"helm.sh/hook":        "pre-install,pre-upgrade",
								},
							},
							Subjects: []rbacv1.Subject{{
								Kind:      "ServiceAccount",
								APIGroup:  "",
								Name:      "certgen",
								Namespace: "gloo-system",
							}},
							RoleRef: rbacv1.RoleRef{
								APIGroup: "rbac.authorization.k8s.io",
								Kind:     "ClusterRole",
								Name:     "gloo-gateway-vwc-update",
							},
						})
					})
				})
			})

			Context("gloo-resource-mutator", func() {
				BeforeEach(func() {
					resourceBuilder = ResourceBuilder{
						Name: "gloo-resource-mutator",
						Labels: map[string]string{
							"app":  "gloo",
							"gloo": "rbac",
						},
						Rules: []rbacv1.PolicyRule{
							{
								APIGroups: []string{"gloo.solo.io"},
								Resources: []string{"proxies"},
								Verbs:     []string{"get", "list", "watch", "create", "update", "patch", "delete"},
							},
						},
						RoleRef: rbacv1.RoleRef{
							APIGroup: "rbac.authorization.k8s.io",
							Kind:     "ClusterRole",
							Name:     "gloo-resource-mutator",
						},
						Subjects: []rbacv1.Subject{{
							Kind:      "ServiceAccount",
							Name:      "gateway",
							Namespace: namespace,
						},
							{
								Kind:      "ServiceAccount",
								Name:      "gloo",
								Namespace: namespace,
							},
						},
					}
				})
				Context("cluster scope", func() {
					It("role", func() {
						resourceBuilder.Name += "-" + namespace
						prepareMakefile("global.glooRbac.namespaced=false")
						testManifest.ExpectClusterRole(resourceBuilder.GetClusterRole())
					})

					It("role binding", func() {
						resourceBuilder.Name += "-binding-" + namespace
						resourceBuilder.RoleRef.Name += "-" + namespace
						prepareMakefile("global.glooRbac.namespaced=false")
						testManifest.ExpectClusterRoleBinding(resourceBuilder.GetClusterRoleBinding())
					})
				})
				Context("namespace scope", func() {
					BeforeEach(func() {
						resourceBuilder.RoleRef.Kind = "Role"
						resourceBuilder.Namespace = namespace
					})

					It("role", func() {
						prepareMakefile("global.glooRbac.namespaced=true")
						testManifest.ExpectRole(resourceBuilder.GetRole())
					})

					It("role binding", func() {
						resourceBuilder.Name += "-binding"
						prepareMakefile("global.glooRbac.namespaced=true")
						testManifest.ExpectRoleBinding(resourceBuilder.GetRoleBinding())
					})
				})
			})

			Context("gateway-resource-reader", func() {
				BeforeEach(func() {
					resourceBuilder = ResourceBuilder{
						Name: "gateway-resource-reader",
						Labels: map[string]string{
							"app":  "gloo",
							"gloo": "rbac",
						},
						Rules: []rbacv1.PolicyRule{
							{
								APIGroups: []string{"gateway.solo.io"},
								Resources: []string{"gateways", "httpgateways", "tcpgateways", "virtualservices", "routetables", "virtualhostoptions", "routeoptions"},
								Verbs:     []string{"get", "list", "watch", "patch"},
							},
						},
						RoleRef: rbacv1.RoleRef{
							APIGroup: "rbac.authorization.k8s.io",
							Kind:     "ClusterRole",
							Name:     "gateway-resource-reader",
						},
						Subjects: []rbacv1.Subject{
							{
								Kind:      "ServiceAccount",
								Name:      "gateway",
								Namespace: namespace,
							},
							{
								Kind:      "ServiceAccount",
								Name:      "gloo",
								Namespace: namespace,
							},
						},
					}
				})
				Context("cluster scope", func() {
					It("role", func() {
						resourceBuilder.Name += "-" + namespace
						prepareMakefile("global.glooRbac.namespaced=false")
						testManifest.ExpectClusterRole(resourceBuilder.GetClusterRole())
					})

					It("role binding", func() {
						resourceBuilder.Name += "-binding-" + namespace
						resourceBuilder.RoleRef.Name += "-" + namespace
						prepareMakefile("global.glooRbac.namespaced=false")
						testManifest.ExpectClusterRoleBinding(resourceBuilder.GetClusterRoleBinding())
					})
				})
				Context("namespace scope", func() {
					BeforeEach(func() {
						resourceBuilder.RoleRef.Kind = "Role"
						resourceBuilder.Namespace = namespace
					})

					It("role", func() {
						prepareMakefile("global.glooRbac.namespaced=true")
						testManifest.ExpectRole(resourceBuilder.GetRole())
					})

					It("role binding", func() {
						resourceBuilder.Name += "-binding"
						prepareMakefile("global.glooRbac.namespaced=true")
						testManifest.ExpectRoleBinding(resourceBuilder.GetRoleBinding())
					})
				})
			})
		})
	}

	runTests(allTests)
})

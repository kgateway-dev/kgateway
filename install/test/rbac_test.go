package test

import (
	. "github.com/onsi/ginkgo"
	rbacv1 "k8s.io/api/rbac/v1"

	. "github.com/solo-io/go-utils/manifesttestutils"
)

var _ = Describe("RBAC Test", func() {
	var (
		testManifest    TestManifest
		resourceBuilder ResourceBuilder
	)

	prepareMakefile := func(helmFlags string) {
		testManifest = renderManifest(helmFlags)
	}

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
						Resources: []string{"pods", "services", "secrets", "endpoints", "configmaps"},
						Verbs:     []string{"get", "list", "watch"},
					},
					{
						APIGroups: []string{""},
						Resources: []string{"namespaces"},
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
				prepareMakefile("--namespace " + namespace + " --set namespace.create=true --set rbac.namespaced=false")
				testManifest.ExpectClusterRole(resourceBuilder.GetClusterRole())
			})

			It("role binding", func() {
				resourceBuilder.Name += "-binding-" + namespace
				prepareMakefile("--namespace " + namespace + " --set namespace.create=true --set rbac.namespaced=false")
				testManifest.ExpectClusterRoleBinding(resourceBuilder.GetClusterRoleBinding())
			})
		})
		Context("namespace scope", func() {
			BeforeEach(func() {
				resourceBuilder.RoleRef.Kind = "Role"
				resourceBuilder.Namespace = namespace
			})

			It("role", func() {
				prepareMakefile("--namespace " + namespace + " --set namespace.create=true --set rbac.namespaced=true")
				testManifest.ExpectRole(resourceBuilder.GetRole())
			})

			It("role binding", func() {
				resourceBuilder.Name += "-binding"
				prepareMakefile("--namespace " + namespace + " --set namespace.create=true --set rbac.namespaced=true")
				testManifest.ExpectRoleBinding(resourceBuilder.GetRoleBinding())
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
						Resources: []string{"settings", "upstreams", "upstreamgroups", "proxies"},
						Verbs:     []string{"get", "update"},
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
				prepareMakefile("--namespace " + namespace + " --set namespace.create=true --set rbac.namespaced=false")
				testManifest.ExpectClusterRole(resourceBuilder.GetClusterRole())
			})

			It("role binding", func() {
				resourceBuilder.Name += "-binding-" + namespace
				prepareMakefile("--namespace " + namespace + " --set namespace.create=true --set rbac.namespaced=false")
				testManifest.ExpectClusterRoleBinding(resourceBuilder.GetClusterRoleBinding())
			})
		})
		Context("namespace scope", func() {
			BeforeEach(func() {
				resourceBuilder.RoleRef.Kind = "Role"
				resourceBuilder.Namespace = namespace
			})

			It("role", func() {
				prepareMakefile("--namespace " + namespace + " --set namespace.create=true --set rbac.namespaced=true")
				testManifest.ExpectRole(resourceBuilder.GetRole())
			})

			It("role binding", func() {
				resourceBuilder.Name += "-binding"
				prepareMakefile("--namespace " + namespace + " --set namespace.create=true --set rbac.namespaced=true")
				testManifest.ExpectRoleBinding(resourceBuilder.GetRoleBinding())
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
						Verbs:     []string{"*"},
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
				}},
			}
		})
		Context("cluster scope", func() {
			It("role", func() {
				prepareMakefile("--namespace " + namespace + " --set namespace.create=true --set rbac.namespaced=false")
				testManifest.ExpectClusterRole(resourceBuilder.GetClusterRole())
			})

			It("role binding", func() {
				resourceBuilder.Name += "-binding-" + namespace
				prepareMakefile("--namespace " + namespace + " --set namespace.create=true --set rbac.namespaced=false")
				testManifest.ExpectClusterRoleBinding(resourceBuilder.GetClusterRoleBinding())
			})
		})
		Context("namespace scope", func() {
			BeforeEach(func() {
				resourceBuilder.RoleRef.Kind = "Role"
				resourceBuilder.Namespace = namespace
			})

			It("role", func() {
				prepareMakefile("--namespace " + namespace + " --set namespace.create=true --set rbac.namespaced=true")
				testManifest.ExpectRole(resourceBuilder.GetRole())
			})

			It("role binding", func() {
				resourceBuilder.Name += "-binding"
				prepareMakefile("--namespace " + namespace + " --set namespace.create=true --set rbac.namespaced=true")
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
						Resources: []string{"virtualservices"},
						Verbs:     []string{"get", "update"},
					}, {
						APIGroups: []string{"gateway.solo.io"},
						Resources: []string{"gateways"},
						Verbs:     []string{"get", "create", "update"},
					}, {
						APIGroups: []string{"gateway.solo.io.v2"},
						Resources: []string{"gateways"},
						Verbs:     []string{"get", "create", "update"},
					},
				},
				RoleRef: rbacv1.RoleRef{
					APIGroup: "rbac.authorization.k8s.io",
					Kind:     "ClusterRole",
					Name:     "gateway-resource-reader",
				},
				Subjects: []rbacv1.Subject{{
					Kind:      "ServiceAccount",
					Name:      "gateway",
					Namespace: namespace,
				}},
			}
		})
		Context("cluster scope", func() {
			It("role", func() {
				prepareMakefile("--namespace " + namespace + " --set namespace.create=true --set rbac.namespaced=false")
				testManifest.ExpectClusterRole(resourceBuilder.GetClusterRole())
			})

			It("role binding", func() {
				resourceBuilder.Name += "-binding-" + namespace
				prepareMakefile("--namespace " + namespace + " --set namespace.create=true --set rbac.namespaced=false")
				testManifest.ExpectClusterRoleBinding(resourceBuilder.GetClusterRoleBinding())
			})
		})
		Context("namespace scope", func() {
			BeforeEach(func() {
				resourceBuilder.RoleRef.Kind = "Role"
				resourceBuilder.Namespace = namespace
			})

			It("role", func() {
				prepareMakefile("--namespace " + namespace + " --set namespace.create=true --set rbac.namespaced=true")
				testManifest.ExpectRole(resourceBuilder.GetRole())
			})

			It("role binding", func() {
				resourceBuilder.Name += "-binding"
				prepareMakefile("--namespace " + namespace + " --set namespace.create=true --set rbac.namespaced=true")
				testManifest.ExpectRoleBinding(resourceBuilder.GetRoleBinding())
			})
		})
	})

})

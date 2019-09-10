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

	Context("implementation-agnostic permissions", func() {
		var permissions *ServiceAccountPermissions
		BeforeEach(func() {
			prepareMakefile("--namespace " + namespace + " --set namespace.create=true --set global.glooRbac.namespaced=false")
			permissions = &ServiceAccountPermissions{}

			// Apiserver
			permissions.AddExpectedPermission(
				"gloo-system.apiserver-ui",
				"gloo-system",
				[]string{""},
				[]string{"pods", "services", "configmaps", "namespaces", "secrets"},
				[]string{"get", "list", "watch"})
			permissions.AddExpectedPermission(
				"gloo-system.apiserver-ui",
				"gloo-system",
				[]string{"apiextensions.k8s.io"},
				[]string{"customresourcedefinitions"},
				[]string{"get"})
			permissions.AddExpectedPermission(
				"gloo-system.apiserver-ui",
				"gloo-system",
				[]string{"gloo.solo.io"},
				[]string{"artifacts", "settings", "upstreams", "upstreamgroups", "proxies", "secrets"},
				[]string{"get", "list", "watch"})
			permissions.AddExpectedPermission(
				"gloo-system.apiserver-ui",
				"gloo-system",
				[]string{"gateway.solo.io.v2"},
				[]string{"gateways"},
				[]string{"get", "list", "watch"})
			permissions.AddExpectedPermission(
				"gloo-system.apiserver-ui",
				"gloo-system",
				[]string{"gateway.solo.io"},
				[]string{"virtualservices"},
				[]string{"get", "list", "watch"})

			// Gateway
			permissions.AddExpectedPermission(
				"gloo-system.gateway",
				"gloo-system",
				[]string{"gloo.solo.io"},
				[]string{"settings"},
				[]string{"get", "list", "watch", "create"})
			permissions.AddExpectedPermission(
				"gloo-system.gateway",
				"gloo-system",
				[]string{"gloo.solo.io"},
				[]string{"proxies"},
				[]string{"get", "list", "watch", "create", "update", "delete"})
			permissions.AddExpectedPermission(
				"gloo-system.gateway",
				"gloo-system",
				[]string{"gateway.solo.io.v2"},
				[]string{"gateways"},
				[]string{"get", "list", "watch", "create", "update"})
			permissions.AddExpectedPermission(
				"gloo-system.gateway",
				"gloo-system",
				[]string{"gateway.solo.io"},
				[]string{"gateways"},
				[]string{"get", "list", "watch", "create", "update"})
			permissions.AddExpectedPermission(
				"gloo-system.gateway",
				"gloo-system",
				[]string{"gateway.solo.io"},
				[]string{"virtualservices", "routetables"},
				[]string{"get", "list", "watch", "update"})

			// Gloo
			permissions.AddExpectedPermission(
				"gloo-system.gloo",
				"gloo-system",
				[]string{""},
				[]string{"pods", "services", "configmaps", "namespaces", "secrets", "endpoints"},
				[]string{"get", "list", "watch"})
			permissions.AddExpectedPermission(
				"gloo-system.gloo",
				"gloo-system",
				[]string{"gloo.solo.io"},
				[]string{"upstreams", "upstreamgroups", "proxies"},
				[]string{"get", "list", "watch", "update"})
			permissions.AddExpectedPermission(
				"gloo-system.gloo",
				"gloo-system",
				[]string{"gloo.solo.io"},
				[]string{"settings"},
				[]string{"get", "list", "watch", "create"})

			// Discovery
			permissions.AddExpectedPermission(
				"gloo-system.discovery",
				"gloo-system",
				[]string{""},
				[]string{"pods", "services", "configmaps", "namespaces", "secrets", "endpoints"},
				[]string{"get", "list", "watch"})
			permissions.AddExpectedPermission(
				"gloo-system.discovery",
				"gloo-system",
				[]string{"gloo.solo.io"},
				[]string{"settings"},
				[]string{"get", "list", "watch", "create"})
			permissions.AddExpectedPermission(
				"gloo-system.discovery",
				"gloo-system",
				[]string{"gloo.solo.io"},
				[]string{"upstream"},
				[]string{"get", "list", "watch", "create", "update", "delete"})
		})

		It("are correctly configured for all service accounts", func() {
			testManifest.ExpectPermissions(permissions)
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
				Annotations: map[string]string{"helm.sh/hook": "pre-install", "helm.sh/hook-weight": "10"},
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
				prepareMakefile("--namespace " + namespace + " --set namespace.create=true --set global.glooRbac.namespaced=false")
				testManifest.ExpectClusterRole(resourceBuilder.GetClusterRole())
			})

			It("role binding", func() {
				resourceBuilder.Name += "-binding-" + namespace
				resourceBuilder.Annotations["helm.sh/hook-weight"] = "15"
				prepareMakefile("--namespace " + namespace + " --set namespace.create=true --set global.glooRbac.namespaced=false")
				testManifest.ExpectClusterRoleBinding(resourceBuilder.GetClusterRoleBinding())
			})
		})
		Context("namespace scope", func() {
			BeforeEach(func() {
				resourceBuilder.RoleRef.Kind = "Role"
				resourceBuilder.Namespace = namespace
			})

			It("role", func() {
				prepareMakefile("--namespace " + namespace + " --set namespace.create=true --set global.glooRbac.namespaced=true")
				testManifest.ExpectRole(resourceBuilder.GetRole())
			})

			It("role binding", func() {
				resourceBuilder.Name += "-binding"
				resourceBuilder.Annotations["helm.sh/hook-weight"] = "15"
				prepareMakefile("--namespace " + namespace + " --set namespace.create=true --set global.glooRbac.namespaced=true")
				testManifest.ExpectRoleBinding(resourceBuilder.GetRoleBinding())
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
				Annotations: map[string]string{"helm.sh/hook": "pre-install", "helm.sh/hook-weight": "10"},
				Rules: []rbacv1.PolicyRule{
					{
						APIGroups: []string{"gloo.solo.io"},
						Resources: []string{"upstreams"},
						Verbs:     []string{"get", "list", "watch", "create", "update", "delete"},
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
				prepareMakefile("--namespace " + namespace + " --set namespace.create=true --set global.glooRbac.namespaced=false")
				testManifest.ExpectClusterRole(resourceBuilder.GetClusterRole())
			})

			It("role binding", func() {
				resourceBuilder.Name += "-binding-" + namespace
				resourceBuilder.Annotations["helm.sh/hook-weight"] = "15"
				prepareMakefile("--namespace " + namespace + " --set namespace.create=true --set global.glooRbac.namespaced=false")
				testManifest.ExpectClusterRoleBinding(resourceBuilder.GetClusterRoleBinding())
			})
		})
		Context("namespace scope", func() {
			BeforeEach(func() {
				resourceBuilder.RoleRef.Kind = "Role"
				resourceBuilder.Namespace = namespace
			})

			It("role", func() {
				prepareMakefile("--namespace " + namespace + " --set namespace.create=true --set global.glooRbac.namespaced=true")
				testManifest.ExpectRole(resourceBuilder.GetRole())
			})

			It("role binding", func() {
				resourceBuilder.Name += "-binding"
				resourceBuilder.Annotations["helm.sh/hook-weight"] = "15"
				prepareMakefile("--namespace " + namespace + " --set namespace.create=true --set global.glooRbac.namespaced=true")
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
				Annotations: map[string]string{"helm.sh/hook": "pre-install", "helm.sh/hook-weight": "10"},
				Rules: []rbacv1.PolicyRule{
					{
						APIGroups: []string{"gloo.solo.io"},
						Resources: []string{"upstreams", "upstreamgroups", "proxies"},
						Verbs:     []string{"get", "list", "watch", "update"},
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
				prepareMakefile("--namespace " + namespace + " --set namespace.create=true --set global.glooRbac.namespaced=false")
				testManifest.ExpectClusterRole(resourceBuilder.GetClusterRole())
			})

			It("role binding", func() {
				resourceBuilder.Name += "-binding-" + namespace
				resourceBuilder.Annotations["helm.sh/hook-weight"] = "15"
				prepareMakefile("--namespace " + namespace + " --set namespace.create=true --set global.glooRbac.namespaced=false")
				testManifest.ExpectClusterRoleBinding(resourceBuilder.GetClusterRoleBinding())
			})
		})
		Context("namespace scope", func() {
			BeforeEach(func() {
				resourceBuilder.RoleRef.Kind = "Role"
				resourceBuilder.Namespace = namespace
			})

			It("role", func() {
				prepareMakefile("--namespace " + namespace + " --set namespace.create=true --set global.glooRbac.namespaced=true")
				testManifest.ExpectRole(resourceBuilder.GetRole())
			})

			It("role binding", func() {
				resourceBuilder.Name += "-binding"
				resourceBuilder.Annotations["helm.sh/hook-weight"] = "15"
				prepareMakefile("--namespace " + namespace + " --set namespace.create=true --set global.glooRbac.namespaced=true")
				testManifest.ExpectRoleBinding(resourceBuilder.GetRoleBinding())
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
				Annotations: map[string]string{"helm.sh/hook": "pre-install", "helm.sh/hook-weight": "10"},
				Rules: []rbacv1.PolicyRule{
					{
						APIGroups: []string{"gloo.solo.io"},
						Resources: []string{"settings"},
						Verbs:     []string{"get", "list", "watch", "create"},
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
				prepareMakefile("--namespace " + namespace + " --set namespace.create=true --set global.glooRbac.namespaced=false")
				testManifest.ExpectClusterRole(resourceBuilder.GetClusterRole())
			})

			It("role binding", func() {
				resourceBuilder.Name += "-binding-" + namespace
				resourceBuilder.Annotations["helm.sh/hook-weight"] = "15"
				prepareMakefile("--namespace " + namespace + " --set namespace.create=true --set global.glooRbac.namespaced=false")
				testManifest.ExpectClusterRoleBinding(resourceBuilder.GetClusterRoleBinding())
			})
		})
		Context("namespace scope", func() {
			BeforeEach(func() {
				resourceBuilder.RoleRef.Kind = "Role"
				resourceBuilder.Namespace = namespace
			})

			It("role", func() {
				prepareMakefile("--namespace " + namespace + " --set namespace.create=true --set global.glooRbac.namespaced=true")
				testManifest.ExpectRole(resourceBuilder.GetRole())
			})

			It("role binding", func() {
				resourceBuilder.Name += "-binding"
				resourceBuilder.Annotations["helm.sh/hook-weight"] = "15"
				prepareMakefile("--namespace " + namespace + " --set namespace.create=true --set global.glooRbac.namespaced=true")
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
				Annotations: map[string]string{"helm.sh/hook": "pre-install", "helm.sh/hook-weight": "10"},
				Rules: []rbacv1.PolicyRule{
					{
						APIGroups: []string{"gloo.solo.io"},
						Resources: []string{"proxies"},
						Verbs:     []string{"get", "list", "watch", "create", "update", "delete"},
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
				prepareMakefile("--namespace " + namespace + " --set namespace.create=true --set global.glooRbac.namespaced=false")
				testManifest.ExpectClusterRole(resourceBuilder.GetClusterRole())
			})

			It("role binding", func() {
				resourceBuilder.Name += "-binding-" + namespace
				resourceBuilder.Annotations["helm.sh/hook-weight"] = "15"
				prepareMakefile("--namespace " + namespace + " --set namespace.create=true --set global.glooRbac.namespaced=false")
				testManifest.ExpectClusterRoleBinding(resourceBuilder.GetClusterRoleBinding())
			})
		})
		Context("namespace scope", func() {
			BeforeEach(func() {
				resourceBuilder.RoleRef.Kind = "Role"
				resourceBuilder.Namespace = namespace
			})

			It("role", func() {
				prepareMakefile("--namespace " + namespace + " --set namespace.create=true --set global.glooRbac.namespaced=true")
				testManifest.ExpectRole(resourceBuilder.GetRole())
			})

			It("role binding", func() {
				resourceBuilder.Name += "-binding"
				resourceBuilder.Annotations["helm.sh/hook-weight"] = "15"
				prepareMakefile("--namespace " + namespace + " --set namespace.create=true --set global.glooRbac.namespaced=true")
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
				Annotations: map[string]string{"helm.sh/hook": "pre-install", "helm.sh/hook-weight": "10"},
				Rules: []rbacv1.PolicyRule{
					{
						APIGroups: []string{"gateway.solo.io"},
						Resources: []string{"virtualservices", "routetables"},
						Verbs:     []string{"get", "list", "watch", "update"},
					}, {
						APIGroups: []string{"gateway.solo.io"},
						Resources: []string{"gateways"},
						Verbs:     []string{"get", "list", "watch", "create", "update"},
					}, {
						APIGroups: []string{"gateway.solo.io.v2"},
						Resources: []string{"gateways"},
						Verbs:     []string{"get", "list", "watch", "create", "update"},
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
				prepareMakefile("--namespace " + namespace + " --set namespace.create=true --set global.glooRbac.namespaced=false")
				testManifest.ExpectClusterRole(resourceBuilder.GetClusterRole())
			})

			It("role binding", func() {
				resourceBuilder.Name += "-binding-" + namespace
				resourceBuilder.Annotations["helm.sh/hook-weight"] = "15"
				prepareMakefile("--namespace " + namespace + " --set namespace.create=true --set global.glooRbac.namespaced=false")
				testManifest.ExpectClusterRoleBinding(resourceBuilder.GetClusterRoleBinding())
			})
		})
		Context("namespace scope", func() {
			BeforeEach(func() {
				resourceBuilder.RoleRef.Kind = "Role"
				resourceBuilder.Namespace = namespace
			})

			It("role", func() {
				prepareMakefile("--namespace " + namespace + " --set namespace.create=true --set global.glooRbac.namespaced=true")
				testManifest.ExpectRole(resourceBuilder.GetRole())
			})

			It("role binding", func() {
				resourceBuilder.Name += "-binding"
				resourceBuilder.Annotations["helm.sh/hook-weight"] = "15"
				prepareMakefile("--namespace " + namespace + " --set namespace.create=true --set global.glooRbac.namespaced=true")
				testManifest.ExpectRoleBinding(resourceBuilder.GetRoleBinding())
			})
		})
	})

})

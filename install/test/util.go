package test

import "github.com/solo-io/go-utils/manifesttestutils"

func GetServiceAccountPermissions(namespace string) *manifesttestutils.ServiceAccountPermissions {
	permissions := &manifesttestutils.ServiceAccountPermissions{}

	// Apiserver
	permissions.AddExpectedPermission(
		"gloo-system.apiserver-ui",
		namespace,
		[]string{""},
		[]string{"pods", "services", "configmaps", "namespaces", "secrets"},
		[]string{"get", "list", "watch"})
	permissions.AddExpectedPermission(
		"gloo-system.apiserver-ui",
		namespace,
		[]string{"apiextensions.k8s.io"},
		[]string{"customresourcedefinitions"},
		[]string{"get"})
	permissions.AddExpectedPermission(
		"gloo-system.apiserver-ui",
		namespace,
		[]string{"gloo.solo.io"},
		[]string{"artifacts", "upstreams", "upstreamgroups", "proxies", "secrets"},
		[]string{"get", "list", "watch"})
	permissions.AddExpectedPermission(
		"gloo-system.apiserver-ui",
		namespace,
		[]string{"gloo.solo.io"},
		[]string{"settings"},
		[]string{"get", "list", "watch", "create"})
	permissions.AddExpectedPermission(
		"gloo-system.apiserver-ui",
		namespace,
		[]string{"gateway.solo.io.v2"},
		[]string{"gateways"},
		[]string{"get", "list", "watch"})
	permissions.AddExpectedPermission(
		"gloo-system.apiserver-ui",
		namespace,
		[]string{"gateway.solo.io"},
		[]string{"virtualservices"},
		[]string{"get", "list", "watch"})

	// Gateway
	permissions.AddExpectedPermission(
		"gloo-system.gateway",
		namespace,
		[]string{"gloo.solo.io"},
		[]string{"settings"},
		[]string{"get", "list", "watch", "create"})
	permissions.AddExpectedPermission(
		"gloo-system.gateway",
		namespace,
		[]string{"gloo.solo.io"},
		[]string{"proxies"},
		[]string{"get", "list", "watch", "create", "update", "delete"})
	permissions.AddExpectedPermission(
		"gloo-system.gateway",
		namespace,
		[]string{"gateway.solo.io.v2"},
		[]string{"gateways"},
		[]string{"get", "list", "watch", "create", "update"})
	permissions.AddExpectedPermission(
		"gloo-system.gateway",
		namespace,
		[]string{"gateway.solo.io"},
		[]string{"gateways"},
		[]string{"get", "list", "watch", "create", "update"})
	permissions.AddExpectedPermission(
		"gloo-system.gateway",
		namespace,
		[]string{"gateway.solo.io"},
		[]string{"virtualservices", "routetables"},
		[]string{"get", "list", "watch", "update"})

	// Gloo
	permissions.AddExpectedPermission(
		"gloo-system.gloo",
		namespace,
		[]string{""},
		[]string{"pods", "services", "configmaps", "namespaces", "secrets", "endpoints"},
		[]string{"get", "list", "watch"})
	permissions.AddExpectedPermission(
		"gloo-system.gloo",
		namespace,
		[]string{"gloo.solo.io"},
		[]string{"upstreams", "upstreamgroups", "proxies"},
		[]string{"get", "list", "watch", "update"})
	permissions.AddExpectedPermission(
		"gloo-system.gloo",
		namespace,
		[]string{"gloo.solo.io"},
		[]string{"settings"},
		[]string{"get", "list", "watch", "create"})

	// Discovery
	permissions.AddExpectedPermission(
		"gloo-system.discovery",
		namespace,
		[]string{""},
		[]string{"pods", "services", "configmaps", "namespaces", "secrets", "endpoints"},
		[]string{"get", "list", "watch"})
	permissions.AddExpectedPermission(
		"gloo-system.discovery",
		namespace,
		[]string{"gloo.solo.io"},
		[]string{"settings"},
		[]string{"get", "list", "watch", "create"})
	permissions.AddExpectedPermission(
		"gloo-system.discovery",
		namespace,
		[]string{"gloo.solo.io"},
		[]string{"upstreams"},
		[]string{"get", "list", "watch", "create", "update", "delete"})

	return permissions
}

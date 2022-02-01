package test

import "github.com/solo-io/k8s-utils/manifesttestutils"

func GetServiceAccountPermissions(namespace string) *manifesttestutils.ServiceAccountPermissions {
	permissions := &manifesttestutils.ServiceAccountPermissions{}

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
		[]string{"gateway.solo.io"},
		[]string{"gateways", "httpgateways", "virtualservices", "routetables", "virtualhostoptions", "routeoptions"},
		[]string{"get", "list", "watch", "update"})
	permissions.AddExpectedPermission(
		"gloo-system.gateway",
		namespace,
		[]string{"gateway.solo.io"},
		[]string{"gateways"},
		[]string{"create"})

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
		[]string{""},
		[]string{"configmaps"},
		[]string{"get", "update"},
	)
	permissions.AddExpectedPermission(
		"gloo-system.gloo",
		namespace,
		[]string{"gloo.solo.io"},
		[]string{"upstreams", "upstreamgroups", "proxies"},
		[]string{"get", "list", "watch", "update"})
	permissions.AddExpectedPermission(
		"gloo-system.gloo",
		namespace,
		[]string{"gateway.solo.io"},
		[]string{"gateways", "httpgateways", "virtualservices", "routetables", "virtualhostoptions", "routeoptions"},
		[]string{"get", "list", "watch", "update"})
	permissions.AddExpectedPermission(
		"gloo-system.gloo",
		namespace,
		[]string{"gateway.solo.io"},
		[]string{"gateways"},
		[]string{"create"})
	permissions.AddExpectedPermission(
		"gloo-system.gloo",
		namespace,
		[]string{"gloo.solo.io"},
		[]string{"settings"},
		[]string{"get", "list", "watch", "create"})
	permissions.AddExpectedPermission(
		"gloo-system.gloo",
		namespace,
		[]string{"enterprise.gloo.solo.io"},
		[]string{"authconfigs"},
		[]string{"get", "list", "watch", "update"})
	permissions.AddExpectedPermission(
		"gloo-system.gloo",
		namespace,
		[]string{"ratelimit.solo.io"},
		[]string{"ratelimitconfigs", "ratelimitconfigs/status"},
		[]string{"get", "list", "watch", "update"})
	permissions.AddExpectedPermission(
		"gloo-system.gloo",
		namespace,
		[]string{"graphql.gloo.solo.io"},
		[]string{"graphqlschemas"},
		[]string{"get", "list", "watch", "update"})

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
	permissions.AddExpectedPermission(
		"gloo-system.discovery",
		namespace,
		[]string{"graphql.gloo.solo.io"},
		[]string{"graphqlschemas"},
		[]string{"get", "list", "watch", "update", "create"})

	return permissions
}

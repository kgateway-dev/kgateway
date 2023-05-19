package test

import "github.com/solo-io/k8s-utils/manifesttestutils"

func GetServiceAccountPermissions(namespace string) *manifesttestutils.ServiceAccountPermissions {
	permissions := &manifesttestutils.ServiceAccountPermissions{}

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
		[]string{"*"},
	)
	permissions.AddExpectedPermission(
		"gloo-system.gloo",
		namespace,
		[]string{"coordination.k8s.io"},
		[]string{"leases"},
		[]string{"*"},
	)
	permissions.AddExpectedPermission(
		"gloo-system.gloo",
		namespace,
		[]string{"gloo.solo.io"},
		[]string{"upstreams", "upstreamgroups", "proxies"},
		[]string{"get", "list", "watch", "patch"})
	permissions.AddExpectedPermission(
		"gloo-system.gloo",
		namespace,
		[]string{"gateway.solo.io"},
		[]string{"gateways", "httpgateways", "tcpgateways", "virtualservices", "routetables", "virtualhostoptions", "routeoptions"},
		[]string{"get", "list", "watch", "patch"})
	permissions.AddExpectedPermission(
		"gloo-system.gloo",
		namespace,
		[]string{"gloo.solo.io"},
		[]string{"proxies"},
		[]string{"get", "list", "watch", "update", "patch", "create", "delete"})
	permissions.AddExpectedPermission(
		"gloo-system.gloo",
		namespace,
		[]string{"gloo.solo.io"},
		[]string{"settings"},
		[]string{"get", "list", "watch"})
	permissions.AddExpectedPermission(
		"gloo-system.gloo",
		namespace,
		[]string{"enterprise.gloo.solo.io"},
		[]string{"authconfigs"},
		[]string{"get", "list", "watch", "patch"})
	permissions.AddExpectedPermission(
		"gloo-system.gloo",
		namespace,
		[]string{"ratelimit.solo.io"},
		[]string{"ratelimitconfigs", "ratelimitconfigs/status"},
		[]string{"get", "list", "watch", "patch", "update"})
	permissions.AddExpectedPermission(
		"gloo-system.gloo",
		namespace,
		[]string{"graphql.gloo.solo.io"},
		[]string{"graphqlapis", "graphqlapis/status"},
		[]string{"get", "list", "watch", "patch", "update"})

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
		[]string{""},
		[]string{"configmaps"},
		[]string{"*"},
	)
	permissions.AddExpectedPermission(
		"gloo-system.discovery",
		namespace,
		[]string{"coordination.k8s.io"},
		[]string{"leases"},
		[]string{"*"},
	)
	permissions.AddExpectedPermission(
		"gloo-system.discovery",
		namespace,
		[]string{"gloo.solo.io"},
		[]string{"settings"},
		[]string{"get", "list", "watch"})
	permissions.AddExpectedPermission(
		"gloo-system.discovery",
		namespace,
		[]string{"gloo.solo.io"},
		[]string{"upstreams"},
		[]string{"get", "list", "watch", "create", "update", "patch", "delete"})
	permissions.AddExpectedPermission(
		"gloo-system.discovery",
		namespace,
		[]string{"graphql.gloo.solo.io"},
		[]string{"graphqlapis", "graphqlapis/status"},
		[]string{"get", "list", "watch", "update", "patch", "create"})
	return permissions
}

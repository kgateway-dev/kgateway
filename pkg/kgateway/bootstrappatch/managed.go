package bootstrappatch

import (
	"fmt"
	"slices"

	envoybootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	"google.golang.org/protobuf/proto"
)

// Paths an overlay can set that the control plane manages. A path is managed when
// kgateway generates it and something outside the bootstrap depends on its name or
// value: the controller, the proxy Deployment, envoyinit, or translated xDS.
const (
	// The controller selects the proxy's xDS snapshot by node.metadata.role, and
	// envoyinit sets node.locality.
	pathNode = "node"
	// The proxy receives all translated configuration through dynamic_resources.
	pathDynamicResources = "dynamic_resources"
	// The Deployment's preStop hook calls the admin endpoint, and admin_port_cluster
	// points at it.
	pathAdminAddress = "admin.address"
	// The controller sends endpoints for the local cluster by name, and translated
	// clusters use it for zone-aware routing.
	pathLocalClusterName = "cluster_manager.local_cluster_name"
)

// Names of generated static entries the control plane manages. An entry is managed
// only when the generated bootstrap contains it.
var (
	managedClusterNames = []string{
		// The proxy's connection to the controller.
		xdsClusterName,
		// The readiness and Prometheus listeners route to it.
		"admin_port_cluster",
		// With the Istio integration, translated listeners and clusters get Istio
		// certificates through it.
		"gateway_proxy_sds",
	}
	managedListenerNames = []string{
		// The Deployment's readiness and startup probes target its port.
		"readiness_listener",
		// The pod's Prometheus scrape annotation targets its port.
		"prometheus_listener",
	}
)

// systemCASecretName is the static secret envoyinit adds in the pod when it finds an
// OS CA bundle. It must match utils.SystemCaSecretName in internal/envoyinit.
const systemCASecretName = "SYSTEM_CA_CERT" //nolint:gosec // G101: the name of a secret, not a credential

// managedPaths returns each managed path overlay sets, in a fixed order. Matching
// uses the parsed proto, so both the snake_case and lowerCamelCase spellings of a
// field match. base is the generated bootstrap, which decides which static entries,
// including the local cluster, exist.
func managedPaths(base, overlay *envoybootstrapv3.Bootstrap) []string {
	// Fields whose presence in the overlay sets a managed path.
	var paths []string
	if overlay.Node != nil {
		paths = append(paths, pathNode)
	}
	if overlay.DynamicResources != nil {
		paths = append(paths, pathDynamicResources)
	}
	if overlay.GetAdmin().GetAddress() != nil {
		paths = append(paths, pathAdminAddress)
	}
	if overlay.GetClusterManager().GetLocalClusterName() != "" {
		paths = append(paths, pathLocalClusterName)
	}

	// Static entries the overlay replaces by name.
	for _, name := range managedClusters(base) {
		if containsName(overlay.GetStaticResources().GetClusters(), name) {
			paths = append(paths, clusterPath(name))
		}
	}
	for _, name := range managedListeners(base) {
		if containsName(overlay.GetStaticResources().GetListeners(), name) {
			paths = append(paths, listenerPath(name))
		}
	}
	return paths
}

// prune returns a copy of overlay without the managed paths managedPaths reports.
func prune(base, overlay *envoybootstrapv3.Bootstrap) *envoybootstrapv3.Bootstrap {
	// Clear the managed fields.
	out := proto.CloneOf(overlay)
	out.Node = nil
	out.DynamicResources = nil
	if out.Admin != nil {
		out.Admin.Address = nil
	}
	if out.ClusterManager != nil {
		out.ClusterManager.LocalClusterName = ""
	}

	// Drop the managed static entries.
	if sr := out.StaticResources; sr != nil {
		sr.Clusters = deleteNamed(sr.Clusters, managedClusters(base))
		sr.Listeners = deleteNamed(sr.Listeners, managedListeners(base))
	}
	return out
}

// deleteNamed removes the entries whose name is in names.
func deleteNamed[T namedEntry](entries []T, names []string) []T {
	return slices.DeleteFunc(entries, func(e T) bool { return slices.Contains(names, e.GetName()) })
}

// managedClusters returns the names of the managed static clusters in base, including
// the local cluster.
func managedClusters(base *envoybootstrapv3.Bootstrap) []string {
	candidates := managedClusterNames
	if local := base.GetClusterManager().GetLocalClusterName(); local != "" {
		candidates = append(slices.Clone(candidates), local)
	}
	return slices.DeleteFunc(slices.Clone(candidates), func(name string) bool {
		return !containsName(base.GetStaticResources().GetClusters(), name)
	})
}

// managedListeners returns the names of the managed static listeners in base.
func managedListeners(base *envoybootstrapv3.Bootstrap) []string {
	return slices.DeleteFunc(slices.Clone(managedListenerNames), func(name string) bool {
		return !containsName(base.GetStaticResources().GetListeners(), name)
	})
}

func containsName[T namedEntry](entries []T, name string) bool {
	return slices.ContainsFunc(entries, func(e T) bool { return e.GetName() == name })
}

func clusterPath(name string) string {
	return fmt.Sprintf("static_resources.clusters[name=%s]", name)
}

func listenerPath(name string) string {
	return fmt.Sprintf("static_resources.listeners[name=%s]", name)
}

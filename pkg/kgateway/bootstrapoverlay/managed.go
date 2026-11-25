package bootstrapoverlay

import (
	"fmt"
	"slices"
	"strings"

	envoybootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	"google.golang.org/protobuf/proto"
)

// Paths an overlay can set that the control plane manages, other than the generated
// bootstrap's named entries. A path is managed when kgateway generates it and
// something outside the bootstrap depends on its name or value: the controller, the
// proxy Deployment, envoyinit, or translated xDS.
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

// UserNamePrefix is the prefix of a static cluster or listener an overlay adds. No
// translated cluster or listener, and no entry of the generated bootstrap, has a name
// with it, so the entry cannot take the place of one.
const UserNamePrefix = "user_"

// systemCASecretName is the static secret envoyinit adds in the pod when it finds an
// OS CA bundle. It must match utils.SystemCaSecretName in internal/envoyinit.
const systemCASecretName = "SYSTEM_CA_CERT" //nolint:gosec // G101: the name of a secret, not a credential

// managedPaths returns each managed path overlay sets, in a fixed order. Matching
// uses the parsed proto, so both the snake_case and lowerCamelCase spellings of a
// field match. base is the generated bootstrap: every named entry in it is a managed
// path, so an entry a later release adds is managed without a change here.
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

	// Generated named entries the overlay replaces by name.
	baseSR, ovSR := base.GetStaticResources(), overlay.GetStaticResources()
	for _, name := range sharedNames(baseSR.GetClusters(), ovSR.GetClusters()) {
		paths = append(paths, clusterPath(name))
	}
	for _, name := range sharedNames(baseSR.GetListeners(), ovSR.GetListeners()) {
		paths = append(paths, listenerPath(name))
	}
	for _, name := range sharedNames(baseSR.GetSecrets(), ovSR.GetSecrets()) {
		paths = append(paths, fmt.Sprintf("static_resources.secrets[name=%s]", name))
	}
	for _, name := range sharedNames(base.GetOverloadManager().GetResourceMonitors(), overlay.GetOverloadManager().GetResourceMonitors()) {
		paths = append(paths, fmt.Sprintf("overload_manager.resource_monitors[name=%s]", name))
	}
	return paths
}

// sharedNames returns the names of the overlay entries that a base entry also has, in
// overlay order.
func sharedNames[T namedEntry](base, overlay []T) []string {
	var out []string
	for _, e := range overlay {
		if e.GetName() != "" && containsName(base, e.GetName()) {
			out = append(out, e.GetName())
		}
	}
	return out
}

// checkNewStaticNames rejects each static cluster or listener in overlay whose name is
// neither the name of an entry in base, the generated bootstrap, nor starts with
// UserNamePrefix. Envoy ignores a cluster or listener from xDS with the name of a
// static one, so an entry with a translated name would take its place, and nothing
// would report it. It returns one error for each rejected name.
func checkNewStaticNames(base, overlay *envoybootstrapv3.Bootstrap) []error {
	// Check the clusters, then the listeners.
	var errs []error
	baseSR, ovSR := base.GetStaticResources(), overlay.GetStaticResources()
	for _, name := range unprefixed(baseSR.GetClusters(), ovSR.GetClusters()) {
		errs = append(errs, fmt.Errorf("static cluster %q is not in the generated bootstrap and its name does not start with %q; "+
			"give a new static cluster a name that starts with %q",
			name, UserNamePrefix, UserNamePrefix))
	}
	for _, name := range unprefixed(baseSR.GetListeners(), ovSR.GetListeners()) {
		errs = append(errs, fmt.Errorf("static listener %q is not in the generated bootstrap and its name does not start with %q; "+
			"give a new static listener a name that starts with %q",
			name, UserNamePrefix, UserNamePrefix))
	}
	return errs
}

// unprefixed returns, once each, the names of the overlay entries that have neither
// the name of a base entry nor UserNamePrefix.
func unprefixed[T namedEntry](base, overlay []T) []string {
	var names []string
	for _, e := range overlay {
		if !strings.HasPrefix(e.GetName(), UserNamePrefix) && !containsName(base, e.GetName()) &&
			!slices.Contains(names, e.GetName()) {
			names = append(names, e.GetName())
		}
	}
	return names
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

	// Drop the entries that replace generated ones.
	if sr := out.StaticResources; sr != nil {
		baseSR := base.GetStaticResources()
		sr.Clusters = deleteNamed(sr.Clusters, baseSR.GetClusters())
		sr.Listeners = deleteNamed(sr.Listeners, baseSR.GetListeners())
		sr.Secrets = deleteNamed(sr.Secrets, baseSR.GetSecrets())
	}
	if om := out.OverloadManager; om != nil {
		om.ResourceMonitors = deleteNamed(om.ResourceMonitors, base.GetOverloadManager().GetResourceMonitors())
	}
	return out
}

// deleteNamed removes the entries that have the name of an entry in base.
func deleteNamed[T namedEntry](entries, base []T) []T {
	return slices.DeleteFunc(entries, func(e T) bool { return containsName(base, e.GetName()) })
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

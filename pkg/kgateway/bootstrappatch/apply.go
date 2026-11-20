package bootstrappatch

import (
	"cmp"
	"fmt"
	"strings"

	envoybootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	"google.golang.org/protobuf/proto"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
)

// Source is the resource that holds an overlay. Status messages and events name it.
type Source struct {
	// Kind is the resource's kind. Empty means GatewayParameters.
	Kind      string
	Namespace string
	Name      string
}

// String returns the source as "<kind> <namespace>/<name>".
func (s Source) String() string {
	return fmt.Sprintf("%s %s/%s", cmp.Or(s.Kind, "GatewayParameters"), s.Namespace, s.Name)
}

// Overlay is a partial bootstrap from one resource.
type Overlay struct {
	// Source is the resource that holds the overlay.
	Source Source
	// Raw is the overlay as JSON.
	Raw []byte
}

// ManagedPath is a control-plane-managed path an overlay sets.
type ManagedPath struct {
	// Path is the field path, with list entries identified by name, for example
	// static_resources.clusters[name=xds_cluster].
	Path string
	// Source is the resource whose overlay sets the path.
	Source Source
}

func (p ManagedPath) String() string {
	return fmt.Sprintf("%s (%s)", p.Path, p.Source)
}

// ReplacedEntry is an entry of the generated bootstrap that an overlay replaces by
// name, and that is not a managed path.
type ReplacedEntry struct {
	// Path is the entry's path, for example
	// overload_manager.resource_monitors[name=envoy.resource_monitors.global_downstream_max_connections].
	Path string
	// Source is the resource whose overlay replaces the entry.
	Source Source
}

func (e ReplacedEntry) String() string {
	return fmt.Sprintf("%s (%s)", e.Path, e.Source)
}

// Result is the outcome of applying overlays to a generated bootstrap.
type Result struct {
	// Bootstrap is the merged bootstrap.
	Bootstrap *envoybootstrapv3.Bootstrap
	// ManagedPaths lists each managed path the overlays set. Under WARN each one was
	// applied; under PRUNE each one was dropped from its overlay before merging.
	ManagedPaths []ManagedPath
	// ReplacedEntries lists each entry of the generated bootstrap that the overlays
	// replace by name, other than the managed paths in ManagedPaths. Each replacement
	// applied under every mode.
	ReplacedEntries []ReplacedEntry
}

// InvalidOverlayError reports an overlay that cannot be applied under any mode.
type InvalidOverlayError struct {
	Source Source
	Err    error
}

func (e *InvalidOverlayError) Error() string {
	return fmt.Sprintf("bootstrap patch in %s: %v", e.Source, e.Err)
}

func (e *InvalidOverlayError) Unwrap() error {
	return e.Err
}

// ManagedPathsDeniedError reports overlays that REJECT refused because they set
// managed paths.
type ManagedPathsDeniedError struct {
	Paths []ManagedPath
}

func (e *ManagedPathsDeniedError) Error() string {
	paths := make([]string, 0, len(e.Paths))
	for _, p := range e.Paths {
		paths = append(paths, p.String())
	}
	return "the controller is configured to reject bootstrap patches that set paths it manages; remove these paths from the patch: " +
		strings.Join(paths, ", ")
}

// Apply merges overlays, in order, onto a copy of base, the generated bootstrap. An
// overlay later in the list wins a conflict with an earlier one. mode decides what
// happens to an overlay that sets a managed path; an empty mode is WARN. Apply returns
// an *InvalidOverlayError or a *ManagedPathsDeniedError when it applies nothing.
func Apply(
	base *envoybootstrapv3.Bootstrap,
	overlays []Overlay,
	mode apisettings.BootstrapPatchManagedPathsMode,
) (*Result, error) {
	// Record which runtime layer names the generated bootstrap uses. An empty owner
	// means the generated bootstrap; otherwise it is the overlay's source.
	layerOwners := map[string]string{}
	for _, l := range base.GetLayeredRuntime().GetLayers() {
		layerOwners[l.GetName()] = ""
	}

	// Parse and check every overlay before merging any, so that REJECT reports the
	// managed paths of all of them.
	parsed := make([]*envoybootstrapv3.Bootstrap, 0, len(overlays))
	var managed []ManagedPath
	var replaced []ReplacedEntry
	for _, o := range overlays {
		ov, err := parseOverlay(o.Raw)
		if err != nil {
			return nil, &InvalidOverlayError{Source: o.Source, Err: err}
		}
		if err := checkReserved(ov); err != nil {
			return nil, &InvalidOverlayError{Source: o.Source, Err: err}
		}
		if err := claimLayers(ov, o.Source.String(), layerOwners); err != nil {
			return nil, &InvalidOverlayError{Source: o.Source, Err: err}
		}

		// Record the managed paths it sets and the other generated entries it replaces,
		// and drop the managed paths under PRUNE.
		for _, p := range managedPaths(base, ov) {
			managed = append(managed, ManagedPath{Path: p, Source: o.Source})
		}
		for _, p := range replacedEntries(base, ov) {
			replaced = append(replaced, ReplacedEntry{Path: p, Source: o.Source})
		}
		if mode == apisettings.BootstrapPatchManagedPathsPrune {
			ov = prune(base, ov)
		}
		parsed = append(parsed, ov)
	}

	// REJECT applies nothing when any overlay sets a managed path.
	if mode == apisettings.BootstrapPatchManagedPathsReject && len(managed) > 0 {
		return nil, &ManagedPathsDeniedError{Paths: managed}
	}

	// Merge the overlays in order.
	merged := proto.CloneOf(base)
	for _, ov := range parsed {
		mergeOverlay(merged, ov)
	}
	return &Result{Bootstrap: merged, ManagedPaths: managed, ReplacedEntries: replaced}, nil
}

// checkReserved rejects names the proxy reserves and lists entries Envoy requires to
// be unique that the overlay defines more than once.
func checkReserved(ov *envoybootstrapv3.Bootstrap) error {
	// envoyinit adds this secret in the pod, so a second one fails there as a duplicate.
	if containsName(ov.GetStaticResources().GetSecrets(), systemCASecretName) {
		return fmt.Errorf("static secret %q is reserved: the proxy adds it for the OS CA bundle", systemCASecretName)
	}

	// Only a static layer has its values in the bootstrap, where route validation and
	// status see them. The values of a disk or RTDS layer come from outside the
	// bootstrap, and the generated bootstrap already has the one admin layer Envoy
	// allows.
	for _, l := range ov.GetLayeredRuntime().GetLayers() {
		if l.GetStaticLayer() == nil {
			return fmt.Errorf("runtime layer %q sets %s instead of static_layer; a bootstrap patch can add only static runtime layers", l.GetName(), runtimeLayerKind(l))
		}
	}

	// A name repeated inside one overlay would make the second entry replace the first.
	sr := ov.GetStaticResources()
	for _, check := range []struct {
		field string
		dup   string
	}{
		{"static_resources.clusters", firstDuplicate(sr.GetClusters())},
		{"static_resources.listeners", firstDuplicate(sr.GetListeners())},
		{"static_resources.secrets", firstDuplicate(sr.GetSecrets())},
		{"overload_manager.resource_monitors", firstDuplicate(ov.GetOverloadManager().GetResourceMonitors())},
	} {
		if check.dup != "" {
			return fmt.Errorf("%s has more than one entry named %q", check.field, check.dup)
		}
	}
	return nil
}

// claimLayers records the overlay's runtime layer names in owners, and rejects a name
// that the generated bootstrap, an earlier overlay or this overlay already uses. Envoy
// rejects duplicate layer names at startup.
func claimLayers(ov *envoybootstrapv3.Bootstrap, source string, owners map[string]string) error {
	for _, l := range ov.GetLayeredRuntime().GetLayers() {
		owner, taken := owners[l.GetName()]
		switch {
		case !taken:
			owners[l.GetName()] = source
		case owner == "":
			return fmt.Errorf("runtime layer %q has the name of a layer in the generated bootstrap; give it another name", l.GetName())
		case owner == source:
			return fmt.Errorf("layered_runtime.layers has more than one entry named %q", l.GetName())
		default:
			return fmt.Errorf("runtime layer %q is also defined in %s; give one of them another name", l.GetName(), owner)
		}
	}
	return nil
}

// firstDuplicate returns the first non-empty name that appears more than once.
func firstDuplicate[T namedEntry](entries []T) string {
	seen := map[string]bool{}
	for _, e := range entries {
		name := e.GetName()
		if name == "" {
			continue
		}
		if seen[name] {
			return name
		}
		seen[name] = true
	}
	return ""
}

// runtimeLayerKind names the field that sets l's values, such as disk_layer.
func runtimeLayerKind(l *envoybootstrapv3.RuntimeLayer) string {
	switch {
	case l.GetDiskLayer() != nil:
		return "disk_layer"
	case l.GetRtdsLayer() != nil:
		return "rtds_layer"
	case l.GetAdminLayer() != nil:
		return "admin_layer"
	}
	return "no layer type"
}

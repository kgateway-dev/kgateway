package bootstrappatch

import (
	"fmt"
	"strings"

	envoybootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	"google.golang.org/protobuf/proto"
	"k8s.io/apimachinery/pkg/types"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
)

// Overlay is a partial bootstrap from one GatewayParameters.
type Overlay struct {
	// Source is the GatewayParameters that holds the overlay.
	Source types.NamespacedName
	// Raw is the overlay as JSON.
	Raw []byte
}

// ManagedPath is a control-plane-managed path an overlay sets.
type ManagedPath struct {
	// Path is the field path, with list entries identified by name, for example
	// static_resources.clusters[name=xds_cluster].
	Path string
	// Source is the GatewayParameters whose overlay sets the path.
	Source types.NamespacedName
}

func (p ManagedPath) String() string {
	return fmt.Sprintf("%s (GatewayParameters %s)", p.Path, p.Source)
}

// Result is the outcome of applying overlays to a generated bootstrap.
type Result struct {
	// Bootstrap is the merged bootstrap.
	Bootstrap *envoybootstrapv3.Bootstrap
	// ManagedPaths lists each managed path the overlays set. Under WARN each one was
	// applied; under PRUNE each one was dropped from its overlay before merging.
	ManagedPaths []ManagedPath
	// CuratedOverrides lists each path the overlays replace that a GatewayParameters
	// field generated. They apply under every mode.
	CuratedOverrides []CuratedOverride
	// UnvalidatedLayers lists each disk and RTDS runtime layer the overlays add. Route
	// validation uses Envoy's defaults for the values they set.
	UnvalidatedLayers []RuntimeLayer
}

// InvalidOverlayError reports an overlay that cannot be applied under any mode.
type InvalidOverlayError struct {
	Source types.NamespacedName
	Err    error
}

func (e *InvalidOverlayError) Error() string {
	return fmt.Sprintf("bootstrap overlay in GatewayParameters %s: %v", e.Source, e.Err)
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
	return "the controller is configured to reject bootstrap overlays that set paths it manages; remove these paths from the overlay: " +
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
	// means the generated bootstrap; otherwise it is the GatewayParameters.
	layerOwners := map[string]string{}
	for _, l := range base.GetLayeredRuntime().GetLayers() {
		layerOwners[l.GetName()] = ""
	}

	// Parse and check every overlay before merging any, so that REJECT reports the
	// managed paths of all of them.
	parsed := make([]*envoybootstrapv3.Bootstrap, 0, len(overlays))
	var managed []ManagedPath
	var curated []CuratedOverride
	var unvalidated []RuntimeLayer
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
		if err := checkRTDSLayers(ov); err != nil {
			return nil, &InvalidOverlayError{Source: o.Source, Err: err}
		}

		unvalidated = append(unvalidated, unvalidatedRuntimeLayers(ov, o.Source)...)
		for _, p := range managedPaths(base, ov) {
			managed = append(managed, ManagedPath{Path: p, Source: o.Source})
		}
		curated = append(curated, curatedOverrides(base, ov, o.Source)...)
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
	return &Result{Bootstrap: merged, ManagedPaths: managed, CuratedOverrides: curated, UnvalidatedLayers: unvalidated}, nil
}

// checkReserved rejects names the proxy reserves and lists entries Envoy requires to
// be unique that the overlay defines more than once.
func checkReserved(ov *envoybootstrapv3.Bootstrap) error {
	// envoyinit adds this secret in the pod, so a second one fails there as a duplicate.
	if containsName(ov.GetStaticResources().GetSecrets(), systemCASecretName) {
		return fmt.Errorf("static secret %q is reserved: the proxy adds it for the OS CA bundle", systemCASecretName)
	}

	// The generated bootstrap has the only admin layer, which /runtime_modify writes to.
	for _, l := range ov.GetLayeredRuntime().GetLayers() {
		if l.GetAdminLayer() != nil {
			return fmt.Errorf("runtime layer %q is an admin layer; the generated bootstrap already has one, and Envoy allows only one", l.GetName())
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
			return fmt.Errorf("runtime layer %q is also defined in GatewayParameters %s; give one of them another name", l.GetName(), owner)
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

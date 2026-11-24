package bootstrapoverlay

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
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
	Source Source
	// Raw is the overlay as JSON.
	Raw []byte
}

// ManagedPath is a control-plane-managed path an overlay sets.
type ManagedPath struct {
	// Path is the field path, with list entries identified by name, for example
	// static_resources.clusters[name=xds_cluster].
	Path   string
	Source Source
}

func (p ManagedPath) String() string {
	return fmt.Sprintf("%s (%s)", p.Path, p.Source)
}

// Result is the outcome of applying overlays to a generated bootstrap.
type Result struct {
	Bootstrap *envoybootstrapv3.Bootstrap
	// ManagedPaths lists each managed path the overlays set. Under WARN each one was
	// applied; under PRUNE each one was dropped from its overlay before merging.
	ManagedPaths []ManagedPath
}

// InvalidOverlayError reports an overlay that cannot be applied under any mode.
type InvalidOverlayError struct {
	Source Source
	Err    error
}

func (e *InvalidOverlayError) Error() string {
	return fmt.Sprintf("bootstrap overlay in %s: %v", e.Source, e.Err)
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
	return "KGW_BOOTSTRAP_OVERLAY_MANAGED_PATHS is REJECT, so a bootstrap overlay cannot set a path kgateway manages; remove these paths from the overlay: " +
		strings.Join(paths, ", ")
}

// Apply merges overlays, in order, onto a copy of base, the generated bootstrap. An
// overlay later in the list wins a conflict with an earlier one. mode decides what
// happens to an overlay that sets a managed path; an empty mode is WARN. When it
// applies nothing, Apply returns a *ManagedPathsDeniedError, or an errors.Join of one
// *InvalidOverlayError for each problem in each invalid overlay.
func Apply(
	base *envoybootstrapv3.Bootstrap,
	overlays []Overlay,
	mode apisettings.BootstrapOverlayManagedPathsMode,
) (*Result, error) {
	// Record which runtime layer names the generated bootstrap uses. An empty owner
	// means the generated bootstrap; otherwise it is the overlay's source.
	layerOwners := map[string]string{}
	for _, l := range base.GetLayeredRuntime().GetLayers() {
		layerOwners[l.GetName()] = ""
	}

	// Parse and check every overlay before merging any, so that the error reports
	// every problem in every overlay, and REJECT reports the managed paths of all of
	// them. Each check reads only the overlay, base, and the layer names of the
	// overlays before it, so a problem in one overlay does not hide a problem in another.
	parsed := make([]*envoybootstrapv3.Bootstrap, 0, len(overlays))
	var managed []ManagedPath
	var invalid []error
	for _, o := range overlays {
		// Parse the overlay. An overlay that does not parse has nothing else to check.
		ov, err := parseOverlay(o.Raw)
		if err != nil {
			invalid = append(invalid, &InvalidOverlayError{Source: o.Source, Err: err})
			continue
		}

		// Run every check, so that the error lists each problem in the overlay.
		problems := slices.Concat(
			checkReserved(ov),
			checkNewStaticNames(base, ov),
			claimLayers(ov, o.Source.String(), layerOwners),
		)
		for _, p := range problems {
			invalid = append(invalid, &InvalidOverlayError{Source: o.Source, Err: p})
		}
		if len(problems) > 0 {
			continue
		}

		// Record the managed paths it sets, and drop them under PRUNE.
		for _, p := range managedPaths(base, ov) {
			managed = append(managed, ManagedPath{Path: p, Source: o.Source})
		}
		if mode == apisettings.BootstrapOverlayManagedPathsPrune {
			ov = prune(base, ov)
		}
		parsed = append(parsed, ov)
	}

	// Any invalid overlay stops the merge under every mode.
	if len(invalid) > 0 {
		return nil, errors.Join(invalid...)
	}

	// REJECT applies nothing when any overlay sets a managed path.
	if mode == apisettings.BootstrapOverlayManagedPathsReject && len(managed) > 0 {
		return nil, &ManagedPathsDeniedError{Paths: managed}
	}

	// Merge the overlays in order.
	merged := proto.CloneOf(base)
	for _, ov := range parsed {
		mergeOverlay(merged, ov)
	}
	return &Result{Bootstrap: merged, ManagedPaths: managed}, nil
}

// checkReserved rejects names the proxy reserves, runtime layers other than static
// ones, and lists entries Envoy requires to be unique that the overlay defines more
// than once. It returns one error for each such problem.
func checkReserved(ov *envoybootstrapv3.Bootstrap) []error {
	var errs []error

	// envoyinit adds this secret in the pod, so a second one fails there as a duplicate.
	if containsName(ov.GetStaticResources().GetSecrets(), systemCASecretName) {
		errs = append(errs, fmt.Errorf("static secret %q is reserved: the proxy adds it for the OS CA bundle", systemCASecretName))
	}

	// Only a static layer has its values in the bootstrap, where route validation and
	// status see them. The values of a disk or RTDS layer come from outside the
	// bootstrap, and the generated bootstrap already has the one admin layer Envoy allows.
	for _, l := range ov.GetLayeredRuntime().GetLayers() {
		if l.GetStaticLayer() == nil {
			errs = append(errs, fmt.Errorf("runtime layer %q sets %s instead of static_layer; a bootstrap overlay can add only static runtime layers", l.GetName(), runtimeLayerKind(l)))
		}
	}

	// A name repeated inside one overlay would make the second entry replace the
	// first, and Envoy rejects duplicate runtime layer names at startup.
	sr := ov.GetStaticResources()
	for _, check := range []struct {
		field string
		dups  []string
	}{
		{"static_resources.clusters", duplicates(sr.GetClusters())},
		{"static_resources.listeners", duplicates(sr.GetListeners())},
		{"static_resources.secrets", duplicates(sr.GetSecrets())},
		{"overload_manager.resource_monitors", duplicates(ov.GetOverloadManager().GetResourceMonitors())},
		{"layered_runtime.layers", duplicates(ov.GetLayeredRuntime().GetLayers())},
	} {
		for _, name := range check.dups {
			errs = append(errs, fmt.Errorf("%s has more than one entry named %q", check.field, name))
		}
	}
	return errs
}

// claimLayers records the overlay's runtime layer names in owners, and rejects each
// name that the generated bootstrap or an earlier overlay already uses. Envoy rejects
// duplicate layer names at startup. checkReserved reports a name repeated inside the
// overlay itself. It returns one error for each rejected name.
func claimLayers(ov *envoybootstrapv3.Bootstrap, source string, owners map[string]string) []error {
	var errs []error
	for _, l := range ov.GetLayeredRuntime().GetLayers() {
		owner, taken := owners[l.GetName()]
		switch {
		case !taken:
			owners[l.GetName()] = source
		case owner == "":
			errs = append(errs, fmt.Errorf("runtime layer %q has the name of a layer in the generated bootstrap; give it another name", l.GetName()))
		case owner != source:
			errs = append(errs, fmt.Errorf("runtime layer %q is also defined in %s; give one of them another name", l.GetName(), owner))
		}
	}
	return errs
}

// duplicates returns each non-empty name that appears more than once, once each, in
// the order of its second appearance.
func duplicates[T namedEntry](entries []T) []string {
	counts := map[string]int{}
	var dups []string
	for _, e := range entries {
		name := e.GetName()
		if name == "" {
			continue
		}
		counts[name]++
		if counts[name] == 2 {
			dups = append(dups, name)
		}
	}
	return dups
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

// Package gatewayapiversion provides a startup check that refuses to run
// kgateway when the installed Gateway API bundle version is not one this
// release supports. The list of supported versions is embedded from
// supported_versions.yaml.
package gatewayapiversion

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync"

	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/yaml"

	_ "embed"
)

// BundleVersionAnnotation is the annotation key Gateway API CRDs use to record
// the release bundle they were generated from, e.g. "v1.5.1".
const BundleVersionAnnotation = "gateway.networking.k8s.io/bundle-version"

// DocsURL points users at the kgateway docs when their installed Gateway API
// bundle is not supported by this release of kgateway.
const DocsURL = "https://kgateway.dev/"

// probeCRD is a Gateway API CRD we use to read the bundle-version annotation.
// Every Gateway API CRD in a given bundle carries the same annotation, so any
// one of them works; Gateways is required for kgateway to be useful at all. Do
// not choose a recently added type here because someone might install
// 1.99.0[experimental] and overwrite with 1.98.0[standard], leaving
// experimental APIs in place.
const probeCRD = "gateways.gateway.networking.k8s.io"

//go:embed supported_versions.yaml
var supportedVersionsYAML []byte

type supportedVersionsFile struct {
	SupportedVersions []string `json:"supportedVersions"`
}

// minorVersionRE matches a leading "vMAJOR.MINOR" (patch and suffixes ignored).
var minorVersionRE = regexp.MustCompile(`^v?(\d+)\.(\d+)`)

// Check reads the bundle-version annotation from a Gateway API CRD installed
// in the cluster and returns a non-nil error if its minor version is not one
// the embedded supported_versions.yaml lists.
//
// If the Gateways CRD is not present in the cluster at all, Check returns nil
// — kgateway cannot function without Gateway API CRDs, but that is a separate
// failure mode from "wrong Gateway API version" and the controller will
// surface it through its own informers. Callers who need to enforce presence
// should do so separately.
func Check(ctx context.Context, restConfig *rest.Config) error {
	clientset, err := apiextensionsclient.NewForConfig(restConfig)
	if err != nil {
		return fmt.Errorf("creating apiextensions client to check Gateway API version: %w. %s", err, bypassHint())
	}

	crd, err := clientset.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, probeCRD, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Gateway API CRDs are not installed. Nothing to check here.
			return nil
		}
		return fmt.Errorf("fetching %s to check Gateway API version: %w. %s", probeCRD, err, bypassHint())
	}

	bundleVersion := crd.Annotations[BundleVersionAnnotation]
	return checkBundleVersion(bundleVersion)
}

// bypassHint is the common suffix appended to version-check errors so every
// failure mode points operators at the docs and the escape hatch.
func bypassHint() string {
	return fmt.Sprintf("See %s for supported versions, or set KGW_SKIP_GATEWAY_API_VERSION_CHECK=true to bypass this check", DocsURL)
}

// checkBundleVersion validates a bundle-version annotation value (e.g. "v1.5.1")
// against the embedded supported list. Split out for testability.
func checkBundleVersion(bundleVersion string) error {
	supported, err := loadSupportedVersions()
	if err != nil {
		return err
	}

	bundleVersion = strings.TrimSpace(bundleVersion)
	if bundleVersion == "" {
		return fmt.Errorf(
			"Gateway API CRD %s is missing the %s annotation; unable to verify compatibility with kgateway. %s",
			probeCRD, BundleVersionAnnotation, bypassHint(),
		)
	}

	minor, ok := parseMinorVersion(bundleVersion)
	if !ok {
		return fmt.Errorf(
			"Gateway API CRD %s has an unparseable %s annotation %q; expected form vMAJOR.MINOR[.PATCH][-suffix] (e.g. v1.3, v1.3.0, or v1.3.0-rc.1). %s",
			probeCRD, BundleVersionAnnotation, bundleVersion, bypassHint(),
		)
	}

	if slices.Contains(supported, minor) {
		return nil
	}

	return fmt.Errorf(
		"installed Gateway API version %s (minor %s) is not supported by this release of kgateway; supported versions: %s. %s",
		bundleVersion, minor, strings.Join(supported, ", "), bypassHint(),
	)
}

// parseMinorVersion returns the "MAJOR.MINOR" prefix of a bundle-version
// string. Accepts an optional leading "v". Patch / prerelease suffixes are
// stripped. Returns false when the string does not start with a version.
func parseMinorVersion(v string) (string, bool) {
	m := minorVersionRE.FindStringSubmatch(v)
	if m == nil {
		return "", false
	}
	return m[1] + "." + m[2], true
}

var (
	supportedVersionsOnce  sync.Once
	supportedVersionsCache []string
	supportedVersionsErr   error
)

func loadSupportedVersions() ([]string, error) {
	supportedVersionsOnce.Do(func() {
		var f supportedVersionsFile
		if err := yaml.Unmarshal(supportedVersionsYAML, &f); err != nil {
			supportedVersionsErr = fmt.Errorf("parsing embedded supported_versions.yaml: %w", err)
			return
		}
		if len(f.SupportedVersions) == 0 {
			supportedVersionsErr = fmt.Errorf("embedded supported_versions.yaml lists no supported versions")
			return
		}
		supportedVersionsCache = f.SupportedVersions
	})
	if supportedVersionsErr != nil {
		return nil, supportedVersionsErr
	}
	out := make([]string, len(supportedVersionsCache))
	copy(out, supportedVersionsCache)
	return out, nil
}

// SupportedVersions returns the list of supported Gateway API minor versions
// embedded in the binary. Primarily exposed for logging and diagnostics.
func SupportedVersions() ([]string, error) {
	return loadSupportedVersions()
}

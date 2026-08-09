package collections

import (
	"context"
	"slices"

	"istio.io/istio/pkg/util/sets"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// routeVersionDiscovery is everything CRD discovery could establish about one route kind.
// Only the raw fact is recorded here; which versions we act on is decided by
// selectRouteGVRs, so there is no way to represent a self-contradictory result.
type routeVersionDiscovery struct {
	// Authoritative is false when the CRD could not be read at all: the API server was
	// unavailable, or the CRD is not installed yet. It is not the same as "serves nothing".
	Authoritative bool
	// Served holds the version strings the CRD reports as served. Meaningful only when
	// Authoritative.
	Served sets.Set[string]
}

// selectRouteGVRs picks the API versions of one route kind to watch and to write status
// through, most preferred first. Watching and writing deliberately share this answer:
// letting them diverge means either watching a version we cannot write, or building a client
// for a version we never watch, and both have shipped as bugs.
//
// known lists every version we understand for the kind, most preferred first. includeLegacy
// reports whether pre-promotion versions are enabled at all; when they are not, only the
// promoted version is a candidate no matter what the cluster serves.
//
// An authoritative result narrows to exactly one version, so the common case costs one
// informer and one client. A non-authoritative result cannot narrow anything: we do not know
// which versions the CRD will serve once it appears, so every allowed version stays a
// candidate and the writer dispatches to whichever informer actually holds the object.
// Guessing a single version instead is a silent, permanent status outage when the guess is
// wrong, because a client for a never-served version returns nil from every Get.
func selectRouteGVRs(
	discovery routeVersionDiscovery,
	known []schema.GroupVersionResource,
	includeLegacy bool,
) []schema.GroupVersionResource {
	// Full slice expression: a two-element view of known would otherwise let an append by
	// any caller write straight into the package-level preference list.
	allowed := known[:1:1]
	if includeLegacy {
		allowed = known
	}

	if !discovery.Authoritative {
		return slices.Clone(allowed)
	}
	for _, gvr := range allowed {
		if discovery.Served.Contains(gvr.Version) {
			return []schema.GroupVersionResource{gvr}
		}
	}

	// Authoritative discovery found nothing we can use: either the CRD serves only versions
	// we do not understand, or the one version it serves is disabled by includeLegacy. No
	// watch will yield objects and no write can land, so the choice is inconsequential; we
	// still need a stable identity for GVK keying, and naming the promoted version keeps the
	// watch and the writer pointed at the same place.
	logger.Warn("no usable served API version for route kind; its resources will not be reconciled",
		"resource", known[0].Resource,
		"served", discovery.Served.UnsortedList(),
		"legacy_versions_enabled", includeLegacy,
	)
	return []schema.GroupVersionResource{known[0]}
}

// discoverRouteVersions reads which API versions a route kind's CRD currently serves.
// A result that is not Authoritative means we could not find out, not that nothing is
// served: the watch path recovers through delayed informers and the write path spans every
// candidate, but neither can narrow to the served version without a restart, so it is worth
// saying loudly.
func discoverRouteVersions(ctx context.Context, extClient apiextensionsclient.Interface, crdName string) routeVersionDiscovery {
	if extClient == nil {
		logger.Warn("no CRD discovery client; watching and writing status through every known API version until the CRD is observed",
			"crd", crdName)
		return routeVersionDiscovery{}
	}

	ctx, cancel := context.WithTimeout(ctx, crdLookupTimeout)
	defer cancel()

	crd, err := extClient.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, crdName, metav1.GetOptions{})
	if err != nil {
		logger.Warn("could not resolve served API versions; watching and writing status through every known API version until restart",
			"crd", crdName,
			"error", err,
		)
		return routeVersionDiscovery{}
	}

	served := sets.New[string]()
	for _, version := range crd.Spec.Versions {
		if version.Served {
			served.Insert(version.Name)
		}
	}
	return routeVersionDiscovery{Authoritative: true, Served: served}
}

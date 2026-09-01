package collections

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/sleep"
	"istio.io/istio/pkg/util/sets"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"

	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
)

var logger = logging.New("pluginsdk/collections")

const (
	// crdLookupTimeout bounds a single read of the served versions.
	crdLookupTimeout = 5 * time.Second

	// startupResolveAttempts and startupResolveDelay bound the retry the startup resolution
	// gets. Only the startup answer is worth retrying: it decides which collections exist
	// and which versions status is written through for the life of the process, and it
	// cannot be revised later. The delayed informers' poll loop is its own retry, so it
	// reads once per round instead.
	startupResolveAttempts = 3
	startupResolveDelay    = 500 * time.Millisecond
)

// routeVersionSource reads which API versions a kind is served under. It holds two readers
// of the same fact because they fail independently: the apiextensions read is precise but
// needs get on customresourcedefinitions, while the discovery read needs only the
// system:discovery role every authenticated client already has, so it still answers when an
// RBAC gap hides CRDs. Either reader's answer is authoritative; only failing both leaves us
// guessing, and a guess is the one outcome with no good handling (see informerGate).
type routeVersionSource struct {
	ext   apiextensionsclient.Interface
	disco discovery.DiscoveryInterface
}

func newRouteVersionSource(c kube.Client) routeVersionSource {
	if c == nil {
		return routeVersionSource{}
	}
	src := routeVersionSource{ext: c.Ext()}
	if kubeClient := c.Kube(); kubeClient != nil {
		src.disco = kubeClient.Discovery()
	}
	return src
}

// allowedRouteGVRs narrows the versions we understand to the ones enabled. known is most
// preferred first; includeLegacy reports whether pre-promotion versions are candidates at
// all, and when they are not only the promoted version is, no matter what the cluster
// serves.
func allowedRouteGVRs(known []schema.GroupVersionResource, includeLegacy bool) []schema.GroupVersionResource {
	if includeLegacy {
		return known
	}
	// Full slice expression: known is package-level state, and a one-element view of it with
	// room to spare would let an append here overwrite the second preference.
	return known[:1:1]
}

// preferredServedGVR returns the most preferred version that is both enabled and currently
// served. ok is false when there is none, which covers both "the kind is not installed" and
// "we could not find out" -- neither of which licenses committing to a version. It is
// deliberately silent: the delayed informers' poll loop calls it every round, so a log line
// here would repeat forever.
func preferredServedGVR(
	served sets.Set[string],
	known []schema.GroupVersionResource,
	includeLegacy bool,
) (schema.GroupVersionResource, bool) {
	for _, gvr := range allowedRouteGVRs(known, includeLegacy) {
		if served.Contains(gvr.Version) {
			return gvr, true
		}
	}
	return schema.GroupVersionResource{}, false
}

// selectRouteGVRs picks the API versions of one route kind to build collections for and to
// write status through, most preferred first. Watching and writing deliberately share this
// answer: letting them diverge means either watching a version we cannot write, or building
// a client for a version we never watch, and both have shipped as bugs.
//
// A version that is served now narrows this to exactly one, so the common case costs one
// informer and one client. Otherwise nothing can be narrowed: krt collections are built once
// at startup, so committing to a version here is committing for the life of the process, and
// a version that turns out to be the wrong guess is a silent permanent status outage -- a
// client for a never-served version returns nil from every Get. So every enabled version
// keeps a collection, and whichever one the cluster turns out to serve is the one that
// carries objects and gets written back through. The extra collections are not extra
// watches: each is gated on its own version being the served one (see routeInformerGate), so
// at most one of them ever runs, and a kind installed after startup still lands on the right
// version without a restart.
func selectRouteGVRs(
	served sets.Set[string],
	known []schema.GroupVersionResource,
	includeLegacy bool,
) []schema.GroupVersionResource {
	if gvr, ok := preferredServedGVR(served, known, includeLegacy); ok {
		return []schema.GroupVersionResource{gvr}
	}
	if served.Len() > 0 {
		// Versions are served but none is usable: the CRD serves only versions we do not
		// understand, or its only served version is disabled by includeLegacy. Unlike a kind
		// that is merely absent, this one is installed and still will not reconcile.
		logger.Warn("no usable served API version for route kind; its resources will not be reconciled",
			"resource", known[0].Resource,
			"served", served.UnsortedList(),
			"legacy_versions_enabled", includeLegacy,
		)
	}
	return slices.Clone(allowedRouteGVRs(known, includeLegacy))
}

// resolveRouteVersions is discoverRouteVersions with a bounded retry, for the startup
// resolution that cannot be revised later. It also owns the reporting, because it runs once
// per kind: discoverRouteVersions itself is silent so the poll loop can call it every round.
// A nil result means the versions could not be read, which is not the same as nothing being
// served -- but both leave selectRouteGVRs with nothing to narrow to, so the two need no
// distinguishing downstream.
func resolveRouteVersions(
	ctx context.Context,
	src routeVersionSource,
	crdName string,
	known []schema.GroupVersionResource,
) sets.Set[string] {
	var err error
	for attempt := 1; ; attempt++ {
		var served sets.Set[string]
		served, err = discoverRouteVersions(ctx, src, crdName, known)
		if err == nil {
			return served
		}
		if attempt >= startupResolveAttempts || !sleep.UntilContext(ctx, startupResolveDelay) {
			break
		}
	}

	logger.Error("could not resolve served API versions; building collections and status writers for every known API version until restart",
		"crd", crdName,
		"error", err,
	)
	return nil
}

// discoverRouteVersions reads which API versions a kind is currently served under, from the
// CRD if that read is permitted and from the discovery API otherwise. An error means we could
// not find out; an empty set means we found out that nothing is served.
func discoverRouteVersions(
	ctx context.Context,
	src routeVersionSource,
	crdName string,
	known []schema.GroupVersionResource,
) (sets.Set[string], error) {
	served, crdErr := servedVersionsFromCRD(ctx, src.ext, crdName)
	if crdErr == nil {
		return served, nil
	}

	// The CRD read is unavailable. Ask the API server what it actually serves instead: that
	// read needs only discovery permissions, so it is the one that survives an RBAC gap on
	// customresourcedefinitions.
	served, discoveryErr := servedVersionsFromDiscovery(src.disco, known)
	if discoveryErr != nil {
		return nil, fmt.Errorf("crd read: %w; discovery read: %w", crdErr, discoveryErr)
	}
	return served, nil
}

// servedVersionsFromCRD reads the served versions off the CRD itself.
func servedVersionsFromCRD(
	ctx context.Context,
	extClient apiextensionsclient.Interface,
	crdName string,
) (sets.Set[string], error) {
	if extClient == nil {
		return nil, errors.New("no CRD client")
	}

	ctx, cancel := context.WithTimeout(ctx, crdLookupTimeout)
	defer cancel()

	crd, err := extClient.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, crdName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		// A confirmed absence is an answer, not a failure: nothing is served.
		return sets.New[string](), nil
	}
	if err != nil {
		return nil, err
	}

	served := sets.New[string]()
	for _, version := range crd.Spec.Versions {
		if version.Served {
			served.Insert(version.Name)
		}
	}
	return served, nil
}

// servedVersionsFromDiscovery asks the API server which of the candidate group/versions
// actually serves the resource. It answers only when every candidate resolved either way:
// one unreadable candidate would otherwise look unserved, and the caller would narrow away
// from the version the cluster is really using.
func servedVersionsFromDiscovery(
	disco discovery.DiscoveryInterface,
	known []schema.GroupVersionResource,
) (sets.Set[string], error) {
	if disco == nil {
		return nil, errors.New("no discovery client")
	}

	served := sets.New[string]()
	for _, gvr := range known {
		groupVersion := gvr.GroupVersion().String()
		resources, err := disco.ServerResourcesForGroupVersion(groupVersion)
		switch {
		case err == nil:
		case apierrors.IsNotFound(err), meta.IsNoMatchError(err):
			// The group/version is not served at all: a definite no for this version.
			continue
		default:
			return nil, fmt.Errorf("group version %s: %w", groupVersion, err)
		}
		for _, resource := range resources.APIResources {
			if resource.Name == gvr.Resource {
				served.Insert(gvr.Version)
				break
			}
		}
	}
	return served, nil
}

// routeInformerGate decides whether the collection built for one candidate version is the
// one whose informer should run. Every candidate for a kind asks the same question of the
// same freshly read discovery result, so at most one can answer yes -- which is what keeps
// a cluster serving two understood versions from joining the same route twice.
func routeInformerGate(
	src routeVersionSource,
	crdName string,
	known []schema.GroupVersionResource,
	includeLegacy bool,
	self schema.GroupVersionResource,
) informerGate {
	return func(ctx context.Context) (bool, bool) {
		served, err := discoverRouteVersions(ctx, src, crdName, known)
		if err != nil {
			return false, false
		}
		gvr, ok := preferredServedGVR(served, known, includeLegacy)
		return ok && gvr == self, true
	}
}

// servedVersionGate is routeInformerGate for a kind with a single known version, where
// being served is the whole question.
func servedVersionGate(src routeVersionSource, gvr schema.GroupVersionResource) informerGate {
	return routeInformerGate(src, crdNameFor(gvr), []schema.GroupVersionResource{gvr}, true, gvr)
}

func crdNameFor(gvr schema.GroupVersionResource) string {
	return gvr.Resource + "." + gvr.Group
}

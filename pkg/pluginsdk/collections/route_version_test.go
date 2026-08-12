package collections

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/util/sets"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsfake "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	fakediscovery "k8s.io/client-go/discovery/fake"
	k8stesting "k8s.io/client-go/testing"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
)

// This table is the whole contract: for every discovery outcome and feature-flag setting, the
// versions we build collections for and the versions we write status through, which are the
// same list by construction.
func TestSelectRouteGVRs(t *testing.T) {
	tcpV1, tcpV1a2 := wellknown.TCPRouteV1GVR, wellknown.TCPRouteGVR
	tlsV1, tlsV1a3, tlsV1a2 := wellknown.TLSRouteV1GVR, wellknown.TLSRouteV1Alpha3GVR, wellknown.TLSRouteGVR

	tests := []struct {
		name          string
		served        sets.Set[string]
		known         []schema.GroupVersionResource
		includeLegacy bool
		want          []schema.GroupVersionResource
	}{
		{
			name:          "tcp: promoted served wins over legacy",
			served:        sets.New(gwv1.GroupVersion.Version, gwv1a2.GroupVersion.Version),
			known:         tcpRouteGVRs,
			includeLegacy: true,
			want:          []schema.GroupVersionResource{tcpV1},
		},
		{
			name:   "tcp: promoted served, legacy disabled",
			served: sets.New(gwv1.GroupVersion.Version, gwv1a2.GroupVersion.Version),
			known:  tcpRouteGVRs,
			want:   []schema.GroupVersionResource{tcpV1},
		},
		{
			name:          "tcp: only legacy served, legacy enabled",
			served:        sets.New(gwv1a2.GroupVersion.Version),
			known:         tcpRouteGVRs,
			includeLegacy: true,
			want:          []schema.GroupVersionResource{tcpV1a2},
		},
		{
			// The served version is not one we are allowed to use, so there is nothing to
			// reconcile, and with legacy disabled the promoted version is the only candidate
			// there could be.
			name:   "tcp: only legacy served but legacy disabled",
			served: sets.New(gwv1a2.GroupVersion.Version),
			known:  tcpRouteGVRs,
			want:   []schema.GroupVersionResource{tcpV1},
		},
		{
			// No version to commit to, so every candidate stays live: the cluster may be
			// mid-upgrade to a version we do understand, and that must not need a restart.
			name:          "tcp: nothing we understand is served",
			served:        sets.New("v1beta17"),
			known:         tcpRouteGVRs,
			includeLegacy: true,
			want:          []schema.GroupVersionResource{tcpV1, tcpV1a2},
		},
		{
			// The kind is simply not installed. Same reasoning: it may be installed later, at
			// whichever version, and each candidate's informer stays parked until then.
			name:          "tcp: nothing served at all",
			served:        sets.New[string](),
			known:         tcpRouteGVRs,
			includeLegacy: true,
			want:          []schema.GroupVersionResource{tcpV1, tcpV1a2},
		},
		{
			name:          "tcp: discovery failed, every allowed version stays a candidate",
			known:         tcpRouteGVRs,
			includeLegacy: true,
			want:          []schema.GroupVersionResource{tcpV1, tcpV1a2},
		},
		{
			name:  "tcp: discovery failed, legacy disabled",
			known: tcpRouteGVRs,
			want:  []schema.GroupVersionResource{tcpV1},
		},
		{
			name:          "tls: promoted served wins over both legacy versions",
			served:        sets.New(gwv1.GroupVersion.Version, wellknown.TLSRouteV1Alpha3Version),
			known:         tlsRouteGVRs,
			includeLegacy: true,
			want:          []schema.GroupVersionResource{tlsV1},
		},
		{
			// Gateway API v1.4.1 serves both pre-v1 versions; preference order settles on
			// the newer one so we do not watch the same route twice.
			name:          "tls: both legacy versions served prefers v1alpha3",
			served:        sets.New(wellknown.TLSRouteV1Alpha3Version, gwv1a2.GroupVersion.Version),
			known:         tlsRouteGVRs,
			includeLegacy: true,
			want:          []schema.GroupVersionResource{tlsV1a3},
		},
		{
			name:          "tls: only v1alpha2 served",
			served:        sets.New(gwv1a2.GroupVersion.Version),
			known:         tlsRouteGVRs,
			includeLegacy: true,
			want:          []schema.GroupVersionResource{tlsV1a2},
		},
		{
			name:   "tls: only v1alpha2 served but legacy disabled",
			served: sets.New(gwv1a2.GroupVersion.Version),
			known:  tlsRouteGVRs,
			want:   []schema.GroupVersionResource{tlsV1},
		},
		{
			name:          "tls: discovery failed, every allowed version stays a candidate",
			known:         tlsRouteGVRs,
			includeLegacy: true,
			want:          []schema.GroupVersionResource{tlsV1, tlsV1a3, tlsV1a2},
		},
		{
			name:  "tls: discovery failed, legacy disabled",
			known: tlsRouteGVRs,
			want:  []schema.GroupVersionResource{tlsV1},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, selectRouteGVRs(tc.served, tc.known, tc.includeLegacy))
		})
	}
}

// known is package-level state shared by every call, so the returned slice must never alias
// it in a way an append could reach.
func TestSelectRouteGVRsDoesNotAliasKnownVersions(t *testing.T) {
	known := []schema.GroupVersionResource{wellknown.TCPRouteV1GVR, wellknown.TCPRouteGVR}

	selected := selectRouteGVRs(nil, known, false)
	require.Len(t, selected, 1)
	//nolint:gocritic // appending to the result is exactly the misuse being guarded against
	_ = append(selected, wellknown.TLSRouteV1GVR)

	require.Equal(t, wellknown.TCPRouteGVR, known[1], "appending to the result must not overwrite the preference list")
}

// crdWithVersions builds an apiextensions client holding one CRD with the given versions.
func crdWithVersions(crdName string, versions ...apiextensionsv1.CustomResourceDefinitionVersion) *apiextensionsfake.Clientset {
	return apiextensionsfake.NewClientset(&apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: crdName},
		Spec:       apiextensionsv1.CustomResourceDefinitionSpec{Versions: versions},
	})
}

// unreadableCRDs is an apiextensions client whose reads are denied, standing in for the RBAC
// gap and the API server outage that both make the CRD read unusable.
func unreadableCRDs() *apiextensionsfake.Clientset {
	client := apiextensionsfake.NewClientset()
	client.PrependReactor("get", "customresourcedefinitions", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(
			schema.GroupResource{Group: "apiextensions.k8s.io", Resource: "customresourcedefinitions"}, "", errors.New("denied"))
	})
	return client
}

// servingDiscovery is a discovery client reporting the given resources per group/version.
func servingDiscovery(resources ...*metav1.APIResourceList) *fakediscovery.FakeDiscovery {
	return &fakediscovery.FakeDiscovery{Fake: &k8stesting.Fake{Resources: resources}}
}

func tlsRoutesIn(groupVersion string) *metav1.APIResourceList {
	return &metav1.APIResourceList{
		GroupVersion: groupVersion,
		APIResources: []metav1.APIResource{{Name: "gateways"}, {Name: "tlsroutes"}},
	}
}

func TestDiscoverRouteVersionsFromCRD(t *testing.T) {
	t.Run("reports every served version", func(t *testing.T) {
		src := routeVersionSource{ext: crdWithVersions(wellknown.TCPRouteCRDName,
			apiextensionsv1.CustomResourceDefinitionVersion{Name: gwv1a2.GroupVersion.Version, Served: true},
			apiextensionsv1.CustomResourceDefinitionVersion{Name: gwv1.GroupVersion.Version, Served: true},
		)}

		got, err := discoverRouteVersions(context.Background(), src, wellknown.TCPRouteCRDName, tcpRouteGVRs)
		require.NoError(t, err)
		require.Equal(t, sets.New(gwv1.GroupVersion.Version, gwv1a2.GroupVersion.Version), got)
	})

	t.Run("excludes versions that are not served", func(t *testing.T) {
		src := routeVersionSource{ext: crdWithVersions(wellknown.TLSRouteCRDName,
			apiextensionsv1.CustomResourceDefinitionVersion{Name: gwv1a2.GroupVersion.Version, Served: false},
			apiextensionsv1.CustomResourceDefinitionVersion{Name: gwv1.GroupVersion.Version, Served: true},
		)}

		got, err := discoverRouteVersions(context.Background(), src, wellknown.TLSRouteCRDName, tlsRouteGVRs)
		require.NoError(t, err)
		require.Equal(t, sets.New(gwv1.GroupVersion.Version), got)
	})

	t.Run("an absent CRD serves nothing, and that is an answer", func(t *testing.T) {
		// Distinct from an unreadable CRD only in that no error comes back. Selection treats
		// the two the same -- neither gives it a version to commit to -- but the informer gate
		// does not: an absence is a definite no, so it parks quietly instead of warning.
		src := routeVersionSource{ext: apiextensionsfake.NewClientset()}

		got, err := discoverRouteVersions(context.Background(), src, wellknown.TCPRouteCRDName, tcpRouteGVRs)
		require.NoError(t, err)
		require.Empty(t, got)
	})
}

// The discovery API needs only the system:discovery role every authenticated client already
// has, so it is the reader that still answers when the CRD read is denied.
func TestDiscoverRouteVersionsFallsBackToDiscoveryAPI(t *testing.T) {
	t.Run("resolves the served version the CRD read could not", func(t *testing.T) {
		src := routeVersionSource{
			ext:   unreadableCRDs(),
			disco: servingDiscovery(tlsRoutesIn(gwv1a2.GroupVersion.String())),
		}

		got, err := discoverRouteVersions(context.Background(), src, wellknown.TLSRouteCRDName, tlsRouteGVRs)
		require.NoError(t, err)
		require.Equal(t, sets.New(gwv1a2.GroupVersion.Version), got)
	})

	t.Run("a group version serving other resources does not count as serving this one", func(t *testing.T) {
		// gateway.networking.k8s.io/v1 exists on every cluster with HTTPRoute; that must not
		// be read as v1 TLSRoutes being served.
		src := routeVersionSource{
			ext: unreadableCRDs(),
			disco: servingDiscovery(
				&metav1.APIResourceList{
					GroupVersion: gwv1.GroupVersion.String(),
					APIResources: []metav1.APIResource{{Name: "httproutes"}},
				},
				tlsRoutesIn(gwv1a2.GroupVersion.String()),
			),
		}

		got, err := discoverRouteVersions(context.Background(), src, wellknown.TLSRouteCRDName, tlsRouteGVRs)
		require.NoError(t, err)
		require.Equal(t, sets.New(gwv1a2.GroupVersion.Version), got)
	})

	t.Run("both readers failing leaves the answer unresolved", func(t *testing.T) {
		disco := servingDiscovery()
		disco.PrependReactor("get", "resource", func(k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, apierrors.NewForbidden(schema.GroupResource{}, "", errors.New("denied"))
		})
		src := routeVersionSource{ext: unreadableCRDs(), disco: disco}

		_, err := discoverRouteVersions(context.Background(), src, wellknown.TLSRouteCRDName, tlsRouteGVRs)
		require.Error(t, err, "an unresolved answer must be reported, not reported as an absence")
	})

	t.Run("one unreadable candidate makes the whole answer unresolved", func(t *testing.T) {
		// A partial answer is worse than none: the unreadable version would look unserved,
		// and selection would narrow away from the version the cluster is really using.
		denied := 0
		disco := servingDiscovery(tlsRoutesIn(gwv1a2.GroupVersion.String()))
		disco.PrependReactor("get", "resource", func(k8stesting.Action) (bool, runtime.Object, error) {
			denied++
			if denied == 1 {
				return true, nil, apierrors.NewForbidden(schema.GroupResource{}, "", errors.New("denied"))
			}
			return false, nil, nil
		})
		src := routeVersionSource{ext: unreadableCRDs(), disco: disco}

		_, err := discoverRouteVersions(context.Background(), src, wellknown.TLSRouteCRDName, tlsRouteGVRs)
		require.Error(t, err)
	})

	t.Run("no readers at all leaves the answer unresolved", func(t *testing.T) {
		_, err := discoverRouteVersions(context.Background(), routeVersionSource{}, wellknown.TCPRouteCRDName, tcpRouteGVRs)
		require.Error(t, err)
	})
}

// The startup answer decides which collections exist and which versions status is written
// through for the life of the process, and cannot be revised, so it is the one read worth
// retrying past a brief failure.
func TestResolveRouteVersionsRetriesTransientFailures(t *testing.T) {
	client := crdWithVersions(wellknown.TCPRouteCRDName,
		apiextensionsv1.CustomResourceDefinitionVersion{Name: gwv1.GroupVersion.Version, Served: true},
	)
	attempts := 0
	client.PrependReactor("get", "customresourcedefinitions", func(k8stesting.Action) (bool, runtime.Object, error) {
		attempts++
		if attempts == 1 {
			return true, nil, apierrors.NewServiceUnavailable("apiserver is restarting")
		}
		return false, nil, nil
	})

	got := resolveRouteVersions(context.Background(), routeVersionSource{ext: client}, wellknown.TCPRouteCRDName, tcpRouteGVRs)
	require.Equal(t, sets.New(gwv1.GroupVersion.Version), got)
	require.Equal(t, 2, attempts)
}

func TestResolveRouteVersionsGivesUpAsUnresolved(t *testing.T) {
	got := resolveRouteVersions(context.Background(), routeVersionSource{ext: unreadableCRDs()}, wellknown.TCPRouteCRDName, tcpRouteGVRs)
	require.Nil(t, got, "exhausting the retries must leave every candidate live, not commit to one")
}

// Every candidate version for a kind asks the same question of the same discovery result, so
// at most one can answer yes -- which is what keeps a cluster serving two understood versions
// from joining the same route twice.
func TestRouteInformerGateStartsExactlyOneCandidate(t *testing.T) {
	// Gateway API v1.4.1 serves both pre-v1 TLSRoute versions.
	src := routeVersionSource{ext: crdWithVersions(wellknown.TLSRouteCRDName,
		apiextensionsv1.CustomResourceDefinitionVersion{Name: gwv1a2.GroupVersion.Version, Served: true},
		apiextensionsv1.CustomResourceDefinitionVersion{Name: wellknown.TLSRouteV1Alpha3Version, Served: true},
	)}

	started := make([]schema.GroupVersionResource, 0, len(tlsRouteGVRs))
	for _, gvr := range tlsRouteGVRs {
		start, resolved := routeInformerGate(src, wellknown.TLSRouteCRDName, tlsRouteGVRs, true, gvr)(context.Background())
		require.True(t, resolved)
		if start {
			started = append(started, gvr)
		}
	}

	require.Equal(t, []schema.GroupVersionResource{wellknown.TLSRouteV1Alpha3GVR}, started)
}

func TestRouteInformerGateDoesNotStartUnservedOrUnresolvedVersions(t *testing.T) {
	t.Run("authoritatively unserved", func(t *testing.T) {
		src := routeVersionSource{ext: crdWithVersions(wellknown.TLSRouteCRDName,
			apiextensionsv1.CustomResourceDefinitionVersion{Name: gwv1a2.GroupVersion.Version, Served: true},
		)}

		start, resolved := routeInformerGate(src, wellknown.TLSRouteCRDName, tlsRouteGVRs, true, wellknown.TLSRouteV1GVR)(context.Background())
		require.True(t, resolved)
		require.False(t, start)
	})

	t.Run("nothing usable is served", func(t *testing.T) {
		// selectRouteGVRs names the promoted version here so the writer has a stable
		// identity; the gate must still refuse to watch it.
		src := routeVersionSource{ext: crdWithVersions(wellknown.TLSRouteCRDName,
			apiextensionsv1.CustomResourceDefinitionVersion{Name: gwv1a2.GroupVersion.Version, Served: true},
		)}

		require.Equal(t, []schema.GroupVersionResource{wellknown.TLSRouteV1GVR},
			selectRouteGVRs(sets.New(gwv1a2.GroupVersion.Version), tlsRouteGVRs, false))

		start, resolved := routeInformerGate(src, wellknown.TLSRouteCRDName, tlsRouteGVRs, false, wellknown.TLSRouteV1GVR)(context.Background())
		require.True(t, resolved)
		require.False(t, start, "an unserved version's informer would 404 its initial list forever")
	})

	t.Run("unresolved", func(t *testing.T) {
		src := routeVersionSource{ext: unreadableCRDs()}

		start, resolved := routeInformerGate(src, wellknown.TLSRouteCRDName, tlsRouteGVRs, true, wellknown.TLSRouteV1GVR)(context.Background())
		require.False(t, resolved)
		require.False(t, start)
	})
}

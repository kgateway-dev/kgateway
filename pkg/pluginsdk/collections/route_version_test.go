package collections

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/util/sets"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsfake "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
)

func authoritative(versions ...string) routeVersionDiscovery {
	return routeVersionDiscovery{Authoritative: true, Served: sets.New(versions...)}
}

// This table is the whole contract: for every discovery outcome and feature-flag setting, the
// versions we watch and the versions we write status through, which are the same list by
// construction.
func TestSelectRouteGVRs(t *testing.T) {
	tcpV1, tcpV1a2 := wellknown.TCPRouteV1GVR, wellknown.TCPRouteGVR
	tlsV1, tlsV1a3, tlsV1a2 := wellknown.TLSRouteV1GVR, wellknown.TLSRouteV1Alpha3GVR, wellknown.TLSRouteGVR

	tests := []struct {
		name          string
		discovery     routeVersionDiscovery
		known         []schema.GroupVersionResource
		includeLegacy bool
		want          []schema.GroupVersionResource
	}{
		{
			name:          "tcp: promoted served wins over legacy",
			discovery:     authoritative(gwv1.GroupVersion.Version, gwv1a2.GroupVersion.Version),
			known:         tcpRouteGVRs,
			includeLegacy: true,
			want:          []schema.GroupVersionResource{tcpV1},
		},
		{
			name:      "tcp: promoted served, legacy disabled",
			discovery: authoritative(gwv1.GroupVersion.Version, gwv1a2.GroupVersion.Version),
			known:     tcpRouteGVRs,
			want:      []schema.GroupVersionResource{tcpV1},
		},
		{
			name:          "tcp: only legacy served, legacy enabled",
			discovery:     authoritative(gwv1a2.GroupVersion.Version),
			known:         tcpRouteGVRs,
			includeLegacy: true,
			want:          []schema.GroupVersionResource{tcpV1a2},
		},
		{
			// The version that is served is not one we are allowed to use, so there is
			// nothing to reconcile. Naming the promoted version keeps the watch and the
			// writer pointed at the same place; selecting the served-but-disabled version
			// would build a writer client for a version we deliberately do not watch.
			name:      "tcp: only legacy served but legacy disabled",
			discovery: authoritative(gwv1a2.GroupVersion.Version),
			known:     tcpRouteGVRs,
			want:      []schema.GroupVersionResource{tcpV1},
		},
		{
			name:          "tcp: nothing we understand is served",
			discovery:     authoritative("v1beta17"),
			known:         tcpRouteGVRs,
			includeLegacy: true,
			want:          []schema.GroupVersionResource{tcpV1},
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
			discovery:     authoritative(gwv1.GroupVersion.Version, wellknown.TLSRouteV1Alpha3Version),
			known:         tlsRouteGVRs,
			includeLegacy: true,
			want:          []schema.GroupVersionResource{tlsV1},
		},
		{
			// Gateway API v1.4.1 serves both pre-v1 versions; preference order settles on
			// the newer one so we do not watch the same route twice.
			name:          "tls: both legacy versions served prefers v1alpha3",
			discovery:     authoritative(wellknown.TLSRouteV1Alpha3Version, gwv1a2.GroupVersion.Version),
			known:         tlsRouteGVRs,
			includeLegacy: true,
			want:          []schema.GroupVersionResource{tlsV1a3},
		},
		{
			name:          "tls: only v1alpha2 served",
			discovery:     authoritative(gwv1a2.GroupVersion.Version),
			known:         tlsRouteGVRs,
			includeLegacy: true,
			want:          []schema.GroupVersionResource{tlsV1a2},
		},
		{
			name:      "tls: only v1alpha2 served but legacy disabled",
			discovery: authoritative(gwv1a2.GroupVersion.Version),
			known:     tlsRouteGVRs,
			want:      []schema.GroupVersionResource{tlsV1},
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
			require.Equal(t, tc.want, selectRouteGVRs(tc.discovery, tc.known, tc.includeLegacy))
		})
	}
}

// known is package-level state shared by every call, so the returned slice must never alias
// it in a way an append could reach.
func TestSelectRouteGVRsDoesNotAliasKnownVersions(t *testing.T) {
	known := []schema.GroupVersionResource{wellknown.TCPRouteV1GVR, wellknown.TCPRouteGVR}

	selected := selectRouteGVRs(routeVersionDiscovery{}, known, false)
	require.Len(t, selected, 1)
	//nolint:gocritic // appending to the result is exactly the misuse being guarded against
	_ = append(selected, wellknown.TLSRouteV1GVR)

	require.Equal(t, wellknown.TCPRouteGVR, known[1], "appending to the result must not overwrite the preference list")
}

func TestDiscoverRouteVersions(t *testing.T) {
	t.Run("reports every served version", func(t *testing.T) {
		client := apiextensionsfake.NewClientset(&apiextensionsv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: wellknown.TCPRouteCRDName},
			Spec: apiextensionsv1.CustomResourceDefinitionSpec{
				Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
					{Name: gwv1a2.GroupVersion.Version, Served: true},
					{Name: gwv1.GroupVersion.Version, Served: true},
				},
			},
		})

		require.Equal(t,
			routeVersionDiscovery{Authoritative: true, Served: sets.New(gwv1.GroupVersion.Version, gwv1a2.GroupVersion.Version)},
			discoverRouteVersions(context.Background(), client, wellknown.TCPRouteCRDName))
	})

	t.Run("excludes versions that are not served", func(t *testing.T) {
		client := apiextensionsfake.NewClientset(&apiextensionsv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: wellknown.TLSRouteCRDName},
			Spec: apiextensionsv1.CustomResourceDefinitionSpec{
				Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
					{Name: gwv1a2.GroupVersion.Version, Served: false},
					{Name: gwv1.GroupVersion.Version, Served: true},
				},
			},
		})

		require.Equal(t,
			routeVersionDiscovery{Authoritative: true, Served: sets.New(gwv1.GroupVersion.Version)},
			discoverRouteVersions(context.Background(), client, wellknown.TLSRouteCRDName))
	})

	t.Run("an absent CRD is not authoritative", func(t *testing.T) {
		// Distinct from "serves nothing": the CRD may be installed later, and selection must
		// keep every candidate alive rather than committing to a guess.
		require.Equal(t, routeVersionDiscovery{},
			discoverRouteVersions(context.Background(), apiextensionsfake.NewClientset(), wellknown.TCPRouteCRDName))
	})

	t.Run("no discovery client is not authoritative", func(t *testing.T) {
		require.Equal(t, routeVersionDiscovery{}, discoverRouteVersions(context.Background(), nil, wellknown.TCPRouteCRDName))
	})
}

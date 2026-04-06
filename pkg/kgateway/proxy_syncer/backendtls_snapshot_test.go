package proxy_syncer

import (
	"context"
	"strings"
	"testing"
	"time"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoywellknown "github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	apifake "github.com/kgateway-dev/kgateway/v2/pkg/apiclient/fake"
	backendtlsplugin "github.com/kgateway-dev/kgateway/v2/pkg/kgateway/extensions2/plugins/backendtlspolicy"
	k8splugin "github.com/kgateway-dev/kgateway/v2/pkg/kgateway/extensions2/plugins/kubernetes"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/extensions2/registry"
	kgtranslator "github.com/kgateway-dev/kgateway/v2/pkg/kgateway/translator"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/xds"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

func TestPerClientSnapshotUpdatesWhenBackendTLSPolicyConflictsAddedLater(t *testing.T) {
	ctx := t.Context()

	gatewayClass := &gwv1.GatewayClass{
		TypeMeta: metav1.TypeMeta{
			APIVersion: gwv1.GroupVersion.String(),
			Kind:       "GatewayClass",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "example-gateway-class",
		},
		Spec: gwv1.GatewayClassSpec{
			ControllerName: gwv1.GatewayController(wellknown.DefaultGatewayControllerName),
		},
	}
	gateway := &gwv1.Gateway{
		TypeMeta: metav1.TypeMeta{
			APIVersion: gwv1.GroupVersion.String(),
			Kind:       "Gateway",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-gateway",
			Namespace: "default",
		},
		Spec: gwv1.GatewaySpec{
			GatewayClassName: "example-gateway-class",
			Listeners: []gwv1.Listener{
				{
					Name:     "http",
					Protocol: gwv1.HTTPProtocolType,
					Port:     80,
				},
			},
		},
	}
	httpRoute := &gwv1.HTTPRoute{
		TypeMeta: metav1.TypeMeta{
			APIVersion: gwv1.GroupVersion.String(),
			Kind:       "HTTPRoute",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-route",
			Namespace: "default",
		},
		Spec: gwv1.HTTPRouteSpec{
			CommonRouteSpec: gwv1.CommonRouteSpec{
				ParentRefs: []gwv1.ParentReference{
					{Name: "example-gateway"},
				},
			},
			Hostnames: []gwv1.Hostname{"abc.example.com"},
			Rules: []gwv1.HTTPRouteRule{
				{
					Matches: []gwv1.HTTPRouteMatch{
						{
							Path: &gwv1.HTTPPathMatch{
								Type:  snapshotPtr(gwv1.PathMatchExact),
								Value: new("/backendtlspolicy-conflicted-without-section-name"),
							},
						},
					},
					BackendRefs: []gwv1.HTTPBackendRef{
						{
							BackendRef: gwv1.BackendRef{
								BackendObjectReference: gwv1.BackendObjectReference{
									Group: snapshotPtr(gwv1.Group("")),
									Kind:  snapshotPtr(gwv1.Kind("Service")),
									Name:  "backend-service",
									Port:  snapshotPtr(gwv1.PortNumber(443)),
								},
							},
						},
					},
				},
			},
		},
	}

	fakeClient := apifake.NewClient(
		t,
		gatewayClass,
		gateway,
		httpRoute,
		newActualBackendTLSTestService(),
		newActualBackendTLSTestConfigMap(),
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
	)
	settings := apisettings.Settings{EnableEnvoy: true}
	krtopts := krtutil.NewKrtOptions(ctx.Done(), nil)

	commoncol, err := collections.NewCommonCollections(
		ctx,
		krtopts,
		fakeClient,
		wellknown.DefaultGatewayControllerName,
		settings,
	)
	require.NoError(t, err)

	plugins := registry.MergePlugins(
		k8splugin.NewPlugin(ctx, commoncol),
		backendtlsplugin.NewPlugin(ctx, commoncol),
	)
	commoncol.InitPlugins(ctx, plugins, settings)

	translator := kgtranslator.NewCombinedTranslator(ctx, plugins, commoncol, nil)
	translator.Init(ctx)

	ucc := ir.NewUniqlyConnectedClient(
		xds.OwnerNamespaceNameID(wellknown.GatewayApiProxyValue, "default", "example-gateway"),
		"",
		nil,
		ir.PodLocality{},
	)
	uccs := krt.NewStaticCollection(nil, []ir.UniqlyConnectedClient{ucc}, krtopts.ToOptions("UniqueClients")...)
	finalBackends := krt.JoinCollection(
		commoncol.BackendIndex.BackendsWithPolicy(),
		append(krtopts.ToOptions("FinalBackends"), krt.WithJoinUnchecked())...,
	)
	finalEndpoints := newFinalBackendEndpoints(krtopts, finalBackends, commoncol.Endpoints)

	mostXdsSnapshots := krt.NewCollection(commoncol.GatewayIndex.Gateways, func(kctx krt.HandlerContext, gw ir.Gateway) *GatewayXdsResources {
		xdsSnap, reportsMap := translator.TranslateGateway(kctx, ctx, gw)
		if xdsSnap == nil {
			return nil
		}
		return toResources(gw, *xdsSnap, reportsMap)
	}, krtopts.ToOptions("MostXdsSnapshots")...)

	epPerClient := NewPerClientEnvoyEndpoints(
		krtopts,
		uccs,
		finalEndpoints,
		translator.TranslateEndpoints,
	)
	clustersPerClient := NewPerClientEnvoyClusters(
		ctx,
		krtopts,
		translator.GetBackendTranslator(),
		finalBackends,
		uccs,
	)
	snapshots := snapshotPerClient(krtopts, uccs, mostXdsSnapshots, epPerClient, clustersPerClient)

	fakeClient.RunAndWait(ctx.Done())

	require.Eventually(t, func() bool {
		return commoncol.HasSynced() &&
			plugins.HasSynced() &&
			translator.HasSynced() &&
			finalBackends.HasSynced() &&
			mostXdsSnapshots.HasSynced() &&
			snapshots.HasSynced()
	}, 5*time.Second, 50*time.Millisecond)

	require.Eventually(t, func() bool {
		cluster := fetchClusterFromSnapshot(krt.TestingDummyContext{}, snapshots, ucc.ResourceName(), "kube_default_backend-service_443")
		return cluster != nil && cluster.GetTransportSocket() == nil
	}, 5*time.Second, 50*time.Millisecond)

	older := time.Now()
	_, err = fakeClient.GatewayAPI().GatewayV1().BackendTLSPolicies("default").Create(
		ctx,
		newActualBackendTLSPolicy("backend-tls-older", "other.example.com", older),
		metav1.CreateOptions{},
	)
	require.NoError(t, err)
	_, err = fakeClient.GatewayAPI().GatewayV1().BackendTLSPolicies("default").Create(
		ctx,
		newActualBackendTLSPolicy("backend-tls-newer", "abc.example.com", older.Add(time.Second)),
		metav1.CreateOptions{},
	)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		_, cluster := fetchNonBaseClusterFromSnapshotWithPrefix(
			krt.TestingDummyContext{},
			snapshots,
			ucc.ResourceName(),
			"kube_default_backend-service_",
			"kube_default_backend-service_443",
		)
		if cluster == nil {
			return false
		}
		transportSocket := cluster.GetTransportSocket()
		if transportSocket == nil || transportSocket.GetName() != envoywellknown.TransportSocketTls {
			return false
		}
		tlsContext := &envoytlsv3.UpstreamTlsContext{}
		if err := transportSocket.GetTypedConfig().UnmarshalTo(tlsContext); err != nil {
			return false
		}
		return tlsContext.GetSni() == "other.example.com"
	}, 5*time.Second, 50*time.Millisecond)
}

func fetchClusterFromSnapshot(kctx krt.HandlerContext, snapshots krt.Collection[XdsSnapWrapper], snapshotKey, clusterName string) *envoyclusterv3.Cluster {
	snap := krt.FetchOne(kctx, snapshots, krt.FilterKey(snapshotKey))
	if snap == nil {
		return nil
	}
	res, ok := snap.snap.Resources[envoycachetypes.Cluster].Items[clusterName]
	if !ok {
		return nil
	}
	return res.Resource.(*envoyclusterv3.Cluster)
}

func fetchNonBaseClusterFromSnapshotWithPrefix(
	kctx krt.HandlerContext,
	snapshots krt.Collection[XdsSnapWrapper],
	snapshotKey, clusterNamePrefix, baseClusterName string,
) (string, *envoyclusterv3.Cluster) {
	snap := krt.FetchOne(kctx, snapshots, krt.FilterKey(snapshotKey))
	if snap == nil {
		return "", nil
	}
	for name, res := range snap.snap.Resources[envoycachetypes.Cluster].Items {
		if !strings.HasPrefix(name, clusterNamePrefix) || name == baseClusterName {
			continue
		}
		cluster, ok := res.Resource.(*envoyclusterv3.Cluster)
		if !ok {
			continue
		}
		return name, cluster
	}
	return "", nil
}

//go:fix inline
func stringPtr(in string) *string {
	return new(in)
}

//go:fix inline
func snapshotPtr[T any](in T) *T {
	return new(in)
}

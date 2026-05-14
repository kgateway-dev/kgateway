package proxy_syncer

import (
	"testing"
	"time"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoywellknown "github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
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
		newFinalBackendEndpoints(krtopts, finalBackends, commoncol.Endpoints),
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
		cluster := fetchClusterFromSnapshot(snapshots, ucc.ResourceName(), "kube_default_backend-service_443")
		return cluster != nil && cluster.GetTransportSocket() == nil
	}, 5*time.Second, 50*time.Millisecond)
	initialEndpointVersion := snapshotEndpointVersion(snapshots, ucc.ResourceName())
	require.NotEmpty(t, initialEndpointVersion)

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
		clusterName := "kube_default_backend-service_443"
		cluster := fetchClusterFromSnapshot(snapshots, ucc.ResourceName(), clusterName)
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
		if tlsContext.GetSni() != "other.example.com" {
			return false
		}
		return fetchEndpointFromSnapshot(snapshots, ucc.ResourceName(), clusterName) != nil &&
			snapshotEndpointVersion(snapshots, ucc.ResourceName()) != initialEndpointVersion
	}, 5*time.Second, 50*time.Millisecond)
}

func TestPerClientSnapshotUsesSectionSpecificAndServiceWideBackendTLSPolicies(t *testing.T) {
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
			Listeners: []gwv1.Listener{{
				Name:     "http",
				Protocol: gwv1.HTTPProtocolType,
				Port:     80,
			}},
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
				ParentRefs: []gwv1.ParentReference{{Name: "example-gateway"}},
			},
			Hostnames: []gwv1.Hostname{"abc.example.com"},
			Rules: []gwv1.HTTPRouteRule{
				{
					Matches: []gwv1.HTTPRouteMatch{{
						Path: &gwv1.HTTPPathMatch{
							Type:  snapshotPtr(gwv1.PathMatchExact),
							Value: new("/with-section-name"),
						},
					}},
					BackendRefs: []gwv1.HTTPBackendRef{{
						BackendRef: gwv1.BackendRef{
							BackendObjectReference: gwv1.BackendObjectReference{
								Group: snapshotPtr(gwv1.Group("")),
								Kind:  snapshotPtr(gwv1.Kind("Service")),
								Name:  "backend-service",
								Port:  snapshotPtr(gwv1.PortNumber(443)),
							},
						},
					}},
				},
				{
					Matches: []gwv1.HTTPRouteMatch{{
						Path: &gwv1.HTTPPathMatch{
							Type:  snapshotPtr(gwv1.PathMatchExact),
							Value: new("/without-section-name"),
						},
					}},
					BackendRefs: []gwv1.HTTPBackendRef{{
						BackendRef: gwv1.BackendRef{
							BackendObjectReference: gwv1.BackendObjectReference{
								Group: snapshotPtr(gwv1.Group("")),
								Kind:  snapshotPtr(gwv1.Kind("Service")),
								Name:  "backend-service",
								Port:  snapshotPtr(gwv1.PortNumber(8443)),
							},
						},
					}},
				},
			},
		},
	}

	service := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "backend-service",
			Namespace: "default",
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       "https-1",
					Port:       443,
					TargetPort: intstr.FromInt(8443),
				},
				{
					Name:       "https-2",
					Port:       8443,
					TargetPort: intstr.FromInt(8443),
				},
			},
		},
	}

	serviceWidePolicy := newActualBackendTLSPolicy("service-wide", "abc.example.com", time.Now())
	serviceWidePolicy.Spec.TargetRefs[0].SectionName = nil
	sectionSpecificPolicy := newActualBackendTLSPolicy("section-specific", "other.example.com", time.Now().Add(time.Second))
	sectionSpecificPolicy.Spec.TargetRefs[0].SectionName = snapshotPtr(gwv1.SectionName("https-1"))

	fakeClient := apifake.NewClient(
		t,
		gatewayClass,
		gateway,
		httpRoute,
		service,
		newActualBackendTLSTestConfigMap(),
		serviceWidePolicy,
		sectionSpecificPolicy,
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
		newFinalBackendEndpoints(krtopts, finalBackends, commoncol.Endpoints),
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
		cluster443 := fetchClusterFromSnapshot(snapshots, ucc.ResourceName(), "kube_default_backend-service_443")
		cluster8443 := fetchClusterFromSnapshot(snapshots, ucc.ResourceName(), "kube_default_backend-service_8443")
		if cluster443 == nil || cluster8443 == nil {
			return false
		}
		sni443, ok := upstreamTLSSNI(cluster443)
		if !ok || sni443 != "other.example.com" {
			return false
		}
		sni8443, ok := upstreamTLSSNI(cluster8443)
		if !ok || sni8443 != "abc.example.com" {
			return false
		}
		return fetchEndpointFromSnapshot(snapshots, ucc.ResourceName(), "kube_default_backend-service_443") != nil &&
			fetchEndpointFromSnapshot(snapshots, ucc.ResourceName(), "kube_default_backend-service_8443") != nil
	}, 5*time.Second, 50*time.Millisecond)
}

func upstreamTLSSNI(cluster *envoyclusterv3.Cluster) (string, bool) {
	transportSocket := cluster.GetTransportSocket()
	if transportSocket == nil || transportSocket.GetName() != envoywellknown.TransportSocketTls {
		return "", false
	}
	tlsContext := &envoytlsv3.UpstreamTlsContext{}
	if err := transportSocket.GetTypedConfig().UnmarshalTo(tlsContext); err != nil {
		return "", false
	}
	return tlsContext.GetSni(), true
}

func fetchClusterFromSnapshot(snapshots krt.Collection[XdsSnapWrapper], snapshotKey, clusterName string) *envoyclusterv3.Cluster {
	snap := krt.FetchOne(krt.TestingDummyContext{}, snapshots, krt.FilterKey(snapshotKey))
	if snap == nil {
		return nil
	}
	res, ok := snap.snap.Resources[envoycachetypes.Cluster].Items[clusterName]
	if !ok {
		return nil
	}
	return res.Resource.(*envoyclusterv3.Cluster)
}

func fetchEndpointFromSnapshot(
	snapshots krt.Collection[XdsSnapWrapper],
	snapshotKey, clusterName string,
) *envoyendpointv3.ClusterLoadAssignment {
	snap := krt.FetchOne(krt.TestingDummyContext{}, snapshots, krt.FilterKey(snapshotKey))
	if snap == nil {
		return nil
	}
	res, ok := snap.snap.Resources[envoycachetypes.Endpoint].Items[clusterName]
	if !ok {
		return nil
	}
	endpoint, ok := res.Resource.(*envoyendpointv3.ClusterLoadAssignment)
	if !ok {
		return nil
	}
	return endpoint
}

func snapshotEndpointVersion(snapshots krt.Collection[XdsSnapWrapper], snapshotKey string) string {
	snap := krt.FetchOne(krt.TestingDummyContext{}, snapshots, krt.FilterKey(snapshotKey))
	if snap == nil {
		return ""
	}
	return snap.snap.Resources[envoycachetypes.Endpoint].Version
}

//go:fix inline
func snapshotPtr[T any](in T) *T {
	return new(in)
}

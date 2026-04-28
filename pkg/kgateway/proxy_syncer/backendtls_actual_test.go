package proxy_syncer

import (
	"testing"
	"time"

	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoywellknown "github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	apifake "github.com/kgateway-dev/kgateway/v2/pkg/apiclient/fake"
	backendtlsplugin "github.com/kgateway-dev/kgateway/v2/pkg/kgateway/extensions2/plugins/backendtlspolicy"
	k8splugin "github.com/kgateway-dev/kgateway/v2/pkg/kgateway/extensions2/plugins/kubernetes"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/extensions2/registry"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/translator/irtranslator"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

const backendTLSConformanceCACert = `-----BEGIN CERTIFICATE-----
MIIC1jCCAb4CCQCJczLyBBZ1GTANBgkqhkiG9w0BAQsFADAtMRUwEwYDVQQKDAxl
eGFtcGxlIEluYy4xFDASBgNVBAMMC2V4YW1wbGUuY29tMB4XDTI1MDMwNzE0Mjkx
NloXDTI2MDMwNzE0MjkxNlowLTEVMBMGA1UECgwMZXhhbXBsZSBJbmMuMRQwEgYD
VQQDDAtleGFtcGxlLmNvbTCCASIwDQYJKoZIhvcNAQEBBQADggEPADCCAQoCggEB
AN0U6TVYECkwqnxh1Kt3dS+LialrXBOXKagj9tE582T6dwmqThD75VZPrNKkRoYO
aUzCctfDkUBXRemOTMut7ES5xoAtSAhr2GAnqgM3+yBCLOxooSjEFdlpFT7dhi1w
jOPa5iMh6ve/pHuRHvEuaF/J6P8tr83wGutx/xFZVuGA9V1AmBmYhePM+JhdcwaB
1+IbJp30gGyPfY4vdRQ9VQWbThE8psEzah+3SgTKJSIT7NAdwiIu3O3rXORbaYYU
oycgXUHdOKRbJnbvy3pTnFZJ50sg1HIA4yBdX7c0diy8Zz3Suoondg3DforWr0pB
Hs6tySAQoz2RiAqDqcE2rbMCAwEAATANBgkqhkiG9w0BAQsFAAOCAQEAWPkz3dJW
b+LFtnv7MlOVM79Y4PqeiHnazP1G9FwnWBHARkjISsax3b0zX8/RHnU83c3tLP5D
VwenYb9B9mzXbLiWI8aaX0UXP//D593ti15y0Od7yC2hQszlqIbxYnkFVwXoT9fQ
bdQ9OtpCt8EZnKEyCxck+hlKEyYTcH2PqZ7Ndp0M8I2znz3Kut/uYHLUddfoPF/m
O0V6fbyB/Mx/G1uLiv/BVpx3AdP+3ygJyKtelXkD+IdlY3y110fzmVr6NgxAbz/h
n9KpuK4SEloIycZUaKVXAaX7T42SFYw7msmB+Uu7z5oLOijsjX6TjeofdFBZ/Byl
SxODgqhtaPnOxQ==
-----END CERTIFICATE-----`

func newActualBackendTLSTestService() *corev1.Service {
	return &corev1.Service{
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
					Name:       "https",
					Port:       443,
					TargetPort: intstr.FromInt(8443),
				},
			},
		},
	}
}

func newActualBackendTLSTestConfigMap() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tls-checks-ca-certificate",
			Namespace: "default",
		},
		Data: map[string]string{
			"ca.crt": backendTLSConformanceCACert,
		},
	}
}

func newActualBackendTLSPolicy(name, hostname string, creationTime time.Time) *gwv1.BackendTLSPolicy {
	return &gwv1.BackendTLSPolicy{
		TypeMeta: metav1.TypeMeta{
			APIVersion: gwv1.GroupVersion.String(),
			Kind:       wellknown.BackendTLSPolicyGVK.Kind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         "default",
			CreationTimestamp: metav1.NewTime(creationTime),
			Generation:        1,
		},
		Spec: gwv1.BackendTLSPolicySpec{
			TargetRefs: []gwv1.LocalPolicyTargetReferenceWithSectionName{
				{
					LocalPolicyTargetReference: gwv1.LocalPolicyTargetReference{
						Group: "",
						Kind:  "Service",
						Name:  "backend-service",
					},
				},
			},
			Validation: gwv1.BackendTLSPolicyValidation{
				CACertificateRefs: []gwv1.LocalObjectReference{
					{
						Group: "",
						Kind:  "ConfigMap",
						Name:  "tls-checks-ca-certificate",
					},
				},
				Hostname: gwv1.PreciseHostname(hostname),
			},
		},
	}
}

func TestPerClientClustersUpdateWhenActualBackendTLSPolicyAddedLater(t *testing.T) {
	ctx := t.Context()

	service := newActualBackendTLSTestService()
	configMap := newActualBackendTLSTestConfigMap()

	fakeClient := apifake.NewClient(t, service, configMap)
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

	translator := &irtranslator.BackendTranslator{
		ContributedBackends: make(map[schema.GroupKind]ir.BackendInit),
		ContributedPolicies: plugins.ContributesPolicies,
		CommonCols:          commoncol,
		Mode:                settings.ValidationMode,
	}
	for gk, backendPlugin := range plugins.ContributesBackends {
		translator.ContributedBackends[gk] = backendPlugin.BackendInit
	}

	ucc := ir.NewUniqlyConnectedClient("test-role", "", nil, ir.PodLocality{})
	uccs := krt.NewStaticCollection(nil, []ir.UniqlyConnectedClient{ucc}, krtopts.ToOptions("UniqueClients")...)
	finalBackends := krt.JoinCollection(
		commoncol.BackendIndex.BackendsWithPolicy(),
		append(krtopts.ToOptions("FinalBackends"), krt.WithJoinUnchecked())...,
	)
	clusters := NewPerClientEnvoyClusters(ctx, krtopts, translator, finalBackends, uccs)

	fakeClient.RunAndWait(ctx.Done())

	require.Eventually(t, func() bool {
		return commoncol.HasSynced() &&
			commoncol.BackendIndex.HasSynced() &&
			plugins.HasSynced() &&
			finalBackends.HasSynced() &&
			clusters.clusters.HasSynced()
	}, 5*time.Second, 50*time.Millisecond)

	require.Eventually(t, func() bool {
		fetched := clusters.FetchClustersForClient(krt.TestingDummyContext{}, ucc)
		return len(fetched) == 1 && fetched[0].Cluster != nil && fetched[0].Cluster.GetTransportSocket() == nil
	}, 5*time.Second, 50*time.Millisecond)

	older := time.Now()
	policy1 := newActualBackendTLSPolicy("backend-tls-older", "other.example.com", older)
	_, err = fakeClient.GatewayAPI().GatewayV1().BackendTLSPolicies("default").Create(ctx, policy1, metav1.CreateOptions{})
	require.NoError(t, err)

	policy2 := newActualBackendTLSPolicy("backend-tls-newer", "abc.example.com", older.Add(time.Second))
	_, err = fakeClient.GatewayAPI().GatewayV1().BackendTLSPolicies("default").Create(ctx, policy2, metav1.CreateOptions{})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		fetched := clusters.FetchClustersForClient(krt.TestingDummyContext{}, ucc)
		if len(fetched) != 1 || fetched[0].Cluster == nil {
			return false
		}

		transportSocket := fetched[0].Cluster.GetTransportSocket()
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

func TestPerClientClustersUseActualBackendTLSPolicyWhenConflictsExistAtStartup(t *testing.T) {
	ctx := t.Context()

	older := time.Now()
	fakeClient := apifake.NewClient(
		t,
		newActualBackendTLSTestService(),
		newActualBackendTLSTestConfigMap(),
		newActualBackendTLSPolicy("backend-tls-older", "other.example.com", older),
		newActualBackendTLSPolicy("backend-tls-newer", "abc.example.com", older.Add(time.Second)),
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

	translator := &irtranslator.BackendTranslator{
		ContributedBackends: make(map[schema.GroupKind]ir.BackendInit),
		ContributedPolicies: plugins.ContributesPolicies,
		CommonCols:          commoncol,
		Mode:                settings.ValidationMode,
	}
	for gk, backendPlugin := range plugins.ContributesBackends {
		translator.ContributedBackends[gk] = backendPlugin.BackendInit
	}

	ucc := ir.NewUniqlyConnectedClient("test-role", "", nil, ir.PodLocality{})
	uccs := krt.NewStaticCollection(nil, []ir.UniqlyConnectedClient{ucc}, krtopts.ToOptions("UniqueClients")...)
	finalBackends := krt.JoinCollection(
		commoncol.BackendIndex.BackendsWithPolicy(),
		append(krtopts.ToOptions("FinalBackends"), krt.WithJoinUnchecked())...,
	)
	clusters := NewPerClientEnvoyClusters(ctx, krtopts, translator, finalBackends, uccs)

	fakeClient.RunAndWait(ctx.Done())

	require.Eventually(t, func() bool {
		return commoncol.HasSynced() &&
			commoncol.BackendIndex.HasSynced() &&
			plugins.HasSynced() &&
			finalBackends.HasSynced() &&
			clusters.clusters.HasSynced()
	}, 5*time.Second, 50*time.Millisecond)

	require.Eventually(t, func() bool {
		fetched := clusters.FetchClustersForClient(krt.TestingDummyContext{}, ucc)
		if len(fetched) != 1 || fetched[0].Cluster == nil {
			return false
		}
		if fetched[0].Name != "kube_default_backend-service_443" {
			return false
		}

		transportSocket := fetched[0].Cluster.GetTransportSocket()
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

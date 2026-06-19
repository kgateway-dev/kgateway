package irtranslator_test

import (
	"context"
	"testing"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/endpoints"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/translator/irtranslator"
	kgwellknown "github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func TestOrderedEndpointPluginsUsesStablePolicyOrder(t *testing.T) {
	var calls []string
	plugins := irtranslator.OrderedEndpointPlugins(sdk.ContributesPolicies{
		{Group: "gateway.kgateway.dev", Kind: "BackendConfigPolicy"}: {
			Name:                      "BackendConfigPolicy",
			PerClientProcessEndpoints: recordingEndpointPlugin("backendconfigpolicy", &calls),
		},
		{Group: "networking.istio.io", Kind: "DestinationRule"}: {
			Name:                      "destrule",
			PerClientProcessEndpoints: recordingEndpointPlugin("destrule", &calls),
		},
		{Group: "example.io", Kind: "ExamplePolicy"}: {
			Name:                      "example",
			PerClientProcessEndpoints: recordingEndpointPlugin("example", &calls),
		},
		{Group: "example.io", Kind: "NoEndpointPolicy"}: {
			Name: "no-endpoint",
		},
	})

	require.Len(t, plugins, 3)
	for _, plugin := range plugins {
		plugin(krt.TestingDummyContext{}, context.Background(), ir.UniquelyConnectedClient{}, &endpoints.EndpointsInputs{})
	}

	assert.Equal(t, []string{"example", "backendconfigpolicy", "destrule"}, calls)
}

func TestBackendTranslatorRunsOrderedEndpointPluginsForInlineEndpoints(t *testing.T) {
	backendGK := schema.GroupKind{Group: "example.io", Kind: "InlineBackend"}
	var calls []string
	translator := &irtranslator.BackendTranslator{
		ContributedBackends: map[schema.GroupKind]ir.BackendInit{
			backendGK: {
				InitEnvoyBackend: func(ctx context.Context, in ir.BackendObjectIR, out *envoyclusterv3.Cluster) *ir.EndpointsForBackend {
					out.ClusterDiscoveryType = &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_STATIC}
					eps := ir.NewEndpointsForBackend(in)
					eps.Add(ir.PodLocality{Region: "region-1", Zone: "zone-a"}, endpointWithLabels("10.0.0.1", map[string]string{
						corev1.LabelTopologyRegion: "region-1",
						corev1.LabelTopologyZone:   "zone-a",
					}))
					eps.Add(ir.PodLocality{Region: "region-1", Zone: "zone-b"}, endpointWithLabels("10.0.0.2", map[string]string{
						corev1.LabelTopologyRegion: "region-1",
						corev1.LabelTopologyZone:   "zone-b",
					}))
					return eps
				},
			},
		},
		ContributedPolicies: sdk.ContributesPolicies{
			kgwellknown.BackendConfigPolicyGVK.GroupKind(): {
				Name: "BackendConfigPolicy",
				PerClientProcessEndpoints: func(kctx krt.HandlerContext, ctx context.Context, ucc ir.UniquelyConnectedClient, out *endpoints.EndpointsInputs) uint64 {
					calls = append(calls, "backendconfigpolicy")
					out.PriorityInfo = &endpoints.PriorityInfo{
						FailoverPriority: endpoints.NewPriorities([]string{corev1.LabelTopologyZone + "=zone-a"}),
					}
					return 0
				},
			},
			{Group: "networking.istio.io", Kind: "DestinationRule"}: {
				Name: "destrule",
				PerClientProcessEndpoints: func(kctx krt.HandlerContext, ctx context.Context, ucc ir.UniquelyConnectedClient, out *endpoints.EndpointsInputs) uint64 {
					calls = append(calls, "destrule")
					out.PriorityInfo = &endpoints.PriorityInfo{
						FailoverPriority: endpoints.NewPriorities([]string{corev1.LabelTopologyRegion}),
					}
					return 0
				},
			},
		},
	}
	backend := ir.NewBackendObjectIR(ir.ObjectSource{
		Group:     backendGK.Group,
		Kind:      backendGK.Kind,
		Namespace: "default",
		Name:      "inline",
	}, 80, "", "")
	backendPtr := &backend
	ucc := ir.UniquelyConnectedClient{
		Locality: ir.PodLocality{Region: "region-1", Zone: "zone-a"},
		Labels: map[string]string{
			corev1.LabelTopologyRegion: "region-1",
			corev1.LabelTopologyZone:   "zone-a",
		},
	}

	cluster, err := translator.TranslateBackend(context.Background(), krt.TestingDummyContext{}, ucc, backendPtr)
	require.NoError(t, err)
	assert.Equal(t, []string{"backendconfigpolicy", "destrule"}, calls)

	assignment := cluster.GetLoadAssignment()
	require.NotNil(t, assignment)
	prioritiesByZone := map[string]uint32{}
	for _, localityEndpoints := range assignment.GetEndpoints() {
		prioritiesByZone[localityEndpoints.GetLocality().GetZone()] = localityEndpoints.GetPriority()
	}
	assert.Equal(t, map[string]uint32{"zone-a": 0, "zone-b": 0}, prioritiesByZone)
}

func recordingEndpointPlugin(name string, calls *[]string) sdk.EndpointPlugin {
	return func(
		kctx krt.HandlerContext,
		ctx context.Context,
		ucc ir.UniquelyConnectedClient,
		out *endpoints.EndpointsInputs,
	) uint64 {
		*calls = append(*calls, name)
		return 0
	}
}

func endpointWithLabels(address string, labels map[string]string) ir.EndpointWithMd {
	return ir.EndpointWithMd{
		LbEndpoint: &envoyendpointv3.LbEndpoint{
			HostIdentifier: &envoyendpointv3.LbEndpoint_Endpoint{
				Endpoint: &envoyendpointv3.Endpoint{
					Address: &envoycorev3.Address{
						Address: &envoycorev3.Address_SocketAddress{
							SocketAddress: &envoycorev3.SocketAddress{Address: address},
						},
					},
				},
			},
		},
		EndpointMd: ir.EndpointMetadata{Labels: labels},
	}
}

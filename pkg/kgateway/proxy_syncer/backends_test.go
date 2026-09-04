package proxy_syncer

import (
	"errors"
	"testing"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/endpoints"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/translator/irtranslator"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

// TestBaseClusterVersion_InlineCLAReflectsEndpointChanges is a regression test
// for the stale-CLA bug. When the cluster type supports an inline CLA, the
// per-client CLA is built from BaseCluster.EndpointInputs (not from the cluster
// proto), so the base version must change when endpoints change — otherwise
// KRT does not re-publish the base, the delta collection does not recompute,
// and clients stay pinned to a stale LoadAssignment.
//
// The bug surfaces on non-EDS backends (e.g. ServiceEntry-style STRICT_DNS)
// where addresses live in EndpointsForBackend rather than the cluster proto.
func TestBaseClusterVersion_InlineCLAReflectsEndpointChanges(t *testing.T) {
	cluster := staticInlineCLACluster()
	us := ir.NewBackendObjectIR(ir.ObjectSource{Namespace: "ns", Name: "svc"}, 0, "", "")

	withNoEps := &irtranslator.BaseCluster{
		Cluster:           cluster,
		EndpointInputs:    &endpoints.EndpointsInputs{EndpointsForBackend: *ir.NewEndpointsForBackend(us)},
		SupportsInlineCLA: true,
	}
	v0 := baseClusterVersion(&us, withNoEps)

	withOneEp := *withNoEps
	epsOne := *ir.NewEndpointsForBackend(us)
	epsOne.Add(ir.PodLocality{Region: "r1"}, ir.EndpointWithMd{
		LbEndpoint: lbEndpointPipe("a"),
	})
	withOneEp.EndpointInputs = &endpoints.EndpointsInputs{EndpointsForBackend: epsOne}
	v1 := baseClusterVersion(&us, &withOneEp)

	withTwoEps := *withNoEps
	epsTwo := *ir.NewEndpointsForBackend(us)
	epsTwo.Add(ir.PodLocality{Region: "r1"}, ir.EndpointWithMd{
		LbEndpoint: lbEndpointPipe("a"),
	})
	epsTwo.Add(ir.PodLocality{Region: "r2"}, ir.EndpointWithMd{
		LbEndpoint: lbEndpointPipe("b"),
	})
	withTwoEps.EndpointInputs = &endpoints.EndpointsInputs{EndpointsForBackend: epsTwo}
	v2 := baseClusterVersion(&us, &withTwoEps)

	require.NotEqual(t, v0, v1, "adding an endpoint to an inline-CLA backend must change the base version")
	require.NotEqual(t, v1, v2, "adding another endpoint must change the base version again")
}

// TestBaseClusterVersion_EDSStableAcrossEndpointChanges asserts that endpoint
// churn does NOT churn the base version for EDS clusters. EDS endpoints feed a
// separate pipeline and are not used by ApplyPerClient; if the version flipped
// here we would needlessly recompute every delta on every endpoint change.
func TestBaseClusterVersion_EDSStableAcrossEndpointChanges(t *testing.T) {
	cluster := &envoyclusterv3.Cluster{
		Name: "eds-cluster",
		ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{
			Type: envoyclusterv3.Cluster_EDS,
		},
	}
	us := ir.NewBackendObjectIR(ir.ObjectSource{Namespace: "ns", Name: "svc"}, 0, "", "")

	v0 := baseClusterVersion(&us, &irtranslator.BaseCluster{
		Cluster:           cluster,
		EndpointInputs:    &endpoints.EndpointsInputs{EndpointsForBackend: *ir.NewEndpointsForBackend(us)},
		SupportsInlineCLA: false,
	})

	withEp := *ir.NewEndpointsForBackend(us)
	withEp.Add(ir.PodLocality{Region: "r1"}, ir.EndpointWithMd{
		LbEndpoint: lbEndpointPipe("a"),
	})
	v1 := baseClusterVersion(&us, &irtranslator.BaseCluster{
		Cluster:           cluster,
		EndpointInputs:    &endpoints.EndpointsInputs{EndpointsForBackend: withEp},
		SupportsInlineCLA: false,
	})

	require.Equal(t, v0, v1, "EDS base version must not depend on endpoint changes")
}

// TestBaseClusterVersion_ErroredHasZeroVersion confirms that errored bases
// produce a stable zero version (matching the original HashProto-only path
// where errored bases were skipped). Two errored bases compare equal so KRT
// does not churn on transient error reasons.
func TestBaseClusterVersion_ErroredHasZeroVersion(t *testing.T) {
	cluster := staticInlineCLACluster()
	us := ir.NewBackendObjectIR(ir.ObjectSource{Namespace: "ns", Name: "svc"}, 0, "", "")
	v := baseClusterVersion(&us, &irtranslator.BaseCluster{
		Cluster: cluster,
		Error:   errors.New("boom"),
	})
	require.Equal(t, uint64(0), v)
}

// TestBaseClusterVersion_ClusterChangesAreReflected sanity-checks that a
// different cluster proto produces a different version regardless of endpoints.
func TestBaseClusterVersion_ClusterChangesAreReflected(t *testing.T) {
	us := ir.NewBackendObjectIR(ir.ObjectSource{Namespace: "ns", Name: "svc"}, 0, "", "")
	epsIn := &endpoints.EndpointsInputs{EndpointsForBackend: *ir.NewEndpointsForBackend(us)}

	clusterA := staticInlineCLACluster()
	clusterB := staticInlineCLACluster()
	clusterB.Name = "different-name"

	vA := baseClusterVersion(&us, &irtranslator.BaseCluster{Cluster: clusterA, EndpointInputs: epsIn, SupportsInlineCLA: true})
	vB := baseClusterVersion(&us, &irtranslator.BaseCluster{Cluster: clusterB, EndpointInputs: epsIn, SupportsInlineCLA: true})
	require.NotEqual(t, vA, vB, "different cluster proto must produce a different version")
}

// TestBaseClusterVersion_InlineCLAReflectsPolicyChanges is a regression test for
// the stale-AttachedPolicies bug: PerClientProcessEndpoints hooks (e.g. the
// backendconfigpolicy zone-aware plugin) read AttachedPolicies from the cached
// BaseCluster.EndpointInputs, and KRT keeps the OLD stored object when Equals
// returns true. If a policy-only change (cluster proto and endpoints unchanged)
// did not move the version, ApplyPerClient would build per-client CLAs from the
// stale attachment forever. Mirrors the EDS path's backendEndpointVersionHash
// folding in newFinalBackendEndpoints.
func TestBaseClusterVersion_InlineCLAReflectsPolicyChanges(t *testing.T) {
	cluster := staticInlineCLACluster()

	backendWithPolicyGen := func(gen int64) ir.BackendObjectIR {
		us := ir.NewBackendObjectIR(ir.ObjectSource{Namespace: "ns", Name: "svc"}, 0, "", "")
		us.AttachedPolicies = ir.AttachedPolicies{
			Policies: map[schema.GroupKind][]ir.PolicyAtt{
				{Group: "gateway.kgateway.dev", Kind: "BackendConfigPolicy"}: {
					{
						PolicyRef:  &ir.AttachedPolicyRef{Group: "gateway.kgateway.dev", Kind: "BackendConfigPolicy", Name: "pol", Namespace: "ns"},
						Generation: gen,
					},
				},
			},
		}
		return us
	}

	baseFor := func(us ir.BackendObjectIR) *irtranslator.BaseCluster {
		epsIn := &endpoints.EndpointsInputs{EndpointsForBackend: *ir.NewEndpointsForBackend(us)}
		epsIn.EndpointsForBackend.AttachedPolicies = us.AttachedPolicies
		return &irtranslator.BaseCluster{Cluster: cluster, EndpointInputs: epsIn, SupportsInlineCLA: true}
	}

	usGen1 := backendWithPolicyGen(1)
	usGen2 := backendWithPolicyGen(2)

	vGen1 := baseClusterVersion(&usGen1, baseFor(usGen1))
	vGen1Again := baseClusterVersion(&usGen1, baseFor(usGen1))
	vGen2 := baseClusterVersion(&usGen2, baseFor(usGen2))

	require.Equal(t, vGen1, vGen1Again, "version must be stable for identical inputs")
	require.NotEqual(t, vGen1, vGen2,
		"a policy generation bump must change the inline-CLA base version even when the cluster proto and endpoints are unchanged")

	// EDS bases must stay stable across policy changes: their endpoints (and
	// policy-driven endpoint versioning) flow through the separate EDS pipeline.
	edsCluster := &envoyclusterv3.Cluster{
		Name:                 "eds-cluster",
		ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_EDS},
	}
	edsGen1 := baseClusterVersion(&usGen1, &irtranslator.BaseCluster{Cluster: edsCluster})
	edsGen2 := baseClusterVersion(&usGen2, &irtranslator.BaseCluster{Cluster: edsCluster})
	require.Equal(t, edsGen1, edsGen2, "EDS base version must not depend on attached-policy changes")
}

func staticInlineCLACluster() *envoyclusterv3.Cluster {
	return &envoyclusterv3.Cluster{
		Name: "inline-cla-cluster",
		ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{
			Type: envoyclusterv3.Cluster_STRICT_DNS,
		},
	}
}

func lbEndpointPipe(path string) *envoyendpointv3.LbEndpoint {
	return &envoyendpointv3.LbEndpoint{
		HostIdentifier: &envoyendpointv3.LbEndpoint_Endpoint{
			Endpoint: &envoyendpointv3.Endpoint{
				Address: &envoycorev3.Address{
					Address: &envoycorev3.Address_Pipe{Pipe: &envoycorev3.Pipe{Path: path}},
				},
			},
		},
	}
}

package serviceentry

import (
	"context"

	"go.uber.org/zap"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils/krtutil"

	networking "istio.io/api/networking/v1alpha3"
	networkingclient "istio.io/client-go/pkg/apis/networking/v1"
	"istio.io/istio/pkg/config/schema/gvk"
	"istio.io/istio/pkg/kube/krt"

	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"

	"k8s.io/utils/ptr"
)

func (s *serviceEntryPlugin) initServiceEntryBackend(ctx context.Context, in ir.BackendObjectIR, out *clusterv3.Cluster) *ir.EndpointsForBackend {
	se, ok := in.Obj.(*networkingclient.ServiceEntry)
	if !ok {
		return nil
	}

	// Only ServiceEntry that uses STATIC resolution with a workloadSelector
	// results in an EDS cluster.
	// All other cases should result in envoy STATIC or DNS clusters.
	switch se.Spec.GetResolution() {
	case networking.ServiceEntry_STATIC:
		if !isEDSServiceEntry(se) {
			out.ClusterDiscoveryType = &clusterv3.Cluster_Type{
				Type: clusterv3.Cluster_STATIC,
			}
		}
	case networking.ServiceEntry_DNS:
		out.ClusterDiscoveryType = &clusterv3.Cluster_Type{
			Type: clusterv3.Cluster_STRICT_DNS,
		}
	case networking.ServiceEntry_DNS_ROUND_ROBIN:
		out.ClusterDiscoveryType = &clusterv3.Cluster_Type{
			Type: clusterv3.Cluster_LOGICAL_DNS,
		}
	}

	var staticEps *ir.EndpointsForBackend
	if isEDSServiceEntry(se) {
		out.ClusterDiscoveryType = &clusterv3.Cluster_Type{
			Type: clusterv3.Cluster_EDS,
		}
		out.EdsClusterConfig = &clusterv3.Cluster_EdsClusterConfig{
			EdsConfig: &corev3.ConfigSource{
				ResourceApiVersion: corev3.ApiVersion_V3,
				ConfigSourceSpecifier: &corev3.ConfigSource_Ads{
					Ads: &corev3.AggregatedConfigSource{},
				},
			},
		}
	} else {
		// STATIC with inline endpoints, or either kind of DNS require an inline load assignment

		// compute endpoints from ServiceEntry
		staticEps = s.buildInlineEndpoints(ctx, in, se)
	}
	return staticEps
}

// backendsCollections produces a one-to-many collection from ServiceEntry into BackendObjectIR.
// For each ServiceEntry, we create hosts*ports BackendObjectIRs.
func backendsCollections(
	logger *zap.SugaredLogger,
	ServiceEntries krt.Collection[*networkingclient.ServiceEntry],
	krtOpts krtutil.KrtOptions,
) krt.Collection[ir.BackendObjectIR] {
	return krt.NewManyCollection(ServiceEntries, func(ctx krt.HandlerContext, se *networkingclient.ServiceEntry) []ir.BackendObjectIR {
		// passthrough not supported here
		if se.Spec.GetResolution() == networking.ServiceEntry_NONE {
			logger.Debugw("skipping ServiceEntry with resolution: NONE", "name", se.GetName(), "namespace", se.GetNamespace())
			return nil
		}

		logger.Debugw("converting ServiceEntry to Upstream", "name", se.GetName(), "namespace", se.GetNamespace())
		var out []ir.BackendObjectIR

		for _, hostname := range se.Spec.GetHosts() {
			for _, svcPort := range se.Spec.GetPorts() {
				out = append(out, BuildServiceEntryBackendObjectIR(
					se,
					hostname,
					int32(svcPort.GetNumber()),
					svcPort.GetProtocol(),
				))
			}
		}

		return out
	}, krtOpts.ToOptions("ServiceEntryBackends")...)
}

func BuildServiceEntryBackendObjectIR(
	se *networkingclient.ServiceEntry,
	hostname string,
	svcPort int32,
	svcProtocol string,
) ir.BackendObjectIR {
	return ir.BackendObjectIR{
		ObjectSource: ir.ObjectSource{
			Group:     gvk.ServiceEntry.Group,
			Kind:      gvk.ServiceEntry.Kind,
			Namespace: se.GetNamespace(),
			Name:      se.GetName(),
		},
		Port:              svcPort,
		AppProtocol:       ir.ParseAppProtocol(ptr.To(svcProtocol)),
		GvPrefix:          BackendClusterPrefix,
		CanonicalHostname: hostname,
		Obj:               se,
		// TODO ObjIr:             nil,
		AttachedPolicies: ir.AttachedPolicies{},

		// TODO this is a hack so we don't have key conflicts in krt since we
		// build per-hostname backends. This means the generic `getBackend`
		// in policy.go will never work, and we resolve the direct
		// ServiceEntry backendRefs in our backendref plugin.
		ExtraKey: hostname,
	}
}

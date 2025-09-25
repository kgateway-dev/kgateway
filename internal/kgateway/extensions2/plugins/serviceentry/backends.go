package serviceentry

import (
	"context"
	"log/slog"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"

	"istio.io/api/annotation"
	networking "istio.io/api/networking/v1alpha3"
	networkingclient "istio.io/client-go/pkg/apis/networking/v1"
	"istio.io/istio/pkg/config/schema/gvk"
	"istio.io/istio/pkg/kube/krt"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"

	"k8s.io/utils/ptr"
)

func (s *serviceEntryPlugin) initServiceEntryBackend(ctx context.Context, in ir.BackendObjectIR, out *envoyclusterv3.Cluster) *ir.EndpointsForBackend {
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
			out.ClusterDiscoveryType = &envoyclusterv3.Cluster_Type{
				Type: envoyclusterv3.Cluster_STATIC,
			}
		}
	case networking.ServiceEntry_DNS:
		out.ClusterDiscoveryType = &envoyclusterv3.Cluster_Type{
			Type: envoyclusterv3.Cluster_STRICT_DNS,
		}
	case networking.ServiceEntry_DNS_ROUND_ROBIN:
		out.ClusterDiscoveryType = &envoyclusterv3.Cluster_Type{
			Type: envoyclusterv3.Cluster_LOGICAL_DNS,
		}
	}

	var staticEps *ir.EndpointsForBackend
	if isEDSServiceEntry(se) {
		out.ClusterDiscoveryType = &envoyclusterv3.Cluster_Type{
			Type: envoyclusterv3.Cluster_EDS,
		}
		out.EdsClusterConfig = &envoyclusterv3.Cluster_EdsClusterConfig{
			EdsConfig: &envoycorev3.ConfigSource{
				ResourceApiVersion: envoycorev3.ApiVersion_V3,
				ConfigSourceSpecifier: &envoycorev3.ConfigSource_Ads{
					Ads: &envoycorev3.AggregatedConfigSource{},
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
	logger *slog.Logger,
	ServiceEntries krt.Collection[*networkingclient.ServiceEntry],
	krtOpts krtutil.KrtOptions,
	aliaser Aliaser,
) krt.Collection[ir.BackendObjectIR] {
	return krt.NewManyCollection(ServiceEntries, func(ctx krt.HandlerContext, se *networkingclient.ServiceEntry) []ir.BackendObjectIR {
		// passthrough not supported here
		if se.Spec.GetResolution() == networking.ServiceEntry_NONE {
			logger.Debug("skipping ServiceEntry with resolution: NONE", "name", se.GetName(), "namespace", se.GetNamespace())
			return nil
		}

		logger.Debug("converting ServiceEntry to Upstream", "name", se.GetName(), "namespace", se.GetNamespace())
		var out []ir.BackendObjectIR

		for _, hostname := range se.Spec.GetHosts() {
			for _, svcPort := range se.Spec.GetPorts() {
				out = append(out, BuildServiceEntryBackendObjectIR(
					se,
					hostname,
					int32(svcPort.GetNumber()), //nolint:gosec // G115: ServiceEntry port numbers are always valid port range (1-65535)
					svcPort.GetProtocol(),
					aliaser,
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
	aliaser Aliaser,
) ir.BackendObjectIR {
	objSrc := ir.ObjectSource{
		Group:     gvk.ServiceEntry.Group,
		Kind:      gvk.ServiceEntry.Kind,
		Namespace: se.GetNamespace(),
		Name:      se.GetName(),
	}
	// TODO hostname as extraKey here is a hack so we don't have key conflicts in krt since we
	// build per-hostname backends; since getBackend tries to use krt-key by
	// default, it will never find ServiceEntry, so we "alias" ServiceEntry to ServiceEntry
	// to get the ref-index-based logic instead of the krt-key based lookup.
	backend := ir.NewBackendObjectIR(objSrc, svcPort, hostname)
	backend.AppProtocol = ir.ParseAppProtocol(ptr.To(svcProtocol))
	backend.GvPrefix = BackendClusterPrefix
	backend.CanonicalHostname = hostname
	backend.Obj = se

	// include ourselves as alias to fix issues with one-to-many se-to-backend
	backend.Aliases = []ir.ObjectSource{objSrc}
	if aliaser != nil {
		backend.Aliases = append(backend.Aliases, aliaser(se)...)
	}

	// We support specifying the Istio traffic distribution in the annotations of the ServicEntry.
	if val, ok := se.Annotations[annotation.NetworkingTrafficDistribution.Name]; ok {
		backend.TrafficDistribution = wellknown.ParseTrafficDistribution(val)
	}

	// Parse common annotations
	ir.ParseObjectAnnotations(&backend, se)

	backend.AttachedPolicies = ir.AttachedPolicies{}
	return backend
}

package serviceentry

import (
	"context"
	"fmt"
	"net"

	"google.golang.org/protobuf/types/known/wrapperspb"

	networking "istio.io/api/networking/v1alpha3"
	networkingclient "istio.io/client-go/pkg/apis/networking/v1"
	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

// buildInlineEndpoints creates a static EndpointsForBackend for non-EDS a ServiceEntry using
// the inline endpoints (or hosts field for DNS resolution without endpoints).
func (s *serviceEntryPlugin) buildInlineEndpoints(ctx context.Context, be ir.BackendObjectIR, se *networkingclient.ServiceEntry) *ir.EndpointsForBackend {
	var inlineWorkloads []selectedWorkload
	for i, e := range se.Spec.GetEndpoints() {
		converted := selectedWorkloadFromEntry(
			fmt.Sprintf("%s-endpoints-%d", se.Name, i), // synthetic name
			se.Namespace, // inherit se namespace
			nil,          // no metadata labels to inherit
			e,
			nil, // not in krt, don't need selectedBy
		)
		inlineWorkloads = append(inlineWorkloads, converted)
	}

	// DNS resolution can use the hosts field if endpoints are empty
	// https://istio.io/latest/docs/reference/config/networking/service-entry/#ServiceEntry-Resolution
	if isDNSServiceEntry(se) && len(inlineWorkloads) == 0 {
		for i, hostname := range se.Spec.GetHosts() {
			converted := selectedWorkloadFromEntry(
				fmt.Sprintf("%s-hosts-%d", se.Name, i), // synthetic name
				se.Namespace,                           // inherit se namespace
				nil,                                    // no metadata labels to inherit
				&networking.WorkloadEntry{Address: hostname},
				nil, // not in krt, don't need selectedBy
			)
			inlineWorkloads = append(inlineWorkloads, converted)
		}
	}

	endpointsForBackend := endpointsFromWorkloads(se, be, inlineWorkloads)
	if endpointsForBackend == nil {
		// this is pretty much impossible, but `ir.NewEndpointsForBackend(be)`
		// returns a pointer, so this is for safety
		s.logger.Error("unresolved endpoints for ServiceEntry", "resource_ref", krt.NewNamed(se).ResourceName())
		return nil
	}

	// Set the traffic distribution for the endpoints based on the one specified in the backend
	endpointsForBackend.TrafficDistribution = be.TrafficDistribution

	return endpointsForBackend
}

func endpointsCollection(
	Backends krt.Collection[ir.BackendObjectIR],
	SelectedWorkloads krt.Collection[selectedWorkload],
	selectedWorkloadsIndex krt.Index[string, selectedWorkload],
	krtOpts krtutil.KrtOptions,
) krt.Collection[ir.EndpointsForBackend] {
	return krt.NewCollection(
		Backends,
		func(ctx krt.HandlerContext, be ir.BackendObjectIR) *ir.EndpointsForBackend {
			se, ok := be.Obj.(*networkingclient.ServiceEntry)
			if !ok {
				return nil
			}
			if !isEDSServiceEntry(se) {
				return nil
			}
			workloads := krt.Fetch(ctx, SelectedWorkloads, krt.FilterIndex(selectedWorkloadsIndex, serviceEntryKey(se)))

			return endpointsFromWorkloads(se, be, workloads)
		},
		krtOpts.ToOptions("ServiceEntryEndpoints")...,
	)
}

// endpointsFromWorkloads converts a group of SelectedWorkloads into
// ir.EndpointsForBackend in the context of a single ServiceEntry backend.
// We use selectedWorkload as the input because:
// 1. It lets us re-use code for both Pod and WorkloadEntry
// 2. We can easily construct synthetic selectedWorkload for inline endpoints on a ServiceEntry
func endpointsFromWorkloads(
	se *networkingclient.ServiceEntry,
	be ir.BackendObjectIR,
	workloads []selectedWorkload,
) *ir.EndpointsForBackend {
	if len(workloads) == 0 {
		return ir.NewEndpointsForBackend(be)
	}

	// this should never miss, we only call buildInlineCLA using BackendObjectIR
	// generated from the ServiceEntry iteself
	var servicePort *networking.ServicePort
	for _, sp := range se.Spec.GetPorts() {
		if int32(sp.GetNumber()) == be.Port { //nolint:gosec // G115: ServiceEntry port numbers are always valid port range (1-65535)
			servicePort = sp
			break
		}
	}

	seTargetPort := be.Port
	if sePortTargetPort := servicePort.GetTargetPort(); sePortTargetPort > 0 {
		seTargetPort = int32(sePortTargetPort) //nolint:gosec // G115: ServiceEntry target port numbers are always valid port range (1-65535)
	}

	eps := ir.NewEndpointsForBackend(be)
	for _, workload := range workloads {
		address := workload.Address()

		// for static, it must be an IP
		// for DNS it can be IP or hostname
		if se.Spec.GetResolution() == networking.ServiceEntry_STATIC {
			// allow empty address workloads that have a network specificed
			// so that endpoint plugins can process them.
			// if they reach the ClusterLoadAssignment generation code with no
			// address (post-plugins) then we will filter them there
			if net.ParseIP(address) == nil && workload.network == "" {
				continue
			}
		}

		// respect target ports on the WorkloadEntry
		epPort := workload.mapPort(servicePort.GetName(), seTargetPort)

		// only MESH_INTERNAL can use auto mTLS
		allowAutoMTLS := se.Spec.GetLocation() == networking.ServiceEntry_MESH_INTERNAL

		ep := ir.EndpointWithMd{
			LbEndpoint: krtcollections.CreateLBEndpoint(address, uint32(epPort), workload.AugmentedLabels, allowAutoMTLS), //nolint:gosec // G115: epPort is derived from valid port numbers
			EndpointMd: ir.EndpointMetadata{
				Labels: workload.AugmentedLabels,
			},
		}
		if workload.weight > 0 {
			ep.LbEndpoint.LoadBalancingWeight = &wrapperspb.UInt32Value{Value: workload.weight}
		}
		eps.Add(workload.Locality, ep)
	}
	return eps
}

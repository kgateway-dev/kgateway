package translator

import (
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	"github.com/solo-io/gloo/internal/control-plane/snapshot"

	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/endpointdiscovery"
)

// trim the snapshot config to contain only what the listener needs to know
func trimSnapshot(role *v1.Role, listener *v1.Listener, inputs *snapshot.Cache, configErrs configErrors) *snapshot.Cache {
	virtualServices, err := virtualServicesForListener(listener, inputs.Cfg.VirtualServices)
	if err != nil {
		configErrs.addError(role, err)
	}
	upstreams := destinationUpstreams(inputs.Cfg.Upstreams, virtualServices)

	return &snapshot.Cache{
		Cfg: &v1.Config{
			VirtualServices: virtualServices,
			Upstreams:       upstreams,
			Roles:           []*v1.Role{role},
		},
		Endpoints: destinationEndpoints(upstreams, inputs.Endpoints),
		Secrets:   inputs.Secrets,
		Files:     inputs.Files,
	}
}

// filter virtual services for the listener
func virtualServicesForListener(listener *v1.Listener, virtualServices []*v1.VirtualService) ([]*v1.VirtualService, error) {
	var listenerErrs error
	var listenerVirtualServices []*v1.VirtualService
	for _, name := range listener.VirtualServices {
		var vsFound bool
		for _, vs := range virtualServices {
			if vs.Name == name {
				listenerVirtualServices = append(listenerVirtualServices, vs)
				vsFound = true
				break
			}
		}
		if !vsFound {
			listenerErrs = multierror.Append(listenerErrs, errors.Errorf("virtual service %v not found for listener %v", name, listener.Name))
		}
	}
	return listenerVirtualServices, listenerErrs
}

// gets the subset of upstreams which are destinations for at least one route in at least one
// virtual service
func destinationUpstreams(allUpstreams []*v1.Upstream, virtualServices []*v1.VirtualService) []*v1.Upstream {
	destinationUpstreamNames := make(map[string]bool)
	for _, vs := range virtualServices {
		for _, route := range vs.Routes {
			dests := getAllDestinations(route)
			for _, dest := range dests {
				var upstreamName string
				switch typedDest := dest.DestinationType.(type) {
				case *v1.Destination_Upstream:
					upstreamName = typedDest.Upstream.Name
				case *v1.Destination_Function:
					upstreamName = typedDest.Function.UpstreamName
				default:
					panic("unknown destination type")
				}
				destinationUpstreamNames[upstreamName] = true
			}
		}
	}
	var destinationUpstreams []*v1.Upstream
	for _, us := range allUpstreams {
		if _, ok := destinationUpstreamNames[us.Name]; ok {
			destinationUpstreams = append(destinationUpstreams, us)
		}
	}
	return destinationUpstreams
}

func getAllDestinations(route *v1.Route) []*v1.Destination {
	var dests []*v1.Destination
	if route.SingleDestination != nil {
		dests = append(dests, route.SingleDestination)
	}
	for _, dest := range route.MultipleDestinations {
		dests = append(dests, dest.Destination)
	}
	return dests
}

func destinationEndpoints(upstreams []*v1.Upstream, allEndpoints endpointdiscovery.EndpointGroups) endpointdiscovery.EndpointGroups {
	destinationEndpoints := make(endpointdiscovery.EndpointGroups)
	for _, us := range upstreams {
		eps, ok := allEndpoints[us.Name]
		if !ok {
			continue
		}
		destinationEndpoints[us.Name] = eps
	}
	return destinationEndpoints
}

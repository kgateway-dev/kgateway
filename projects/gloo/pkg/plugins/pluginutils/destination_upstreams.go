package pluginutils

import (
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	usconversions "github.com/solo-io/gloo/projects/gloo/pkg/upstreams"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

func DestinationUpstreams(snap *v1.ApiSnapshot, in *v1.RouteAction) ([]core.ResourceRef, error) {
	switch dest := in.Destination.(type) {
	case *v1.RouteAction_Single:
		upstream, err := usconversions.DestinationToUpstreamRef(dest.Single)
		if err != nil {
			return nil, err
		}
		return []core.ResourceRef{*upstream}, nil

	case *v1.RouteAction_Multi:
		return destinationsToRefs(dest.Multi.Destinations)

	case *v1.RouteAction_UpstreamGroup:

		upstreamGroup, err := snap.Upstreamgroups.Find(dest.UpstreamGroup.Namespace, dest.UpstreamGroup.Name)
		if err != nil {
			return nil, err
		}
		return destinationsToRefs(upstreamGroup.Destinations)
	}
	panic("invalid route")
}

func destinationsToRefs(destinations []*v1.WeightedDestination) ([]core.ResourceRef, error) {
	var upstreams []core.ResourceRef
	for _, dest := range destinations {
		upstream, err := usconversions.DestinationToUpstreamRef(dest.Destination)
		if err != nil {
			return nil, err
		}
		upstreams = append(upstreams, *upstream)
	}
	return upstreams, nil
}

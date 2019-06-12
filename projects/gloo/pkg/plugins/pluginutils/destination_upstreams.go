package pluginutils

import (
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

func DestinationUpstreams(snap *v1.ApiSnapshot, in *v1.RouteAction) ([]core.ResourceRef, error) {
	switch dest := in.Destination.(type) {
	case *v1.RouteAction_Single:
		if upstream := dest.Single.GetUpstream(); upstream != nil {
			return []core.ResourceRef{*upstream}, nil
		}
	case *v1.RouteAction_Multi:

		return destinationsToRefs(dest.Multi.Destinations), nil
	case *v1.RouteAction_UpstreamGroup:

		upstreamGroup, err := snap.Upstreamgroups.Find(dest.UpstreamGroup.Namespace, dest.UpstreamGroup.Name)
		if err != nil {
			return nil, err
		}
		return destinationsToRefs(upstreamGroup.Destinations), nil
	}
	panic("invalid route")
}

func destinationsToRefs(dests []*v1.WeightedDestination) []core.ResourceRef {
	var upstreams []core.ResourceRef
	for _, dest := range dests {
		if upstream := dest.Destination.GetUpstream(); upstream != nil {
			upstreams = append(upstreams, *upstream)
		}
	}
	return upstreams
}

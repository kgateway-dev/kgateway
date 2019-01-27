package pluginutils

import (
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

func DestinationUpstreams(in *v1.RouteAction) []core.ResourceRef {
	switch dest := in.Destination.(type) {
	case *v1.RouteAction_Single:
		return []core.ResourceRef{dest.Single.Upstream}
	case *v1.RouteAction_Multi:
		var upstreams []core.ResourceRef
		for _, dest := range dest.Multi.Destinations {
			upstreams = append(upstreams, dest.Destination.Upstream)
		}
		return upstreams
	}
	panic("invalid route")
}

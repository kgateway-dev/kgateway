package pluginutils

import (
	"errors"
	"fmt"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"

	v1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	usconversions "github.com/solo-io/gloo/projects/gloo/pkg/upstreams"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

func DestinationUpstreams(snap *v1snap.ApiSnapshot, in *v1.RouteAction) ([]*core.ResourceRef, error) {
	switch dest := in.GetDestination().(type) {
	case *v1.RouteAction_Single:
		upstream, err := usconversions.DestinationToUpstreamRef(dest.Single)
		if err != nil {
			return nil, err
		}
		return []*core.ResourceRef{upstream}, nil

	case *v1.RouteAction_Multi:
		return destinationsToRefs(dest.Multi.GetDestinations())

	case *v1.RouteAction_UpstreamGroup:

		upstreamGroup, err := snap.UpstreamGroups.Find(dest.UpstreamGroup.GetNamespace(), dest.UpstreamGroup.GetName())
		if err != nil {
			return nil, NewUpstreamGroupNotFoundErr(dest.UpstreamGroup)
		}
		return destinationsToRefs(upstreamGroup.GetDestinations())
	}
	return nil, errors.New("invalid route")
}

func destinationsToRefs(destinations []*v1.WeightedDestination) ([]*core.ResourceRef, error) {
	var upstreams []*core.ResourceRef
	for _, dest := range destinations {
		upstream, err := usconversions.DestinationToUpstreamRef(dest.GetDestination())
		if err != nil {
			return nil, err
		}
		upstreams = append(upstreams, upstream)
	}
	return upstreams, nil
}

type DestinationNotFoundError struct {
	Ref          *core.ResourceRef
	ResourceType resources.Resource
}

func NewUpstreamNotFoundErr(ref *core.ResourceRef) *plugins.BaseConfigurationError {
	message := fmt.Sprintf("%T { %s.%s } not found", &v1.Upstream{}, ref.GetNamespace(), ref.GetName())
	return plugins.NewWarningConfigurationError(message)
}

func NewUpstreamGroupNotFoundErr(ref *core.ResourceRef) *plugins.BaseConfigurationError {
	message := fmt.Sprintf("%T { %s.%s } not found", &v1.UpstreamGroup{}, ref.GetNamespace(), ref.GetName())
	return plugins.NewWarningConfigurationError(message)
}

func (e *DestinationNotFoundError) Error() string {
	return fmt.Sprintf("%T { %s.%s } not found", e.ResourceType, e.Ref.GetNamespace(), e.Ref.GetName())
}

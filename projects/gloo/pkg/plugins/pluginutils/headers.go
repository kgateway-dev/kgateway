package pluginutils

import (
	"context"
	"reflect"

	v1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"

	errors "github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"

	"github.com/solo-io/go-utils/contextutils"
)

type HeadersToAddFunc func(spec *v1.Destination) ([]*envoy_config_core_v3.HeaderValueOption, error)

// Allows you add extra headers for specific destination.
// The provided callback will be called for all the destinations on the route.
// Any headers returned will be added to requests going to that destination
func MarkHeaders(ctx context.Context, snap *v1snap.ApiSnapshot, in *v1.Route, out *envoy_config_route_v3.Route, headers HeadersToAddFunc) error {
	inAction, outAction, err := getRouteActions(in, out)
	if err != nil {
		return err
	}
	switch dest := inAction.GetDestination().(type) {
	case *v1.RouteAction_UpstreamGroup:
		upstreamGroup, err := snap.UpstreamGroups.Find(dest.UpstreamGroup.GetNamespace(), dest.UpstreamGroup.GetName())
		if err != nil {
			return NewUpstreamGroupNotFoundErr(*dest.UpstreamGroup)
		}
		return configureHeadersMultiDest(upstreamGroup.GetDestinations(), outAction, headers)
	case *v1.RouteAction_Multi:
		return configureHeadersMultiDest(dest.Multi.GetDestinations(), outAction, headers)
	case *v1.RouteAction_Single:
		return configureHeadersSingleDest(dest.Single, &out.RequestHeadersToAdd, headers)
	// Since destination is not known until runtime, headers cannot be added using this function for a ClusterHeader destination
	case *v1.RouteAction_ClusterHeader:
		return nil
	}

	err = errors.Errorf("unexpected destination type %v", reflect.TypeOf(inAction.GetDestination()).Name())
	logger := contextutils.LoggerFrom(ctx)
	logger.DPanic("error: %v", err)
	return err
}

func configureHeadersMultiDest(
	in []*v1.WeightedDestination,
	outAction *envoy_config_route_v3.RouteAction,
	headers HeadersToAddFunc,
) error {

	multiClusterSpecifier, ok := outAction.GetClusterSpecifier().(*envoy_config_route_v3.RouteAction_WeightedClusters)
	if !ok {
		return errors.Errorf("input destination Multi but output destination was not")
	}
	out := multiClusterSpecifier.WeightedClusters

	if len(in) != len(out.GetClusters()) {
		return errors.Errorf("number of input destinations (%d) did not match number of destination weighted clusters (%d)", len(in), len(out.GetClusters()))
	}
	for i := range in {
		err := configureHeadersSingleDest(in[i].GetDestination(), &out.GetClusters()[i].RequestHeadersToAdd, headers)
		if err != nil {
			return err
		}
	}

	return nil
}

func configureHeadersSingleDest(
	in *v1.Destination,
	out *[]*envoy_config_core_v3.HeaderValueOption,
	headers HeadersToAddFunc,
) error {
	config, err := headers(in)
	if err != nil {
		return err
	}
	// the plugin decided not to configure this route
	if config == nil {
		return nil
	}
	*out = append(*out, config...)
	return nil
}

package pluginutils

import (
	"context"
	"reflect"

	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"

	"github.com/pkg/errors"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/contextutils"
)

type HeadersToAddFunc func(spec *v1.Destination) ([]*envoycore.HeaderValueOption, error)

// Allows you add extra headers for specific destination.
// The provided callback will be called for all the destinations on the route.
// Any headers returned will be added to requests going to that destination
func MarkHeaders(ctx context.Context, snap *v1.ApiSnapshot, in *v1.Route, out *envoyroute.Route, headers HeadersToAddFunc) error {
	inAction, outAction, err := getRouteActions(in, out)
	if err != nil {
		return err
	}
	switch dest := inAction.Destination.(type) {
	case *v1.RouteAction_UpstreamGroup:
		upstreamGroup, err := snap.UpstreamGroups.Find(dest.UpstreamGroup.Namespace, dest.UpstreamGroup.Name)
		if err != nil {
			return NewUpstreamGroupNotFoundErr(*dest.UpstreamGroup)
		}
		return configureHeadersMultiDest(upstreamGroup.Destinations, outAction, headers)
	case *v1.RouteAction_Multi:
		return configureHeadersMultiDest(dest.Multi.Destinations, outAction, headers)
	case *v1.RouteAction_Single:
		return configureHeadersSingleDest(dest.Single, &out.RequestHeadersToAdd, headers)
	}

	err = errors.Errorf("unexpected destination type %v", reflect.TypeOf(inAction.Destination).Name())
	logger := contextutils.LoggerFrom(ctx)
	logger.DPanic("error: %v", err)
	return err
}

func configureHeadersMultiDest(in []*v1.WeightedDestination, outAction *envoyroute.RouteAction, headers HeadersToAddFunc) error {

	multiClusterSpecifier, ok := outAction.ClusterSpecifier.(*envoyroute.RouteAction_WeightedClusters)
	if !ok {
		return errors.Errorf("input destination Multi but output destination was not")
	}
	out := multiClusterSpecifier.WeightedClusters

	if len(in) != len(out.Clusters) {
		return errors.Errorf("number of input destinations did not match number of destination weighted clusters")
	}
	for i := range in {
		err := configureHeadersSingleDest(in[i].Destination, &out.Clusters[i].RequestHeadersToAdd, headers)
		if err != nil {
			return err
		}
	}

	return nil
}

func configureHeadersSingleDest(in *v1.Destination, out *[]*envoycore.HeaderValueOption, headers HeadersToAddFunc) error {
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

package pluginutils

import (
	"context"
	"reflect"

	"github.com/golang/protobuf/ptypes/any"

	envoyroute "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/gogo/protobuf/proto"
	errors "github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/contextutils"
)

func SetRoutePerFilterConfig(out *envoyroute.Route, filterName string, protoext proto.Message) error {
	if out.GetTypedPerFilterConfig() == nil {
		out.TypedPerFilterConfig = make(map[string]*any.Any)
	}
	return setConfig(out.TypedPerFilterConfig, filterName, protoext)
}
func SetVhostPerFilterConfig(out *envoyroute.VirtualHost, filterName string, protoext proto.Message) error {
	if out.GetTypedPerFilterConfig() == nil {
		out.TypedPerFilterConfig = make(map[string]*any.Any)
	}
	return setConfig(out.TypedPerFilterConfig, filterName, protoext)
}
func SetWeightedClusterPerFilterConfig(out *envoyroute.WeightedCluster_ClusterWeight, filterName string, protoext proto.Message) error {
	if out.GetTypedPerFilterConfig() == nil {
		out.TypedPerFilterConfig = make(map[string]*any.Any)
	}
	return setConfig(out.TypedPerFilterConfig, filterName, protoext)
}

// Return Per-Filter config for destinations, we put them on the Route (single dest) or WeightedCluster (multi dest)
type TypedPerFilterConfigFunc func(spec *v1.Destination) (proto.Message, error)

// call this from
func MarkPerFilterConfig(ctx context.Context, snap *v1.ApiSnapshot, in *v1.Route, out *envoyroute.Route, filterName string, typedPerFilterConfig TypedPerFilterConfigFunc) error {
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

		return configureMultiDest(upstreamGroup.Destinations, outAction, filterName, typedPerFilterConfig)
	case *v1.RouteAction_Multi:

		return configureMultiDest(dest.Multi.Destinations, outAction, filterName, typedPerFilterConfig)
	case *v1.RouteAction_Single:
		if out.GetTypedPerFilterConfig() == nil {
			out.TypedPerFilterConfig = make(map[string]*any.Any)
		}
		return configureSingleDest(dest.Single, out.TypedPerFilterConfig, filterName, typedPerFilterConfig)
	}

	err = errors.Errorf("unexpected destination type %v", reflect.TypeOf(inAction.Destination).Name())
	logger := contextutils.LoggerFrom(ctx)
	logger.DPanic("error: %v", err)
	return err
}

func configureMultiDest(in []*v1.WeightedDestination, outAction *envoyroute.RouteAction, filterName string, typedPerFilterConfig TypedPerFilterConfigFunc) error {

	multiClusterSpecifier, ok := outAction.ClusterSpecifier.(*envoyroute.RouteAction_WeightedClusters)
	if !ok {
		return errors.Errorf("input destination Multi but output destination was not")
	}
	out := multiClusterSpecifier.WeightedClusters

	if len(in) != len(out.Clusters) {
		return errors.Errorf("number of input destinations did not match number of destination weighted clusters")
	}
	for i := range in {
		if out.Clusters[i].GetTypedPerFilterConfig() == nil {
			out.Clusters[i].TypedPerFilterConfig = make(map[string]*any.Any)
		}
		err := configureSingleDest(in[i].Destination, out.Clusters[i].TypedPerFilterConfig, filterName, typedPerFilterConfig)
		if err != nil {
			return err
		}
	}

	return nil
}

func configureSingleDest(in *v1.Destination, out map[string]*any.Any, filterName string, typedPerFilterConfig TypedPerFilterConfigFunc) error {
	config, err := typedPerFilterConfig(in)
	if err != nil {
		return err
	}
	return setConfig(out, filterName, config)
}

func setConfig(out map[string]*any.Any, filterName string, config proto.Message) error {
	if config == nil {
		return nil
	}
	configAny, err := MessageToAny(config)
	if err != nil {
		return err
	}
	out[filterName] = configAny
	return nil
}

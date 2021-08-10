package pluginutils

import (
	"context"
	"reflect"

	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/any"
	errors "github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/go-utils/contextutils"
)

func SetRoutePerFilterConfig(out *envoy_config_route_v3.Route, filterName string, protoext proto.Message) error {
	if out.GetTypedPerFilterConfig() == nil {
		out.TypedPerFilterConfig = make(map[string]*any.Any)
	}
	return setConfig(out.GetTypedPerFilterConfig(), filterName, protoext)
}
func SetVhostPerFilterConfig(out *envoy_config_route_v3.VirtualHost, filterName string, protoext proto.Message) error {
	if out.GetTypedPerFilterConfig() == nil {
		out.TypedPerFilterConfig = make(map[string]*any.Any)
	}
	return setConfig(out.GetTypedPerFilterConfig(), filterName, protoext)
}
func SetWeightedClusterPerFilterConfig(
	out *envoy_config_route_v3.WeightedCluster_ClusterWeight,
	filterName string,
	protoext proto.Message,
) error {
	if out.GetTypedPerFilterConfig() == nil {
		out.TypedPerFilterConfig = make(map[string]*any.Any)
	}
	return setConfig(out.GetTypedPerFilterConfig(), filterName, protoext)
}

// Return Per-Filter config for destinations, we put them on the Route (single dest) or WeightedCluster (multi dest)
type PerFilterConfigFunc func(spec *v1.Destination) (proto.Message, error)

// call this from
func MarkPerFilterConfig(
	ctx context.Context,
	snap *v1.ApiSnapshot,
	in *v1.Route,
	out *envoy_config_route_v3.Route,
	filterName string,
	perFilterConfig PerFilterConfigFunc,
) error {
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

		return configureMultiDest(upstreamGroup.GetDestinations(), outAction, filterName, perFilterConfig)
	case *v1.RouteAction_Multi:

		return configureMultiDest(dest.Multi.GetDestinations(), outAction, filterName, perFilterConfig)
	case *v1.RouteAction_Single:
		if out.GetTypedPerFilterConfig() == nil {
			out.TypedPerFilterConfig = make(map[string]*any.Any)
		}
		return configureSingleDest(dest.Single, out.GetTypedPerFilterConfig(), filterName, perFilterConfig)
	// intentionally ignored because destination is not specified at runtime, so perFilterConfig is useless
	case *v1.RouteAction_ClusterHeader:
		return nil
	}

	err = errors.Errorf("unexpected destination type %v", reflect.TypeOf(inAction.GetDestination()).Name())
	logger := contextutils.LoggerFrom(ctx)
	logger.DPanic("error: %v", err)
	return err
}

func configureMultiDest(
	in []*v1.WeightedDestination,
	outAction *envoy_config_route_v3.RouteAction,
	filterName string,
	perFilterConfig PerFilterConfigFunc,
) error {

	multiClusterSpecifier, ok := outAction.GetClusterSpecifier().(*envoy_config_route_v3.RouteAction_WeightedClusters)
	if !ok {
		return errors.Errorf("input destination Multi but output destination was not")
	}
	out := multiClusterSpecifier.WeightedClusters

	if len(in) != len(out.GetClusters()) {
		return errors.Errorf("number of input destinations did not match number of destination weighted clusters")
	}
	for i := range in {
		if out.GetClusters()[i].GetTypedPerFilterConfig() == nil {
			out.GetClusters()[i].TypedPerFilterConfig = make(map[string]*any.Any)
		}
		err := configureSingleDest(in[i].GetDestination(), out.GetClusters()[i].GetTypedPerFilterConfig(), filterName, perFilterConfig)
		if err != nil {
			return err
		}
	}

	return nil
}

func configureSingleDest(
	in *v1.Destination,
	out map[string]*any.Any,
	filterName string,
	perFilterConfig PerFilterConfigFunc,
) error {
	config, err := perFilterConfig(in)
	if err != nil {
		return err
	}
	return setConfig(out, filterName, config)
}

func setConfig(out map[string]*any.Any, filterName string, config proto.Message) error {
	if config == nil {
		return nil
	}
	configStruct, err := utils.MessageToAny(config)
	if err != nil {
		return err
	}
	out[filterName] = configStruct
	return nil
}

package pluginutils

import (
	"context"
	"reflect"

	v1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"

	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/any"
	errors "github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"

	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/go-utils/contextutils"
)

type ModifyFunc func(existing *any.Any) (proto.Message, error)

func nopMod(protoext proto.Message) ModifyFunc {
	return func(existing *any.Any) (proto.Message, error) {
		return protoext, nil
	}
}

func SetRoutePerFilterConfig(out *envoy_config_route_v3.Route, filterName string, protoext proto.Message) error {
	return ModifyRoutePerFilterConfig(out, filterName, nopMod(protoext))
}
func SetVhostPerFilterConfig(out *envoy_config_route_v3.VirtualHost, filterName string, protoext proto.Message) error {
	return ModifyVhostPerFilterConfig(out, filterName, nopMod(protoext))
}
func SetWeightedClusterPerFilterConfig(
	out *envoy_config_route_v3.WeightedCluster_ClusterWeight,
	filterName string,
	protoext proto.Message,
) error {
	return ModifyWeightedClusterPerFilterConfig(out, filterName, nopMod(protoext))
}

func ModifyRoutePerFilterConfig(out *envoy_config_route_v3.Route, filterName string, mod ModifyFunc) error {
	if out.GetTypedPerFilterConfig() == nil {
		out.TypedPerFilterConfig = make(map[string]*any.Any)
	}
	protoext, err := mod(out.GetTypedPerFilterConfig()[filterName])
	if err != nil {
		return err
	}
	return setConfig(out.GetTypedPerFilterConfig(), filterName, protoext)
}
func ModifyVhostPerFilterConfig(out *envoy_config_route_v3.VirtualHost, filterName string, mod ModifyFunc) error {
	if out.GetTypedPerFilterConfig() == nil {
		out.TypedPerFilterConfig = make(map[string]*any.Any)
	}
	protoext, err := mod(out.GetTypedPerFilterConfig()[filterName])
	if err != nil {
		return err
	}
	return setConfig(out.GetTypedPerFilterConfig(), filterName, protoext)
}
func ModifyWeightedClusterPerFilterConfig(
	out *envoy_config_route_v3.WeightedCluster_ClusterWeight,
	filterName string,
	mod ModifyFunc,
) error {
	if out.GetTypedPerFilterConfig() == nil {
		out.TypedPerFilterConfig = make(map[string]*any.Any)
	}
	protoext, err := mod(out.GetTypedPerFilterConfig()[filterName])
	if err != nil {
		return err
	}
	return setConfig(out.GetTypedPerFilterConfig(), filterName, protoext)
}

// Return Per-Filter config for destinations, we put them on the Route (single dest) or WeightedCluster (multi dest)
type PerFilterConfigFunc func(spec *v1.Destination) (proto.Message, error)
type PerFilterConfigModifyFunc func(spec *v1.Destination, existing *any.Any) (proto.Message, error)

func ModifyPerFilterConfig(
	ctx context.Context,
	snap *v1snap.ApiSnapshot,
	in *v1.Route,
	out *envoy_config_route_v3.Route,
	filterName string,
	perFilterConfig PerFilterConfigModifyFunc,
) error {
	if in.GetRouteAction() == nil {
		// nothing to configure
		return nil
	}
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
	case *v1.RouteAction_ClusterHeader:
		// intentionally ignored because destination is specified at runtime, so perFilterConfig is useless
		return nil
	case *v1.RouteAction_DynamicForwardProxy:
		// not supported on generated cluster (no user-provided upstream needed).
		// if we want to create an upstream so upstream-specific config has a place to live (e.g. tls), then we can
		// support this on a new upstream type (i.e. will be covered by `configureSingleDest()` code path)
		return nil
	default:
		err = errors.New("unexpected destination type that is nil")
		destination := inAction.GetDestination()
		if destination != nil {
			err = errors.Errorf("unexpected destination type %v", reflect.TypeOf(destination).Name())
		}
		logger := contextutils.LoggerFrom(ctx)
		logger.DPanic("error: %v", err)
		return err
	}
}

// call this from
func MarkPerFilterConfig(
	ctx context.Context,
	snap *v1snap.ApiSnapshot,
	in *v1.Route,
	out *envoy_config_route_v3.Route,
	filterName string,
	perFilterConfig PerFilterConfigFunc,
) error {
	return ModifyPerFilterConfig(ctx, snap, in, out, filterName, func(spec *v1.Destination, existing *any.Any) (proto.Message, error) {
		return perFilterConfig(spec)
	})
}

func configureMultiDest(
	in []*v1.WeightedDestination,
	outAction *envoy_config_route_v3.RouteAction,
	filterName string,
	perFilterConfig PerFilterConfigModifyFunc,
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
	perFilterConfig PerFilterConfigModifyFunc,
) error {
	config, err := perFilterConfig(in, out[filterName])
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

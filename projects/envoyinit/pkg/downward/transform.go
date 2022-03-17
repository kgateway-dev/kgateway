package downward

import (
	"io"

	envoy_config_bootstrap "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	structpb "github.com/golang/protobuf/ptypes/struct"

	// register all top level types used in the bootstrap config
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
)

func Transform(in io.Reader, out io.Writer) error {
	return NewInterpolator().InterpolateIO(in, out, RetrieveDownwardAPI())
}

func TransformConfigTemplatesWithApi(bootstrap *envoy_config_bootstrap.Bootstrap, api DownwardAPI) error {
	interpolator := NewInterpolator()

	var err error

	interpolate := func(s *string) error { return interpolator.InterpolateString(s, api) }
	// interpolate the ID templates:
	err = interpolate(&bootstrap.GetNode().Cluster)
	if err != nil {
		return err
	}

	err = interpolate(&bootstrap.GetNode().Id)
	if err != nil {
		return err
	}

	if err := transformStruct(interpolate, bootstrap.GetNode().GetMetadata()); err != nil {
		return err
	}

	// Interpolate Static Resources
	for _, cluster := range bootstrap.GetStaticResources().GetClusters() {
		for _, endpoint := range cluster.GetLoadAssignment().GetEndpoints() {
			for _, lbEndpoint := range endpoint.GetLbEndpoints() {
				socketAddress := lbEndpoint.GetEndpoint().GetAddress().GetSocketAddress()
				if socketAddress != nil {
					if err = interpolate(&socketAddress.Address); err != nil {
						return err
					}
				}
			}
		}
	}

	return nil
}

func transformValue(interpolate func(*string) error, v *structpb.Value) error {
	switch v := v.GetKind().(type) {
	case *structpb.Value_StringValue:
		return interpolate(&v.StringValue)
	case *structpb.Value_StructValue:
		return transformStruct(interpolate, v.StructValue)
	case *structpb.Value_ListValue:
		for _, val := range v.ListValue.GetValues() {
			if err := transformValue(interpolate, val); err != nil {
				return err
			}
		}
	}
	return nil
}

func transformStruct(interpolate func(*string) error, s *structpb.Struct) error {
	if s == nil {
		return nil
	}

	for _, v := range s.GetFields() {
		if err := transformValue(interpolate, v); err != nil {
			return err
		}
	}
	return nil
}

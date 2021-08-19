package downward

import (
	"io"
	"os"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	structpb "github.com/golang/protobuf/ptypes/struct"

	// register all top level types used in the bootstrap config
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
)

type Transformer struct {
	transformations []func(node *envoy_config_core_v3.Node) error
}

func NewTransformer() *Transformer {
	return &Transformer{
		transformations: []func(node *envoy_config_core_v3.Node) error{TransformConfigTemplates},
	}
}

func (t *Transformer) TransformFiles(in, out string) error {
	inreader, err := os.Open(in)
	if err != nil {
		return err
	}
	defer inreader.Close()

	outwriter, err := os.Create(out)
	if err != nil {
		return err
	}
	defer outwriter.Close()
	return t.Transform(inreader, outwriter)
}

func (t *Transformer) Transform(in io.Reader, out io.Writer) error {
	return NewInterpolator().InterpolateIO(in, out, RetrieveDownwardAPI())
}

func TransformConfigTemplates(node *envoy_config_core_v3.Node) error {
	api := RetrieveDownwardAPI()
	return TransformConfigTemplatesWithApi(node, api)
}

func TransformConfigTemplatesWithApi(node *envoy_config_core_v3.Node, api DownwardAPI) error {

	interpolator := NewInterpolator()

	var err error

	interpolate := func(s *string) error { return interpolator.InterpolateString(s, api) }
	// interpolate the ID templates:
	err = interpolate(&node.Cluster)
	if err != nil {
		return err
	}

	err = interpolate(&node.Id)
	if err != nil {
		return err
	}

	transformStruct(interpolate, node.GetMetadata())

	return nil
}
func transformValue(interpolate func(*string) error, v *structpb.Value) error {
	switch v := v.GetKind().(type) {
	case (*structpb.Value_StringValue):
		return interpolate(&v.StringValue)
	case (*structpb.Value_StructValue):
		return transformStruct(interpolate, v.StructValue)
	case (*structpb.Value_ListValue):
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

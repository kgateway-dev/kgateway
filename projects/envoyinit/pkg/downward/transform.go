package downward

import (
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"os"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	structpb "github.com/golang/protobuf/ptypes/struct"

	envoy_config_bootstrap "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	"github.com/golang/protobuf/jsonpb"
	yaml "gopkg.in/yaml.v2"

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
	// first step - serialize yaml to json
	jsondata, err := getJson(in)

	if err != nil {
		return err
	}

	var genericBootstrap map[string]interface{}
	err = json.Unmarshal(jsondata, &genericBootstrap)
	var bootstrapConfig envoy_config_bootstrap.Bootstrap
	err = jsonpb.UnmarshalString(string(jsondata), &bootstrapConfig)
	if err != nil {
		return err
	}

	node, ok := genericBootstrap["node"]
	if !ok {
		return errors.New("Could not find envoy node in input object")
	}

	remarshaled, err := json.Marshal(node)
	if err != nil {
		return err
	}

	var realNode envoy_config_core_v3.Node
	err = jsonpb.UnmarshalString(string(remarshaled), &realNode)
	if err != nil {
		return err
	}

	for _, transformation := range t.transformations {
		err := transformation(&realNode)
		if err != nil {
			return err
		}
	}

	byt, err := json.Marshal(realNode)
	if err != nil {
		return nil
	}

	var remarshalledNode map[string]interface{}
	if err := json.Unmarshal(byt, &remarshalledNode); err != nil {
		return err
	}

	genericBootstrap["node"] = remarshalledNode

	jsn, err := json.Marshal(genericBootstrap)
	if err != nil {
		return err
	}
	_, err = out.Write(jsn)
	return err
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

	transformStruct(interpolate, node.Metadata)

	return nil
}
func transformValue(interpolate func(*string) error, v *structpb.Value) error {
	switch v := v.Kind.(type) {
	case (*structpb.Value_StringValue):
		return interpolate(&v.StringValue)
	case (*structpb.Value_StructValue):
		return transformStruct(interpolate, v.StructValue)
	case (*structpb.Value_ListValue):
		for _, val := range v.ListValue.Values {
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

	for _, v := range s.Fields {
		if err := transformValue(interpolate, v); err != nil {
			return err
		}
	}
	return nil
}

func getJson(in io.Reader) ([]byte, error) {
	readbytes, err := ioutil.ReadAll(in)
	if err != nil {
		return nil, err
	}
	var body interface{}
	if err := yaml.Unmarshal(readbytes, &body); err != nil {
		return nil, err
	}
	body = convert(body)
	if b, err := json.Marshal(body); err != nil {
		return nil, err
	} else {
		return b, nil
	}
}
func convert(i interface{}) interface{} {
	switch x := i.(type) {
	case map[interface{}]interface{}:
		m2 := map[string]interface{}{}
		for k, v := range x {
			m2[k.(string)] = convert(v)
		}
		return m2
	case []interface{}:
		for i, v := range x {
			x[i] = convert(v)
		}
	}
	return i
}

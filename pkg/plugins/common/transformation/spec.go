package transformation

import (
	"github.com/gogo/protobuf/types"
	"github.com/solo-io/gloo/pkg/protoutil"
)

// this goes on the route extension
type RouteExtension struct {
	Parameters       *Parameters `json:"parameters,omitempty"`
	ResponseTemplate *Template   `json:"response_template,omitempty"`
	ResponseParams   *Parameters `json:"response_parameters,omitempty"`
}

type Parameters struct {
	// headers that will be used to derive the data for processing the output templates
	// if no syntax containing {variables} are detected in the header value,
	// the whole value will be substituted by its name into the template
	// for example:
	/*
		input:
			header_parmeters:
		      x-header-foo: bar
		output:
			body_template: "{\"path\": {{ path }}}"
	*/
	Headers   map[string]string `json:"headers,omitempty"`
	Path      *string           `json:"path,omitempty"`
	Authority *string           `json:"authority,omitempty"`
	//TODO: support query params
	//TODO: support form params
}

// SPECIAL VARIABLE NAMES:
// these variables will be provided even if there is no parameter source
// path
// method
// scheme
// authority
// any field from the request body, assuming it's json

// this goes on the function spec
// or on the response transformation
type Template struct {
	Path   string            `json:"path"`
	Header map[string]string `json:"headers"`
	// body is a pointer because, if null, pass through original body
	Body *string `json:"body"`
	// if enabled, the request body will be passed through untouched
	PassthroughBody bool `json:"passthrough_body"`
	//TODO: support query template
	//TODO: support form template
}

func DecodeRouteExtension(generic *types.Struct) (RouteExtension, error) {
	var s RouteExtension
	err := protoutil.UnmarshalStruct(generic, &s)
	return s, err
}

func EncodeRouteExtension(spec RouteExtension) *types.Struct {
	v1Spec, err := protoutil.MarshalStruct(spec)
	if err != nil {
		panic(err)
	}
	return v1Spec
}

func DecodeFunctionSpec(generic *types.Struct) (Template, error) {
	var s Template
	err := protoutil.UnmarshalStruct(generic, &s)
	return s, err
}

func EncodeFunctionSpec(spec Template) *types.Struct {
	v1Spec, err := protoutil.MarshalStruct(spec)
	if err != nil {
		panic(err)
	}
	return v1Spec
}

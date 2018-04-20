package extensions

import (
	"time"

	"github.com/gogo/protobuf/types"
	"github.com/solo-io/gloo/pkg/protoutil"
)

type RouteExtensionSpec struct {
	AddRequestHeaders     []HeaderValue `json:"add_request_headers,omitempty"`
	AddResponseHeaders    []HeaderValue `json:"add_response_headers,omitempty"`
	RemoveResponseHeaders []string      `json:"remove_response_headers,omitempty"`

	MaxRetries  uint32        `json:"max_retries,omitempty"`
	Timeout     time.Duration `json:"timeout,omitempty"`
	HostRewrite string        `json:"host_rewrite,omitempty"`

	Cors *CorsPolicy `json:"cors",omitempty`
	//TODO: support RateLimit
}

type HeaderValue struct {
	Key    string `json:"key,omitempty"`
	Value  string `json:"value,omitempty"`
	Append bool   `json:"append,omitempty"`
}

type CorsPolicy struct {
	AllowOrigin      []string      `json:"allow_origin",omitempty`
	AllowMethods     string        `json:"allow_methods",omitempty`
	AllowHeaders     string        `json:"allow_headers",omitempty`
	ExposeHeaders    string        `json:"expose_headers",omitempty`
	MaxAge           time.Duration `json:"max_age",omitempty`
	AllowCredentials bool          `json:"allow_credentials",omitempty`
}

func DecodeRouteExtensions(generic *types.Struct) (RouteExtensionSpec, error) {
	var s RouteExtensionSpec
	err := protoutil.UnmarshalStruct(generic, &s)
	return s, err
}

func EncodeRouteExtensionSpec(spec RouteExtensionSpec) *types.Struct {
	v1Spec, err := protoutil.MarshalStruct(spec)
	if err != nil {
		panic(err)
	}
	return v1Spec
}

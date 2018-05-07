package helpers

import (
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/protoutil"
)

func NewTestConfig() *v1.Config {
	upstreams := []*v1.Upstream{
		NewTestUpstream1(),
		NewTestUpstream2(),
	}
	virtualServices := []*v1.VirtualService{
		NewTestVirtualService("my-vservice", NewTestRoute1(), NewTestRoute2()),
		NewTestVirtualService("my-vservice-2", NewTestRoute1(), NewTestRoute2()),
	}
	return &v1.Config{
		Upstreams:       upstreams,
		VirtualServices: virtualServices,
	}
}

func NewTestUpstream1() *v1.Upstream {
	usSpec, _ := protoutil.MarshalStruct(map[string]interface{}{
		"region":         "us-east-1",
		"secret_key_ref": "my-secret-key",
		"access_key_ref": "my-access-key",
	})
	fnSpec, _ := protoutil.MarshalStruct(map[string]interface{}{
		"key": "value",
	})
	return &v1.Upstream{
		Name: "aws",
		Type: "lambda",
		Spec: usSpec,
		Functions: []*v1.Function{
			{
				Name: "my_lambda_function",
				Spec: fnSpec,
			},
		},
		Metadata: &v1.Metadata{
			Annotations: map[string]string{"my_annotation": "value"},
		},
	}
}
func NewTestUpstream2() *v1.Upstream {
	return &v1.Upstream{
		Name: "localhost-python",
		// TODO: revert this to using service plugin, and move this file to its own package
		// to prevent cyclical imports
		Type: "service",
		Metadata: &v1.Metadata{
			Annotations: map[string]string{"my_annotation": "value"},
		},
	}
}

func NewTestVirtualService(name string, routes ...*v1.Route) *v1.VirtualService {
	return &v1.VirtualService{
		Name:   name,
		Routes: routes,
		Metadata: &v1.Metadata{
			Annotations: map[string]string{"my_annotation": "value"},
		},
	}
}

func NewTestRoute1() *v1.Route {
	extensions, _ := protoutil.MarshalStruct(map[string]interface{}{
		"auth": map[string]interface{}{
			"credentials": struct {
				Username, Password string
			}{
				Username: "alice",
				Password: "bob",
			},
			"token": "my-12345",
		}})
	return &v1.Route{
		Matcher: &v1.Route_RequestMatcher{
			RequestMatcher: &v1.RequestMatcher{
				Path: &v1.RequestMatcher_PathPrefix{
					PathPrefix: "/foo",
				},
				Headers: map[string]string{"x-foo-bar": ""},
				Verbs:   []string{"GET", "POST"},
			},
		},
		SingleDestination: &v1.Destination{
			DestinationType: &v1.Destination_Function{
				Function: &v1.FunctionDestination{
					FunctionName: "foo",
					UpstreamName: "aws",
				},
			},
		},
		Extensions: extensions,
	}
}

func NewTestRoute2() *v1.Route {
	extensions, _ := protoutil.MarshalStruct(map[string]interface{}{
		"auth": map[string]interface{}{
			"credentials": struct {
				Username, Password string
			}{
				Username: "alice",
				Password: "bob",
			},
			"token": "my-12345",
		}})
	return &v1.Route{
		Matcher: &v1.Route_RequestMatcher{
			RequestMatcher: &v1.RequestMatcher{
				Path: &v1.RequestMatcher_PathExact{
					PathExact: "/bar",
				},
				Verbs: []string{"GET", "POST"},
			},
		},
		SingleDestination: &v1.Destination{
			DestinationType: &v1.Destination_Upstream{
				Upstream: &v1.UpstreamDestination{
					Name: "my-upstream",
				},
			},
		},
		Extensions: extensions,
	}
}

func NewTestRouteWithCORS() *v1.Route {
	extensions, _ := protoutil.MarshalStruct(map[string]interface{}{
		"cors": map[string]interface{}{
			"allow_origin":  []string{"*.solo.io"},
			"allow_methods": "GET, POST",
			"max_age":       86400000000000,
		},
		"auth": map[string]interface{}{
			"credentials": struct {
				Username, Password string
			}{
				Username: "alice",
				Password: "bob",
			},
			"token": "my-12345",
		}})
	return &v1.Route{
		Matcher: &v1.Route_RequestMatcher{
			RequestMatcher: &v1.RequestMatcher{
				Path: &v1.RequestMatcher_PathExact{
					PathExact: "/bar",
				},
				Verbs: []string{"GET", "POST"},
			},
		},
		SingleDestination: &v1.Destination{
			DestinationType: &v1.Destination_Upstream{
				Upstream: &v1.UpstreamDestination{
					Name: "my-upstream",
				},
			},
		},
		Extensions: extensions,
	}
}

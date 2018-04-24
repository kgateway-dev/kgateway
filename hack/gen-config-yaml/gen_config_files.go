package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ghodss/yaml"
	"github.com/gogo/protobuf/types"
	"github.com/solo-io/gloo/pkg/log"
	"github.com/solo-io/gloo/pkg/protoutil"

	"encoding/json"

	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/coreplugins/service"
	"github.com/solo-io/gloo/pkg/plugins/aws"
	"github.com/solo-io/gloo/pkg/storage/crd"
)

var upstreamAddr string

var upstreamHost string
var upstreamPort uint32

var upstreamName = "localhost8000"

var configType = flag.String("config", "test", "one of: test, lambda")
var storageType = flag.String("type", "file", "one of: crd or file")

func getConfig() v1.Config {
	switch *configType {
	case "test":
		return NewTestConfig()
	case "lambda":
		return NewλConfig()
	}
	panic("No such config")
}

func main() {
	flag.StringVar(&upstreamAddr, "addr", "localhost:8000", "upstream addr")
	flag.Parse()
	parts := strings.Split(upstreamAddr, ":")
	upstreamHost = parts[0]
	p, err := strconv.Atoi(parts[1])
	must(err)
	upstreamPort = uint32(p)
	cfg := getConfig()
	outDir := "_gloo_config"
	err = os.MkdirAll(filepath.Join(outDir, "upstreams"), 0755)
	must(err)
	err = os.MkdirAll(filepath.Join(outDir, "virtualservices"), 0755)
	must(err)
	for _, upstream := range cfg.Upstreams {
		var jsn []byte
		if *storageType == "crd" {
			us, err := crd.UpstreamToCrd("put-namespace-here", upstream)
			must(err)
			us.TypeMeta.APIVersion = "gloo.solo.io/v1"
			us.TypeMeta.Kind = "Upstream"
			jsn, err = json.Marshal(us)
			must(err)
		} else {
			jsn, err = protoutil.Marshal(upstream)
			must(err)
		}
		data, err := yaml.JSONToYAML(jsn)
		must(err)
		filename := filepath.Join(outDir, "upstreams", fmt.Sprintf("upstream-%v.yml", upstream.Name))
		err = ioutil.WriteFile(filename, data, 0644)
		must(err)
	}
	for _, virtualService := range cfg.VirtualServices {
		var jsn []byte
		if *storageType == "crd" {
			vs, err := crd.VirtualServiceToCrd("put-namespace-here", virtualService)
			must(err)
			vs.TypeMeta.APIVersion = "gloo.solo.io/v1"
			vs.TypeMeta.Kind = "VirtualService"
			jsn, err = json.Marshal(vs)
			must(err)
		} else {
			jsn, err = protoutil.Marshal(virtualService)
			must(err)
		}
		data, err := yaml.JSONToYAML(jsn)
		must(err)
		log.GreyPrintf("%s", jsn)
		filename := filepath.Join(outDir, "virtualservices", fmt.Sprintf("virtualservice-%v.yml", virtualService.Name))
		err = ioutil.WriteFile(filename, data, 0644)
		must(err)
	}
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func toProtomessageUnTyped(generic interface{}) *types.Struct {
	m, err := protoutil.MarshalStruct(generic)
	must(err)
	return m
}

func NewλConfig() v1.Config {
	upstreams := []*v1.Upstream{
		{
			Name: "useast1",
			Type: aws.UpstreamTypeAws,
			Spec: toProtomessageUnTyped(&aws.UpstreamSpec{
				Region:    "us-east-1",
				SecretRef: "aws-secret",
			}),
			Functions: []*v1.Function{{
				Name: "up",
				Spec: toProtomessageUnTyped(&aws.FunctionSpec{FunctionName: "uppercase", Qualifier: "1"}),
			}},
		},
	}
	virtualServices := []*v1.VirtualService{
		NewTestVirtualService("localhost-app", NewλRoute()),
	}
	return v1.Config{
		Upstreams:       upstreams,
		VirtualServices: virtualServices,
	}
}

func NewTestConfig() v1.Config {
	upstreams := []*v1.Upstream{
		{
			Name: upstreamName,
			Type: service.UpstreamTypeService,
			Spec: service.EncodeUpstreamSpec(service.UpstreamSpec{
				Hosts: []service.Host{
					{Addr: upstreamHost, Port: upstreamPort},
				},
			}),
		},
	}
	virtualServices := []*v1.VirtualService{
		NewTestVirtualService("localhost-app", NewTestRoute(), NewTestRouteMultiDest()),
	}
	return v1.Config{
		Upstreams:       upstreams,
		VirtualServices: virtualServices,
	}
}

func NewTestVirtualService(name string, routes ...*v1.Route) *v1.VirtualService {
	return &v1.VirtualService{
		Name:   name,
		Routes: routes,
	}
}

func NewλRoute() *v1.Route {
	return &v1.Route{
		Matcher: &v1.Route_RequestMatcher{
			RequestMatcher: &v1.RequestMatcher{
				Path: &v1.RequestMatcher_PathPrefix{
					PathPrefix: "/lambda",
				},
			},
		},
		SingleDestination: &v1.Destination{
			DestinationType: &v1.Destination_Function{
				Function: &v1.FunctionDestination{
					UpstreamName: "useast1",
					FunctionName: "up",
				},
			},
		},
	}
}
func NewTestRoute() *v1.Route {
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
			DestinationType: &v1.Destination_Upstream{
				Upstream: &v1.UpstreamDestination{
					Name: upstreamName,
				},
			},
		},
	}
}

func NewTestRouteMultiDest() *v1.Route {
	return &v1.Route{
		Matcher: &v1.Route_EventMatcher{
			EventMatcher: &v1.EventMatcher{
				EventType: "my-event",
			},
		},
		MultipleDestinations: []*v1.WeightedDestination{
			{
				Destination: &v1.Destination{
					DestinationType: &v1.Destination_Upstream{
						Upstream: &v1.UpstreamDestination{
							Name: upstreamName,
						},
					},
				},
				Weight: 5,
			},
			{
				Destination: &v1.Destination{
					DestinationType: &v1.Destination_Upstream{
						Upstream: &v1.UpstreamDestination{
							Name: upstreamName,
						},
					},
				},
				Weight: 10,
			},
		},
	}
}

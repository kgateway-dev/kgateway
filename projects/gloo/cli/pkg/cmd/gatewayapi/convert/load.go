package convert

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/solo-io/gloo/pkg/schemes"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gatewaykube "github.com/solo-io/gloo/projects/gateway/pkg/api/v1/kube/apis/gateway.solo.io/v1"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/gatewayapi/convert/domain"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	glookube "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/kube/apis/gloo.solo.io/v1"
	v2 "github.com/solo-io/solo-apis/pkg/api/enterprise.gloo.solo.io/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"sigs.k8s.io/yaml"
)

var runtimeScheme *runtime.Scheme
var codecs serializer.CodecFactory
var decoder runtime.Decoder

func init() {
	runtimeScheme = runtime.NewScheme()

	// Gloo Edge APIs
	if err := schemes.SchemeBuilder.AddToScheme(runtimeScheme); err != nil {
		log.Fatal(err)
	}

	codecs = serializer.NewCodecFactory(runtimeScheme)
	decoder = codecs.UniversalDeserializer()
}

func (g *GatewayAPIOutput) Load(files []string, isSnapshotFile bool) error {

	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		if isSnapshotFile {
			if err := g.translateSnapshotToEdgeInput(string(data), file); err != nil {
				return err
			}
		} else {
			if err := g.translateFileToEdgeInput(string(data), file); err != nil {
				return err
			}
		}
	}

	return nil
}

func (g *GatewayAPIOutput) PreProcess(splitMatchers bool) error {

	if splitMatchers {
		if err := g.splitRouteMatchers(); err != nil {
			return err
		}
	}
	return nil
}

// we need to split the route matchers because prefix and exact matchers cause problems with rewrites
func (g *GatewayAPIOutput) splitRouteMatchers() error {
	for _, rt := range g.edgeCache.RouteTables {
		var newRoutes []*gatewayv1.Route
		for _, route := range rt.Spec.Routes {
			editedRoute := generateRoutesForMethodMatchers(route)
			newRoutes = append(newRoutes, editedRoute)
		}
		rt.Spec.Routes = newRoutes

		g.edgeCache.AddRouteTable(rt)
	}
	return nil
}

func generateRoutesForMethodMatchers(route *gatewayv1.Route) *gatewayv1.Route {
	var newMatchers []*matchers.Matcher
	for _, m := range route.Matchers {
		if len(m.Methods) > 1 {
			// for each method we need to split out the matchers
			for _, method := range m.Methods {
				newMatcher := &matchers.Matcher{
					PathSpecifier:   m.PathSpecifier,
					CaseSensitive:   m.CaseSensitive,
					Headers:         m.Headers,
					QueryParameters: m.QueryParameters,
					Methods:         []string{method},
				}
				newMatchers = append(newMatchers, newMatcher)
			}
		} else {
			//it only has one so we just add it
			newMatchers = append(newMatchers, m)
		}
	}
	route.Matchers = newMatchers

	return route
}

type SnapshotResponseData struct {
	Data []map[string]interface{} `json:"data"`
}

func (g *GatewayAPIOutput) translateSnapshotToEdgeInput(jsonData string, fileName string) error {
	var snapshot SnapshotResponseData
	if err := json.Unmarshal([]byte(jsonData), &snapshot); err != nil {
		return err
	}
	//fmt.Println(snapshot.MarshalJSONString())
	for _, objMap := range snapshot.Data {
		// Convert to unstructured
		obj := unstructured.Unstructured{
			Object: objMap,
		}
		if err := g.parseObjects(obj, &schema.GroupVersionKind{}, fileName); err != nil {
			return err
		}
	}

	return nil
}

func (g *GatewayAPIOutput) translateFileToEdgeInput(yamlData string, fileName string) error {

	// Read the file
	for _, resourceYAML := range strings.Split(yamlData, "---") {
		if len(resourceYAML) == 0 {
			continue
		}
		// yaml to object
		obj, k, err := decoder.Decode([]byte(resourceYAML), nil, nil)
		if err != nil {
			if runtime.IsNotRegisteredError(err) {
				// we just want to add the yaml and move on
				g.edgeCache.AddYamlObject(&domain.YAMLWrapper{Yaml: yamlData, OriginalFileName: fileName})
				continue
			}

			// TODO if we cant decode it, don't do anything and continue
			//log.Printf("# Skipping object due to error file parsing error %s", err)
			continue
		}

		// a lot of times lists are missing the group so this object doesnt match
		if k.Kind == "List" {
			var list unstructured.UnstructuredList
			if err := yaml.Unmarshal([]byte(resourceYAML), &list); err != nil {
				return err
			}

			for _, item := range list.Items {
				err := g.parseObjects(item, k, fileName)
				if err != nil {
					return err
				}

			}
			continue
		}
		g.AddObjectToGatewayAPIOutput(obj, fileName, k, resourceYAML)
	}
	return nil
}

func (g *GatewayAPIOutput) parseObjects(item unstructured.Unstructured, k *schema.GroupVersionKind, fileName string) error {

	resourceYaml, err := yaml.Marshal(item)
	if err != nil {
		return err
	}
	gvk := item.GroupVersionKind()
	obj, err := runtimeScheme.New(gvk)
	if err != nil {
		return err
	}
	if runtime.IsNotRegisteredError(err) {
		// we just want to add the yaml and move on
		g.edgeCache.AddYamlObject(&domain.YAMLWrapper{Yaml: string(resourceYaml), OriginalFileName: fileName})
		return nil
	} else if err != nil {
		return err
	}

	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.Object, obj); err != nil {
		return fmt.Errorf("error converting unstructured to typed: %v", err)
	}

	g.AddObjectToGatewayAPIOutput(obj, fileName, k, string(resourceYaml))
	return nil
}

func (g *GatewayAPIOutput) AddObjectToGatewayAPIOutput(obj runtime.Object, fileName string, k *schema.GroupVersionKind, resourceYaml string) {
	switch o := obj.(type) {
	case *v2.AuthConfig:
		glooConfigMetric.WithLabelValues("AuthConfig").Inc()
		g.edgeCache.AddAuthConfig(&domain.AuthConfigWrapper{AuthConfig: o, OriginalFileName: fileName})
	case *glookube.Upstream:
		glooConfigMetric.WithLabelValues("Upstream").Inc()
		g.edgeCache.AddUpstream(&domain.UpstreamWrapper{Upstream: o, OriginalFileName: fileName})
	case *gatewaykube.RouteTable:
		glooConfigMetric.WithLabelValues("RouteTable").Inc()
		g.edgeCache.AddRouteTable(&domain.RouteTableWrapper{RouteTable: o, OriginalFileName: fileName})
	case *gatewaykube.VirtualService:
		glooConfigMetric.WithLabelValues("VirtualService").Inc()
		g.edgeCache.AddVirtualService(&domain.VirtualServiceWrapper{VirtualService: o, OriginalFileName: fileName})
	case *gatewaykube.RouteOption:
		glooConfigMetric.WithLabelValues("RouteOption").Inc()
		g.edgeCache.AddRouteOption(&domain.RouteOptionWrapper{RouteOption: o, OriginalFileName: fileName})
	case *gatewaykube.VirtualHostOption:
		glooConfigMetric.WithLabelValues("VirtualHostOption").Inc()
		g.edgeCache.AddVirtualHostOption(&domain.VirtualHostOptionWrapper{VirtualHostOption: o, OriginalFileName: fileName})
	case *gatewaykube.Gateway:
		glooConfigMetric.WithLabelValues("Gateway").Inc()
		g.edgeCache.AddGlooGateway(&domain.GlooGatewayWrapper{Gateway: o, OriginalFileName: fileName})
	case *gatewaykube.HttpListenerOption:
		glooConfigMetric.WithLabelValues("HttpListenerOption").Inc()
		g.edgeCache.AddHTTPListenerOption(&domain.HTTPListenerOptionWrapper{HttpListenerOption: o, OriginalFileName: fileName})
	case *gatewaykube.ListenerOption:
		glooConfigMetric.WithLabelValues("ListenerOption").Inc()
		g.edgeCache.AddListenerOption(&domain.ListenerOptionWrapper{ListenerOption: o, OriginalFileName: fileName})
	default:
		// if we dont know what type it is we just add it back
		// no change so just add it back
		//g.AddError(ERROR_PARSE_ERROR, "unrecognized object: %v in file %s", reflect.TypeOf(o), fileName)
		glooConfigMetric.WithLabelValues("Unknown").Inc()
		g.edgeCache.AddYamlObject(&domain.YAMLWrapper{Yaml: resourceYaml, OriginalFileName: fileName, Object: obj})
	}
}

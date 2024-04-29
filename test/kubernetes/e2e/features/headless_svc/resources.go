package headless_svc

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/ghodss/yaml"
	errors "github.com/rotisserie/eris"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/solo-io/skv2/codegen/util"
	v1 "github.com/solo-io/solo-apis/pkg/api/gateway.solo.io/v1"
	soloapis_gloov1 "github.com/solo-io/solo-apis/pkg/api/gloo.solo.io/v1"
	"github.com/solo-io/solo-apis/pkg/api/gloo.solo.io/v1/core/matchers"
	soloapis_kubernetes "github.com/solo-io/solo-apis/pkg/api/gloo.solo.io/v1/options/kubernetes"
	gloocore "github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var (
	headlessSvcSetupManifest  = filepath.Join(util.MustGetThisDir(), "inputs/setup.yaml")
	k8sApiRoutingManifest     = filepath.Join(util.MustGetThisDir(), "inputs/generated_k8s_api.yaml")
	classicApiRoutingManifest = filepath.Join(util.MustGetThisDir(), "inputs/generated_classic_api.yaml")

	headlessSvcDomain = "headless.example.com"

	// Classic Edge API resources
	getClassicEdgeResources = func(installNamespace string) []client.Object {
		headlessSvcUpstream := &soloapis_gloov1.Upstream{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Upstream",
				APIVersion: "gloo.solo.io/v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "headless-nginx-upstream",
				Namespace: installNamespace,
			},
			Spec: soloapis_gloov1.UpstreamSpec{
				UpstreamType: &soloapis_gloov1.UpstreamSpec_Kube{
					Kube: &soloapis_kubernetes.UpstreamSpec{
						Selector: map[string]string{
							"app.kubernetes.io/name": "nginx",
						},
						ServiceName:      headlessService.GetName(),
						ServiceNamespace: headlessService.GetNamespace(),
						ServicePort:      8080,
					},
				},
			},
		}

		headlessVs := &v1.VirtualService{
			TypeMeta: metav1.TypeMeta{
				Kind:       "VirtualService",
				APIVersion: "gateway.solo.io/v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "headless-vs",
				Namespace: installNamespace,
			},
			Spec: v1.VirtualServiceSpec{
				VirtualHost: &v1.VirtualHost{
					Domains: []string{headlessSvcDomain},
					Routes: []*v1.Route{{
						Matchers: []*matchers.Matcher{
							{
								PathSpecifier: &matchers.Matcher_Prefix{
									Prefix: "/",
								},
							},
						},
						Action: &v1.Route_RouteAction{
							RouteAction: &soloapis_gloov1.RouteAction{
								Destination: &soloapis_gloov1.RouteAction_Single{
									Single: &soloapis_gloov1.Destination{
										DestinationType: &soloapis_gloov1.Destination_Upstream{
											Upstream: &gloocore.ResourceRef{
												Name:      headlessSvcUpstream.Name,
												Namespace: headlessSvcUpstream.Namespace,
											},
										},
									},
								},
							},
						},
					}},
				},
			},
		}

		var resources []client.Object
		resources = append(resources, headlessVs, headlessSvcUpstream)
		return resources
	}

	gw = &gwv1.Gateway{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Gateway",
			APIVersion: "gateway.networking.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gw",
			Namespace: "default",
		},
		Spec: gwv1.GatewaySpec{
			GatewayClassName: "gloo-gateway",
			Listeners: []gwv1.Listener{
				{
					Name:     "http",
					Port:     80,
					Protocol: "HTTP",
					AllowedRoutes: &gwv1.AllowedRoutes{
						Namespaces: &gwv1.RouteNamespaces{
							From: ptrTo(gwv1.NamespacesFromSame),
						},
					},
				},
			},
		},
	}

	// k8s Gateway API resources
	headlessSvcHTTPRoute = &gwv1.HTTPRoute{
		TypeMeta: metav1.TypeMeta{
			Kind:       "HTTPRoute",
			APIVersion: "gateway.networking.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "headless-httproute",
			Namespace: gw.GetNamespace(), // use the same namespace as the Gateway because NamespacesFromSame is set
		},
		Spec: gwv1.HTTPRouteSpec{
			CommonRouteSpec: gwv1.CommonRouteSpec{
				ParentRefs: []gwv1.ParentReference{
					{
						Name: gwv1.ObjectName(gw.GetName()),
					},
				},
			},
			Hostnames: []gwv1.Hostname{gwv1.Hostname(headlessSvcDomain)},
			Rules: []gwv1.HTTPRouteRule{
				{
					BackendRefs: []gwv1.HTTPBackendRef{
						{
							BackendRef: gwv1.BackendRef{
								BackendObjectReference: gwv1.BackendObjectReference{
									Name: "headless-example-svc",
									Port: ptrTo(gwv1.PortNumber(8080)),
								},
							},
						},
					},
				},
			},
		},
	}
)

func writeResourcesToFile(resources []client.Object, fileName string) error {
	// Marshal resources to YAML
	outputResourceManifest := &bytes.Buffer{}
	for _, resource := range resources {
		yamlData, err := objectToYaml(resource)
		if err != nil {
			return errors.Wrap(err, "can marshal resources to YAML")
		}

		outputResourceManifest.Write(yamlData)

		// Separate resources with '---'
		outputResourceManifest.WriteString("\n---\n")
	}

	// Write YAML data to file
	manifestFile, err := os.Create(fileName)
	if err != nil {
		return errors.Wrap(err, "can create generated file")
	}
	defer manifestFile.Close()

	_, err = manifestFile.Write(outputResourceManifest.Bytes())
	if err != nil {
		return errors.Wrap(err, "can write resources to file")
	}
	return nil
}

func ptrTo[T any](a T) *T {
	return &a
}

func objectToYaml(obj client.Object) ([]byte, error) {
	jsonBytes, err := json.Marshal(obj)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal resource to JSON")
	}

	yamlBytes, err := yaml.JSONToYAML(jsonBytes)
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert resource JSON to YAML")
	}

	return cleanUp(yamlBytes), nil
}

func cleanUp(objYaml []byte) []byte {
	var lines []string
	scan := bufio.NewScanner(bytes.NewBuffer(objYaml))
	for scan.Scan() {
		line := scan.Text()
		if isNullCreationTime(line) {
			continue
		}

		lines = append(lines, line)
	}

	if len(lines) == 0 {
		return nil
	}

	return []byte(strings.Join(lines, "\n"))
}

func isNullCreationTime(line string) bool {
	return strings.TrimSpace(line) == "creationTimestamp: null"
}

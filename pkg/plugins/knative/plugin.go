package knative

import (
	fmt "fmt"
	"net"
	"os"
	"strconv"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/pkg/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	envoyendpoint "github.com/envoyproxy/go-control-plane/envoy/api/v2/endpoint"
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/gogo/protobuf/types"

	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/pkg/endpointdiscovery"
	"github.com/solo-io/gloo/pkg/plugins"
	kubeutils "github.com/solo-io/gloo/pkg/utils/kube"
)

const (
	MetadataNamespace   = "io.solo.knative"
	UpstreamTypeKnative = "knative"
)

func init() {
	plugins.Register(&Plugin{})
}

//go:generate protoc -I=./ -I=${GOPATH}/src/github.com/gogo/protobuf/ -I=${GOPATH}/src/github.com/gogo/protobuf/protobuf/ --gogo_out=Mgoogle/protobuf/wrappers.proto=github.com/gogo/protobuf/types:${GOPATH}/src spec.proto

type Plugin struct {
	kubeConfig         *rest.Config
	knativeIngress     string
	knativeIngressPort int32
}

const (
	// define Upstream type name
	UpstreamTypeKNative = "knative"
	HostnameKNative     = "hostname"
)

// TODO: we are taking advantage of the fact that this function is called first and use it like a contructor.
// should refactor to a real init function
func (p *Plugin) SetupEndpointDiscovery(opts bootstrap.Options) (endpointdiscovery.Interface, error) {
	masterUrl := opts.KubeOptions.MasterURL
	kubeconfigPath := opts.KubeOptions.KubeConfig

	cfg, err := kubeutils.GetConfig(masterUrl, kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to build rest config: %v", err)
	}
	p.kubeConfig = cfg
	return nil, nil
}

func (p *Plugin) GetDependencies(_ *v1.Config) *plugins.Dependencies {
	return nil
}

func (p *Plugin) verifytKNative() error {
	if p.knativeIngress != "" {
		return nil
	}
	fakeIngress := os.Getenv("FAKE_KNATIVE_INGRESS")
	if fakeIngress != "" {
		//TODO: get the up and port from the fake knative var FAKE_KNATIVE_INGRESS
		// create local e2e that checks that the header kombina works and that we get the right host header.
		host, port, err := net.SplitHostPort(fakeIngress)
		if err == nil {
			if portint, err := strconv.ParseInt(port, 0, 0); err == nil {
				p.knativeIngress = host
				p.knativeIngressPort = (int32)(portint)
				return nil
			}
		}
	}

	if p.kubeConfig == nil {
		return errors.New("no valid kube config for knative")
	}
	kubeClient, err := kubernetes.NewForConfig(p.kubeConfig)
	if err != nil {
		return fmt.Errorf("failed to create kube clientset: %v", err)
	}

	servicesClient := kubeClient.CoreV1().Services("istio-system")

	listOptions := meta_v1.ListOptions{LabelSelector: "knative=ingressgateway"}

	svcList, err := servicesClient.List(listOptions)
	if err != nil {
		return fmt.Errorf("failed to list services: %v", err)
	}

	services := svcList.Items

	if len(services) == 0 {
		return fmt.Errorf("no knative ingress found: %v", err)
	}

	// There should be only one, so grap the first one with a cluster IP
	for _, service := range services {
		if service.Spec.ClusterIP != "" {
			for _, port := range service.Spec.Ports {
				if port.Name == "http" {
					p.knativeIngress = service.Spec.ClusterIP
					p.knativeIngressPort = port.Port
					return nil
				}
			}
		}
	}

	return errors.New("can't find knative ingress")
}

func (p *Plugin) ProcessUpstream(_ *plugins.UpstreamPluginParams, in *v1.Upstream, out *envoyapi.Cluster) error {
	if in.Type != UpstreamTypeKNative {
		return nil
	}

	// decode does validation for us
	spec, err := DecodeUpstreamSpec(in.Spec)
	if err != nil {
		return errors.Wrap(err, "invalid knative upstream spec")
	}

	if err := p.verifytKNative(); err != nil {
		return err
	}

	out.Type = envoyapi.Cluster_STATIC

	out.LoadAssignment = &envoyapi.ClusterLoadAssignment{
		ClusterName: out.Name,
		Endpoints: []envoyendpoint.LocalityLbEndpoints{{
			LbEndpoints: []envoyendpoint.LbEndpoint{{
				HealthStatus: envoycore.HealthStatus_HEALTHY,
				Metadata: &envoycore.Metadata{
					FilterMetadata: map[string]*types.Struct{
						MetadataNamespace: &types.Struct{
							Fields: map[string]*types.Value{
								HostnameKNative: &types.Value{
									Kind: &types.Value_StringValue{
										StringValue: spec.Hostname,
									},
								},
							},
						},
					},
				},
				Endpoint: &envoyendpoint.Endpoint{
					Address: &envoycore.Address{
						Address: &envoycore.Address_SocketAddress{
							SocketAddress: &envoycore.SocketAddress{
								Protocol: envoycore.TCP,
								Address:  p.knativeIngress,
								PortSpecifier: &envoycore.SocketAddress_PortValue{
									PortValue: (uint32)(p.knativeIngressPort),
								},
							},
						},
					},
				},
			}},
		}},
	}

	out.LoadAssignment.Endpoints[0].LbEndpoints = append(out.LoadAssignment.Endpoints[0].LbEndpoints)
	out.EdsClusterConfig = &envoyapi.Cluster_EdsClusterConfig{
		EdsConfig: &envoycore.ConfigSource{
			ConfigSourceSpecifier: &envoycore.ConfigSource_Ads{
				Ads: &envoycore.AggregatedConfigSource{},
			},
		},
	}
	return nil
}

func (p *Plugin) ProcessRoute(pluginParams *plugins.RoutePluginParams, in *v1.Route, out *envoyroute.Route) error {

	// if we have one knative upstream, the
	if route, ok := out.Action.(*envoyroute.Route_Route); ok {
		headervalue := "%UPSTREAM_METADATA([\"" + MetadataNamespace + "\", \"" + HostnameKNative + "\"])%"
		route.Route.RequestHeadersToAdd = append(route.Route.RequestHeadersToAdd, &envoycore.HeaderValueOption{
			Header: &envoycore.HeaderValue{
				Key:   ":authority",
				Value: headervalue,
			},
			Append: &types.BoolValue{Value: false},
		})
		route.Route.RequestHeadersToAdd = append(route.Route.RequestHeadersToAdd, &envoycore.HeaderValueOption{
			Header: &envoycore.HeaderValue{
				Key:   "x-yuval",
				Value: "yuval-" + headervalue,
			},
		})
	}

	return nil
}

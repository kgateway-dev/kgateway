package knative

import (
	fmt "fmt"
	"net"
	"os"
	"strconv"
	"time"

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
	"github.com/solo-io/gloo/pkg/plugins"
	kubeutils "github.com/solo-io/gloo/pkg/utils/kube"
)

const (
	MetadataNamespace = "io.solo.knative"
	// define Upstream type name

	UpstreamTypeKnative = "knative"
	HostnameKNative     = "hostname"
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

func (p *Plugin) Init(opts bootstrap.Options) error {
	masterUrl := opts.KubeOptions.MasterURL
	kubeconfigPath := opts.KubeOptions.KubeConfig

	cfg, err := kubeutils.GetConfig(masterUrl, kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to build rest config: %v", err)
	}
	p.kubeConfig = cfg
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

	// There should be only one, so grab the first one with a cluster IP
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
	if in.Type != UpstreamTypeKnative {
		return nil
	}

	// decode does validation for us
	_, err := DecodeUpstreamSpec(in.Spec)
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
	return nil
}

func findUpstream(ClusterName plugins.EnvoyNameForUpstream, clustername string, upstreams []*v1.Upstream) *v1.Upstream {
	for _, us := range upstreams {
		if ClusterName(us.Name) == clustername {
			return us
		}
	}
	return nil
}

func maybeAppendHeader(pluginParams *plugins.RoutePluginParams, clustername string, headers []*envoycore.HeaderValueOption) ([]*envoycore.HeaderValueOption, bool) {
	var knativeroute bool
	if us := findUpstream(pluginParams.EnvoyNameForUpstream, clustername, pluginParams.Upstreams); us != nil {
		if us.Type == UpstreamTypeKnative {
			if spec, err := DecodeUpstreamSpec(us.Spec); err == nil {
				knativeroute = true
				headers = append(headers, &envoycore.HeaderValueOption{
					Header: &envoycore.HeaderValue{
						Key:   ":authority",
						Value: spec.Hostname,
					},
					Append: &types.BoolValue{Value: false},
				})
			}
		}
	}
	return headers, knativeroute
}

func (p *Plugin) ProcessRoute(pluginParams *plugins.RoutePluginParams, in *v1.Route, out *envoyroute.Route) error {

	// if we have one knative upstream, the
	if route, ok := out.Action.(*envoyroute.Route_Route); ok {
		route := route.Route
		var knativeroute bool
		switch clusters := route.ClusterSpecifier.(type) {
		case *envoyroute.RouteAction_Cluster:
			// clusters is actually one cluster
			cluster := clusters
			route.RequestHeadersToAdd, knativeroute = maybeAppendHeader(pluginParams, cluster.Cluster, route.RequestHeadersToAdd)
		case *envoyroute.RouteAction_WeightedClusters:
			for _, cluster := range clusters.WeightedClusters.Clusters {
				cluster.RequestHeadersToAdd, knativeroute = maybeAppendHeader(pluginParams, cluster.Name, cluster.RequestHeadersToAdd)
			}
		}

		// knative needs longer timeouts
		// see here: https://github.com/knative/serving/blob/90ead22dd4842524bcb992588f19511d79a44543/pkg/controller/route/resources/virtual_service.go#L51
		if knativeroute {
			d := 60 * time.Second
			route.Timeout = &d
		}
	}

	return nil
}

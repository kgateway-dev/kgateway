package discovery

import (
	"context"
	"strings"
	"time"

	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	httpv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
	"github.com/solo-io/gloo/v2/pkg/translator/utils"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	corev1 "k8s.io/api/core/v1"
)

const (
	ClusterConnectionTimeout = time.Second * 5
)

type ServiceConverter struct {
}

func (sc *ServiceConverter) ClustersForService(ctx context.Context, svc *corev1.Service) []*clusterv3.Cluster {
	var clusters []*clusterv3.Cluster
	for _, port := range svc.Spec.Ports {
		clusters = append(clusters, sc.translateCluster(ctx, svc, port))
	}
	return clusters
}

func (sc *ServiceConverter) translateCluster(ctx context.Context, svc *corev1.Service, port corev1.ServicePort) *clusterv3.Cluster {

	out := &clusterv3.Cluster{
		Name:                          utils.ClusterName(svc.Namespace, svc.Name, port.Port),
		Metadata:                      new(corev3.Metadata),
		TypedExtensionProtocolOptions: map[string]*anypb.Any{},
		// this field can be overridden by plugins
		ConnectTimeout: durationpb.New(ClusterConnectionTimeout),
	}

	out.ClusterDiscoveryType = &clusterv3.Cluster_Type{
		Type: clusterv3.Cluster_EDS,
	}
	out.EdsClusterConfig = &clusterv3.Cluster_EdsClusterConfig{
		EdsConfig: &corev3.ConfigSource{
			ResourceApiVersion: corev3.ApiVersion_V3,
			ConfigSourceSpecifier: &corev3.ConfigSource_Ads{
				Ads: &corev3.AggregatedConfigSource{},
			},
		},
	}

	if opts := getHttp2options(port); opts != nil {
		out.GetTypedExtensionProtocolOptions()["envoy.extensions.upstreams.http.v3.HttpProtocolOptions"] = utils.ToAny(opts)
	}

	return out
}

func getHttp2options(port corev1.ServicePort) *httpv3.HttpProtocolOptions {
	useH2 := false
	if port.AppProtocol != nil {
		proto := strings.ToLower(*port.AppProtocol)
		useH2 = useH2 || strings.Contains(proto, "h2") ||
			(proto == "kubernetes.io/h2c") ||
			strings.Contains(proto, "http2") ||
			strings.Contains(proto, "grpc")
	}
	if useH2 {
		httpOpts := &httpv3.HttpProtocolOptions{
			UpstreamProtocolOptions: &httpv3.HttpProtocolOptions_ExplicitHttpConfig_{
				ExplicitHttpConfig: &httpv3.HttpProtocolOptions_ExplicitHttpConfig{
					ProtocolConfig: &httpv3.HttpProtocolOptions_ExplicitHttpConfig_Http2ProtocolOptions{
						Http2ProtocolOptions: &corev3.Http2ProtocolOptions{},
					},
				},
			},
		}
		return httpOpts
	}
	return nil
}

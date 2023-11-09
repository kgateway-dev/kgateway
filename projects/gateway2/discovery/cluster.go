package discovery

import (
	"context"
	"crypto/md5"
	"fmt"
	"strings"

	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	httpv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/utils/kubeutils"
	"google.golang.org/protobuf/types/known/durationpb"
	corev1 "k8s.io/api/core/v1"
)

type ServiceConverter struct {
}

func (sc *ServiceConverter) ClustersForService(ctx context.Context, svc *corev1.Service) []*clusterv3.Cluster {
	var clusters []*clusterv3.Cluster
	for _, port := range svc.Spec.Ports {
		clusters = append(clusters, sc.CreateCluster(ctx, svc, port))
	}
	return clusters
}

func (sc *ServiceConverter) CreateCluster(ctx context.Context, svc *corev1.Service, port corev1.ServicePort) *clusterv3.Cluster {

	out := &clusterv3.Cluster{
		Name:     ClusterName(svc.Namespace, svc.Name, port.Port),
		Metadata: new(corev3.Metadata),
		// CircuitBreakers:  getCircuitBreakers(upstream.GetCircuitBreakers(), circuitBreakers),
		// LbSubsetConfig:   createLbConfig(upstream),
		// HealthChecks:     hcConfig,
		// OutlierDetection: detectCfg,
		//defaults to Cluster_USE_CONFIGURED_PROTOCOL
		ProtocolSelection: clusterv3.Cluster_ClusterProtocolSelection(upstream.GetProtocolSelection()),
		// this field can be overridden by plugins
		ConnectTimeout:            durationpb.New(ClusterConnectionTimeout),
		Http2ProtocolOptions:      getHttp2options(upstream),
		IgnoreHealthOnHostRemoval: upstream.GetIgnoreHealthOnHostRemoval().GetValue(),
		RespectDnsTtl:             upstream.GetRespectDnsTtl().GetValue(),
		DnsRefreshRate:            getDnsRefreshRate(upstream, reports),
	}

	meta := svc.ObjectMeta
	coremeta := kubeutils.FromKubeMeta(meta, false)
	coremeta.ResourceVersion = ""
	coremeta.Name = UpstreamName(meta.Namespace, meta.Name, port.Port)
	labels := coremeta.GetLabels()
	coremeta.Labels = make(map[string]string)

	us := &v1.Upstream{
		Metadata: coremeta,
		UpstreamType: &v1.Upstream_Kube{
			Kube: &kubeplugin.UpstreamSpec{
				ServiceName:      meta.Name,
				ServiceNamespace: meta.Namespace,
				ServicePort:      uint32(port.Port),
				Selector:         svc.Spec.Selector,
			},
		},
		DiscoveryMetadata: &v1.DiscoveryMetadata{
			Labels: labels,
		},
	}

	for _, sc := range uc.serviceConverters {
		if err := sc.ConvertService(ctx, svc, port, us); err != nil {
			contextutils.LoggerFrom(ctx).Errorf("error: failed to process service options with err %v", err)
		}
	}

	return us
}

func ClusterName(serviceNamespace, serviceName string, servicePort int32) string {
	return SanitizeNameV2(fmt.Sprintf("%s-%s-%v", serviceNamespace, serviceName, servicePort))
}

func SanitizeNameV2(name string) string {
	name = strings.Replace(name, "*", "-", -1)
	name = strings.Replace(name, "/", "-", -1)
	name = strings.Replace(name, ".", "-", -1)
	name = strings.Replace(name, "[", "", -1)
	name = strings.Replace(name, "]", "", -1)
	name = strings.Replace(name, ":", "-", -1)
	name = strings.Replace(name, "_", "-", -1)
	name = strings.Replace(name, " ", "-", -1)
	name = strings.Replace(name, "\n", "", -1)
	name = strings.Replace(name, "\"", "", -1)
	name = strings.Replace(name, "'", "", -1)
	if len(name) > 63 {
		hash := md5.Sum([]byte(name))
		name = fmt.Sprintf("%s-%x", name[:31], hash)
		name = name[:63]
	}
	name = strings.Replace(name, ".", "-", -1)
	name = strings.ToLower(name)
	return name
}
func getHttp2options(port corev1.ServicePort) *corev3.Http2ProtocolOptions {
	useH2 := false
	if port.AppProtocol != nil {
		proto := strings.ToLower(*port.AppProtocol)
		useH2 = useH2 || strings.Contains(proto, "h2") ||
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

		// put in a map

		return httpOpts

	}
	return nil
}

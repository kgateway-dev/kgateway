package destrule

import (
	"context"
	"fmt"
	"hash/fnv"

	"google.golang.org/protobuf/types/known/wrapperspb"
	"k8s.io/apimachinery/pkg/runtime/schema"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_type_v3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"istio.io/api/networking/v1alpha3"
	"istio.io/istio/pkg/config/schema/gvr"
	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/endpoints"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
)

const (
	ExtensionName = "Destrule"
)

func NewPlugin(ctx context.Context, commoncol *collections.CommonCollections) sdk.Plugin {
	if !commoncol.Settings.EnableIstioIntegration {
		// TODO: should this be a standalone flag specific to DR?
		// don't add support for destination rules if istio integration is not enabled
		return sdk.Plugin{}
	}

	gk := schema.GroupKind{
		Group: gvr.DestinationRule.Group,
		Kind:  "DestinationRule",
	}
	d := &destrulePlugin{
		destinationRulesIndex: NewDestRuleIndex(commoncol.Client, &commoncol.KrtOpts),
	}
	return sdk.Plugin{
		ContributesPolicies: map[schema.GroupKind]sdk.PolicyPlugin{
			gk: {
				Name:                      "destrule",
				PerClientProcessBackend:   d.processBackend,
				PerClientProcessEndpoints: d.processEndpoints,
			},
		},
	}
}

type destrulePlugin struct {
	destinationRulesIndex DestinationRuleIndex
}

// processEndpoints tries to find a destination rule
// for the backend and if it does, it updates the PriorityInfo on `out`.
func (d *destrulePlugin) processEndpoints(
	kctx krt.HandlerContext,
	ctx context.Context,
	ucc ir.UniqlyConnectedClient,
	out *endpoints.EndpointsInputs,
) uint64 {
	destrule := d.destinationRulesIndex.FetchDestRulesFor(kctx, ucc.Namespace, out.EndpointsForBackend.Hostname, ucc.Labels)
	if destrule == nil {
		return 0
	}

	trafficPolicy := getTrafficPolicy(destrule, out.EndpointsForBackend.Port)
	localityLb := getLocalityLbSetting(trafficPolicy)
	if localityLb == nil {
		return 0
	}

	out.PriorityInfo = getPriorityInfoFromDestrule(localityLb)
	hasher := fnv.New64()
	hasher.Write([]byte(destrule.UID))
	hasher.Write([]byte(fmt.Sprintf("%v", destrule.Generation)))
	return hasher.Sum64()
}

func (d *destrulePlugin) processBackend(kctx krt.HandlerContext, ctx context.Context, ucc ir.UniqlyConnectedClient, in ir.BackendObjectIR, outCluster *envoyclusterv3.Cluster) {
	destrule := d.destinationRulesIndex.FetchDestRulesFor(kctx, ucc.Namespace, in.CanonicalHostname, ucc.Labels)
	if destrule != nil {
		trafficPolicy := getTrafficPolicy(destrule, uint32(in.Port)) //nolint:gosec // G115: BackendObjectIR.Port is int32 representing a port number, always in valid range
		if outlier := trafficPolicy.GetOutlierDetection(); outlier != nil {
			if getLocalityLbSetting(trafficPolicy) != nil {
				if outCluster.GetCommonLbConfig() == nil {
					outCluster.CommonLbConfig = &envoyclusterv3.Cluster_CommonLbConfig{}
				}
				outCluster.GetCommonLbConfig().LocalityConfigSpecifier = &envoyclusterv3.Cluster_CommonLbConfig_LocalityWeightedLbConfig_{
					LocalityWeightedLbConfig: &envoyclusterv3.Cluster_CommonLbConfig_LocalityWeightedLbConfig{},
				}
			}

			out := &envoyclusterv3.OutlierDetection{
				Consecutive_5Xx:  outlier.GetConsecutive_5XxErrors(),
				Interval:         outlier.GetInterval(),
				BaseEjectionTime: outlier.GetBaseEjectionTime(),
			}
			if e := outlier.GetConsecutiveGatewayErrors(); e != nil {
				v := e.GetValue()
				out.ConsecutiveGatewayFailure = &wrapperspb.UInt32Value{Value: v}
				if v > 0 {
					v = 100
				}
				out.EnforcingConsecutiveGatewayFailure = &wrapperspb.UInt32Value{Value: v}
			}
			if outlier.GetMaxEjectionPercent() > 0 {
				out.MaxEjectionPercent = &wrapperspb.UInt32Value{Value: uint32(outlier.GetMaxEjectionPercent())} //nolint:gosec // G115: MaxEjectionPercent is a percentage value (0-100), safe for uint32
			}
			if outlier.GetSplitExternalLocalOriginErrors() {
				out.SplitExternalLocalOriginErrors = true
				if outlier.GetConsecutiveLocalOriginFailures().GetValue() > 0 {
					out.ConsecutiveLocalOriginFailure = &wrapperspb.UInt32Value{Value: outlier.GetConsecutiveLocalOriginFailures().Value}
					out.EnforcingConsecutiveLocalOriginFailure = &wrapperspb.UInt32Value{Value: 100}
				}
				// SuccessRate based outlier detection should be disabled.
				out.EnforcingLocalOriginSuccessRate = &wrapperspb.UInt32Value{Value: 0}
			}
			minHealthPercent := outlier.GetMinHealthPercent()
			if minHealthPercent >= 0 {
				if outCluster.GetCommonLbConfig() == nil {
					outCluster.CommonLbConfig = &envoyclusterv3.Cluster_CommonLbConfig{}
				}
				outCluster.GetCommonLbConfig().HealthyPanicThreshold = &envoy_type_v3.Percent{Value: float64(minHealthPercent)}
			}

			outCluster.OutlierDetection = out

			// Translate TCP keepalive settings
			if tcpSettings := trafficPolicy.GetConnectionPool().GetTcp(); tcpSettings != nil {
				if tcpKeepalive := tcpSettings.GetTcpKeepalive(); tcpKeepalive != nil {
					if outCluster.GetUpstreamConnectionOptions() == nil {
						outCluster.UpstreamConnectionOptions = &envoyclusterv3.UpstreamConnectionOptions{}
					}
					if outCluster.GetUpstreamConnectionOptions().GetTcpKeepalive() == nil {
						outCluster.GetUpstreamConnectionOptions().TcpKeepalive = &envoycorev3.TcpKeepalive{}
					}
					if tcpKeepalive.GetTime() != nil {
						outCluster.GetUpstreamConnectionOptions().GetTcpKeepalive().KeepaliveTime = &wrapperspb.UInt32Value{Value: uint32(tcpKeepalive.GetTime().GetSeconds())} //nolint:gosec // G115: TCP keepalive time in seconds, reasonable range for uint32
					}
					if tcpKeepalive.GetInterval() != nil {
						outCluster.GetUpstreamConnectionOptions().GetTcpKeepalive().KeepaliveInterval = &wrapperspb.UInt32Value{Value: uint32(tcpKeepalive.GetInterval().GetSeconds())} //nolint:gosec // G115: TCP keepalive interval in seconds, reasonable range for uint32
					}
					if tcpKeepalive.GetProbes() > 0 {
						outCluster.GetUpstreamConnectionOptions().GetTcpKeepalive().KeepaliveProbes = &wrapperspb.UInt32Value{Value: uint32(tcpKeepalive.GetProbes())} //nolint:gosec // G115: TCP keepalive probe count, reasonable range for uint32
					}
				}
			}
		}
	}
}

func getPriorityInfoFromDestrule(localityLb *v1alpha3.LocalityLoadBalancerSetting) *endpoints.PriorityInfo {
	return &endpoints.PriorityInfo{
		FailoverPriority: endpoints.NewPriorities(localityLb.GetFailoverPriority()),
		Failover:         localityLb.GetFailover(),
	}
}

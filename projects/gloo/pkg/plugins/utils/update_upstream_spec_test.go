package utils_test

import (
	"reflect"

	"github.com/gogo/protobuf/types"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/api/v2/cluster"
	envoycore_gloo "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/api/v2/core"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/utils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("UpdateUpstream", func() {

	It("should preseve config when updating upstreams", func() {
		desired := &gloov1.Upstream{}
		original := &gloov1.Upstream{
			SslConfig:          &gloov1.UpstreamSslConfig{Sni: "testsni"},
			CircuitBreakers:    &gloov1.CircuitBreakerConfig{MaxConnections: &types.UInt32Value{Value: 6}},
			LoadBalancerConfig: &gloov1.LoadBalancerConfig{HealthyPanicThreshold: &types.DoubleValue{Value: 7}},
			ConnectionConfig:   &gloov1.ConnectionConfig{MaxRequestsPerConnection: 8},
			HealthChecks:       []*envoycore_gloo.HealthCheck{{}},
			OutlierDetection:   &cluster.OutlierDetection{Consecutive_5Xx: &types.UInt32Value{Value: 9}},
			Failover:           &gloov1.Failover{PrioritizedLocalities: []*gloov1.Failover_PrioritizedLocality{{}}},
			UseHttp2:           &types.BoolValue{Value: true},
		}
		utils.UpdateUpstream(original, desired)
		Expect(desired.SslConfig).To(Equal(original.SslConfig))
		Expect(desired.CircuitBreakers).To(Equal(original.CircuitBreakers))
		Expect(desired.LoadBalancerConfig).To(Equal(original.LoadBalancerConfig))
		Expect(desired.ConnectionConfig).To(Equal(original.ConnectionConfig))
		Expect(desired.HealthChecks).To(Equal(original.HealthChecks))
		Expect(desired.OutlierDetection).To(Equal(original.OutlierDetection))
		Expect(desired.Failover).To(Equal(original.Failover))
		Expect(desired.UseHttp2).To(Equal(original.UseHttp2))
	})

	It("should update config when one is desired", func() {
		desiredSslConfig := &gloov1.UpstreamSslConfig{
			SslSecrets: &gloov1.UpstreamSslConfig_SecretRef{
				SecretRef: &core.ResourceRef{"hi", "there"},
			},
		}
		desiredCircuitBreaker := &gloov1.CircuitBreakerConfig{MaxConnections: &types.UInt32Value{Value: 6}}
		desiredLoadBalancer := &gloov1.LoadBalancerConfig{HealthyPanicThreshold: &types.DoubleValue{Value: 7}}
		desiredConnectionConfig := &gloov1.ConnectionConfig{MaxRequestsPerConnection: 8}
		desiredHealthChecks := []*envoycore_gloo.HealthCheck{{}}
		desiredOutlierDetection := &cluster.OutlierDetection{Consecutive_5Xx: &types.UInt32Value{Value: 9}}
		desiredFailover := &gloov1.Failover{PrioritizedLocalities: []*gloov1.Failover_PrioritizedLocality{{}}}
		desiredUseHttp2 := &types.BoolValue{Value: true}
		desired := &gloov1.Upstream{
			SslConfig:          desiredSslConfig,
			CircuitBreakers:    desiredCircuitBreaker,
			LoadBalancerConfig: desiredLoadBalancer,
			ConnectionConfig:   desiredConnectionConfig,
			HealthChecks:       desiredHealthChecks,
			OutlierDetection:   desiredOutlierDetection,
			Failover:           desiredFailover,
			UseHttp2:           desiredUseHttp2,
		}
		original := &gloov1.Upstream{
			SslConfig:          &gloov1.UpstreamSslConfig{Sni: "testsni"},
			CircuitBreakers:    &gloov1.CircuitBreakerConfig{MaxPendingRequests: &types.UInt32Value{Value: 6}},
			LoadBalancerConfig: &gloov1.LoadBalancerConfig{HealthyPanicThreshold: &types.DoubleValue{Value: 9}},
			ConnectionConfig:   &gloov1.ConnectionConfig{PerConnectionBufferLimitBytes: &types.UInt32Value{Value: 10}},
			HealthChecks:       []*envoycore_gloo.HealthCheck{{}, {}},
			OutlierDetection:   &cluster.OutlierDetection{ConsecutiveGatewayFailure: &types.UInt32Value{Value: 9}},
			Failover:           &gloov1.Failover{PrioritizedLocalities: []*gloov1.Failover_PrioritizedLocality{{}, {}}},
			UseHttp2:           &types.BoolValue{Value: false},
		}

		utils.UpdateUpstream(original, desired)
		Expect(desired.SslConfig).To(Equal(desiredSslConfig))
		Expect(desired.CircuitBreakers).To(Equal(desiredCircuitBreaker))
		Expect(desired.LoadBalancerConfig).To(Equal(desiredLoadBalancer))
		Expect(desired.ConnectionConfig).To(Equal(desiredConnectionConfig))
		Expect(desired.HealthChecks).To(Equal(desiredHealthChecks))
		Expect(desired.OutlierDetection).To(Equal(desiredOutlierDetection))
		Expect(desired.Failover).To(Equal(desiredFailover))
		Expect(desired.UseHttp2).To(Equal(desiredUseHttp2))
	})

	It("will fail if the upstream proto has a new top level field", func() {
		// This test is important as it checks whether the upstream struct/proto have a new top level field.
		// This should happen very rarely, and should be used as an indication that the `UpdateUpstream` function
		// most likely needs to change.
		Expect(reflect.TypeOf(gloov1.Upstream{}).NumField()).To(
			Equal(15),
			"wrong number of fields found",
		)
	})

})

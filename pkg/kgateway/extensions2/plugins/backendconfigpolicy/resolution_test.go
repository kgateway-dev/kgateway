package backendconfigpolicy

import (
	"context"
	"slices"
	"testing"
	"time"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoycommondnsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/clusters/common/dns/v3"
	envoyproxyprotocolv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/proxy_protocol/v3"
	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoywellknown "github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"google.golang.org/protobuf/types/known/durationpb"
	"k8s.io/apimachinery/pkg/runtime/schema"

	. "github.com/onsi/gomega"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

// fullIR sets every field of a BackendConfigPolicy.
func fullIR() *BackendConfigPolicyIR {
	family := envoycommondnsv3.DnsLookupFamily_V4_ONLY
	return &BackendConfigPolicyIR{
		ct:                            time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
		connectTimeout:                durationpb.New(time.Second),
		perConnectionBufferLimitBytes: new(uint32(1)),
		tcpKeepalive:                  &envoycorev3.TcpKeepalive{},
		commonHttpProtocolOptions:     &envoycorev3.HttpProtocolOptions{},
		http1ProtocolOptions:          &envoycorev3.Http1ProtocolOptions{},
		http2ProtocolOptions:          &envoycorev3.Http2ProtocolOptions{},
		tlsConfig:                     &envoytlsv3.UpstreamTlsContext{},
		loadBalancerConfig:            &LoadBalancerConfigIR{},
		healthCheck:                   &envoycorev3.HealthCheck{},
		outlierDetection:              &envoyclusterv3.OutlierDetection{},
		circuitBreakers:               &envoyclusterv3.CircuitBreakers{},
		dnsRefreshRate:                durationpb.New(time.Second),
		dnsJitter:                     durationpb.New(time.Second),
		respectDnsTtl:                 new(true),
		dnsLookupFamily:               &family,
		upstreamProxyProtocol:         &envoycorev3.ProxyProtocolConfig{},
		tunnel:                        &tunnelIR{},
		sessionPersistence:            &sessionPersistenceIR{},
	}
}

// TestFieldTableMatchesMerge pins the field table SetFields and ClearFields use to the field
// names merge.go records. A field merged but missing from the table would be lost silently:
// never reported when dropped, and never cleared when another kind supersedes it.
func TestFieldTableMatchesMerge(t *testing.T) {
	g := NewWithT(t)
	gk := wellknown.BackendConfigPolicyGVK.GroupKind()
	merged := mergePolicies([]ir.PolicyAtt{{
		GroupKind: gk,
		PolicyRef: &ir.AttachedPolicyRef{Group: gk.Group, Kind: gk.Kind, Namespace: "ns", Name: "full"},
		PolicyIr:  fullIR(),
	}})
	mergedFields := make([]string, 0, len(merged.MergeOrigins))
	for f := range merged.MergeOrigins {
		mergedFields = append(mergedFields, f)
	}

	g.Expect(setFields(fullIR())).To(ConsistOf(mergedFields), "SetFields must name exactly the fields merge.go merges")

	cleared := clearFields(fullIR(), setFields(fullIR())).(*BackendConfigPolicyIR)
	g.Expect(setFields(cleared)).To(BeEmpty(), "ClearFields must clear every field in the table")
}

// TestProxyProtocolWrapsAnotherKindsTLS checks the wrapper is applied after every
// ProcessBackend hook: it wraps a TLS socket another kind set, without either plugin
// knowing about the other.
func TestProxyProtocolWrapsAnotherKindsTLS(t *testing.T) {
	g := NewWithT(t)
	tlsAny, err := utils.MessageToAny(&envoytlsv3.UpstreamTlsContext{Sni: "from-other-kind.example.com"})
	g.Expect(err).NotTo(HaveOccurred())
	cluster := &envoyclusterv3.Cluster{TransportSocket: &envoycorev3.TransportSocket{
		Name:       envoywellknown.TransportSocketTls,
		ConfigType: &envoycorev3.TransportSocket_TypedConfig{TypedConfig: tlsAny},
	}}
	gk := wellknown.BackendConfigPolicyGVK.GroupKind()
	backend := ir.BackendObjectIR{AttachedPolicies: ir.AttachedPolicies{Policies: map[schema.GroupKind][]ir.PolicyAtt{
		gk: {{GroupKind: gk, PolicyIr: &BackendConfigPolicyIR{upstreamProxyProtocol: &envoycorev3.ProxyProtocolConfig{}}}},
	}}}

	processBaseCluster(nil, context.Background(), backend, cluster)

	g.Expect(cluster.GetTransportSocket().GetName()).To(Equal(wellknown.TransportSocketUpstreamProxyProtocol))
	pp := &envoyproxyprotocolv3.ProxyProtocolUpstreamTransport{}
	g.Expect(cluster.GetTransportSocket().GetTypedConfig().UnmarshalTo(pp)).To(Succeed())
	inner := &envoytlsv3.UpstreamTlsContext{}
	g.Expect(pp.GetTransportSocket().GetTypedConfig().UnmarshalTo(inner)).To(Succeed())
	g.Expect(inner.GetSni()).To(Equal("from-other-kind.example.com"), "the wrapper should keep the other kind's TLS inside")
}

func TestClearFieldsLeavesInputUntouched(t *testing.T) {
	g := NewWithT(t)
	in := fullIR()
	out := clearFields(in, []string{TLSField}).(*BackendConfigPolicyIR)

	g.Expect(out.tlsConfig).To(BeNil(), "tls should be cleared on the copy")
	g.Expect(in.tlsConfig).NotTo(BeNil(), "the input policy is shared and must not change")
	g.Expect(slices.Contains(setFields(out), "connectTimeout")).To(BeTrue(), "other fields should be kept")
}

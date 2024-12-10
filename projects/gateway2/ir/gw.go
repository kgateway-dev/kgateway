package ir

import (
	"context"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"google.golang.org/protobuf/types/known/anypb"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// types here are not in krt collections, so no need for equals

type UpstreamInit struct {
	InitUpstream func(ctx context.Context, in Upstream, out *envoy_config_cluster_v3.Cluster)
}

type PolicyTargetRef struct {
	Group       string
	Kind        string
	Name        string
	SectionName string
}

type PolicyAtt struct {
	// original object. ideally with structural errors removed.
	// Opaque to us other than metadata.
	PolicyIr PolicyIR

	PolicyTargetRef PolicyTargetRef
}

func (c PolicyAtt) Obj() PolicyIR {
	return c.PolicyIr
}

func (c PolicyAtt) TargetRef() PolicyTargetRef {
	return c.PolicyTargetRef
}

type AttachedPolicies struct {
	Policies map[schema.GroupKind][]PolicyAtt
}

type Backend struct {
	ClusterName string
	Weight      uint32

	// upstream could be nil if not found or no ref grant
	Upstream *Upstream
	// if nil, error might say why
	Err error
}

type HttpRouteRuleCommonIR struct {
	Parent           *HttpRouteIR
	SourceRule       *gwv1.HTTPRouteRule
	ExtensionRefs    AttachedPolicies
	AttachedPolicies AttachedPolicies
}

type HttpBackendOrDelegate struct {
	Backend  *Backend
	Delegate *ObjectSource
	AttachedPolicies
}

type HttpBackend struct {
	Backend Backend
	AttachedPolicies
}

type HttpRouteRuleIR struct {
	HttpRouteRuleCommonIR
	Backends []HttpBackendOrDelegate
	Matches  []gwv1.HTTPRouteMatch
	Name     string
}

type HttpRouteRuleMatchIR struct {
	HttpRouteRuleCommonIR
	// if there's an error, the listener where to report it.
	ParentRef  gwv1.ParentReference
	Backends   []HttpBackend
	Match      gwv1.HTTPRouteMatch
	MatchIndex int
	Name       string
}

type ListenerIR struct {
	Name             string
	BindAddress      string
	BindPort         uint32
	AttachedPolicies AttachedPolicies

	HttpFilterChain []HttpFilterChainIR
	TcpFilterChain  []TcpIR
}

type VirtualHost struct {
	Name string
	// technically envoy supports multiple domains per vhost, but gwapi translation doesnt
	// if this changes, we can edit the IR; in the mean time keeping it simple.
	Hostname string
	Rules    []HttpRouteRuleMatchIR
}

type FilterChainMatch struct {
	SniDomains []string
}
type TlsBundle struct {
	CA            []byte
	PrivateKey    []byte
	CertChain     []byte
	AlpnProtocols []string
}

type FilterChainCommon struct {
	Matcher              FilterChainMatch
	FilterChainName      string
	CustomNetworkFilters []CustomEnvoyFilter
	TLS                  *TlsBundle
}
type CustomEnvoyFilter struct {
	// Determines filter ordering.
	FilterStage plugins.FilterStage[plugins.WellKnownFilterStage]
	// The name of the filter configuration.
	Name string
	// Filter specific configuration.
	Config *anypb.Any
}

type HttpFilterChainIR struct {
	FilterChainCommon
	Vhosts                  []*VirtualHost
	AttachedPolicies        AttachedPolicies
	AttachedNetworkPolicies AttachedPolicies
}

type TcpIR struct {
	FilterChainCommon
	BackendRefs []Backend
}

// this is 1:1 with envoy deployments
// not in a collection so doesn't need a krt interfaces.
type GatewayIR struct {
	Listeners    []ListenerIR
	SourceObject *gwv1.Gateway

	AttachedPolicies     AttachedPolicies
	AttachedHttpPolicies AttachedPolicies
}

type GatewayWithPoliciesIR struct {
	SourceObject *gwv1.Gateway

	AttachedPolicies     AttachedPolicies
	AttachedHttpPolicies AttachedPolicies
}

type Extension struct {
	ContributedUpstreams map[schema.GroupKind]krt.Collection[Upstream]
}

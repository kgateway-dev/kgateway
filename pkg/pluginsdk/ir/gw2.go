package ir

import (
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"

	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/plugins"
)

// This is the IR that is used in the translation to XDS. it is self contained and no IO/krt is
// needed to process it to xDS.

// As types here are not in krt collections, so no need for equals and resource name.
// Another advantage - because this doesn't appear in any snapshot, we don't need to redact secrets.

type HttpBackend struct {
	Backend BackendRefIR
	AttachedPolicies
}

type HttpRouteRuleMatchIR struct {
	ExtensionRefs    AttachedPolicies
	AttachedPolicies AttachedPolicies
	Parent           *HttpRouteIR
	// DelegatingParent is a pointer to the parent HttpRouteRuleMatchIR that delegated to this one
	DelegatingParent *HttpRouteRuleMatchIR
	// Delegates is a boolean indicating if this HttpRouteRuleMatchIR delegates to other routes
	Delegates bool

	// if there's an error, the gw-api listener to report it in.
	ListenerParentRef gwv1.ParentReference
	// the parent ref the led here (may be delegated httproute or listner)
	ParentRef  gwv1.ParentReference
	Backends   []HttpBackend
	Match      gwv1.HTTPRouteMatch
	MatchIndex int
	Name       string
}

type ListenerIR struct {
	Name        string
	BindAddress string
	BindPort    uint32

	HttpFilterChain []HttpFilterChainIR
	TcpFilterChain  []TcpIR

	PolicyAncestorRef gwv1.ParentReference

	AttachedPolicies AttachedPolicies
}

type VirtualHost struct {
	Name string
	// technically envoy supports multiple domains per vhost, but gwapi translation doesnt
	// if this changes, we can edit the IR; in the mean time keeping it simple.
	Hostname         string
	Rules            []HttpRouteRuleMatchIR
	AttachedPolicies AttachedPolicies
}

type FilterChainMatch struct {
	SniDomains      []string
	PrefixRanges    []*v3.CidrRange
	DestinationPort *wrapperspb.UInt32Value
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
	NetworkFilters       []*anypb.Any
	TLS                  *TlsBundle
}

type CustomEnvoyFilter struct {
	// Determines filter ordering.
	FilterStage plugins.HTTPOrNetworkFilterStage
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
	CustomHTTPFilters       []CustomEnvoyFilter
}

type TcpIR struct {
	FilterChainCommon
	BackendRefs []BackendRefIR
}

// this is 1:1 with envoy deployments
// not in a collection so doesn't need a krt interfaces.
type GatewayIR struct {
	Listeners    []ListenerIR
	SourceObject *Gateway

	AttachedPolicies     AttachedPolicies
	AttachedHttpPolicies AttachedPolicies
}

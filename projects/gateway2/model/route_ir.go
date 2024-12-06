package model

import (
	"fmt"

	"github.com/solo-io/gloo/projects/controller/pkg/plugins"
	"google.golang.org/protobuf/types/known/anypb"
	"istio.io/istio/pkg/kube/krt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type AttachmentPoints int

const (
	HttpAttachmentPoint           AttachmentPoints = iota
	HttpBackendRefAttachmentPoint AttachmentPoints = iota
	ListenerAttachmentPoint
)

type PolicyTargetRef struct {
	Group       string
	Kind        string
	Name        string
	SectionName string
}

type PolicyWrapper struct {
	GK schema.GroupKind
	// Errors processing it for status.
	// note: these errors are based on policy itself, regardless of whether it's attached to a resource.
	// TODO: change for conditions
	Errors []error

	// original object. ideally with structural errors removed.
	// Opaque to us other than metadata.
	Policy metav1.Object

	TargetRefs []PolicyTargetRef
}

func (c PolicyWrapper) ResourceName() string {
	return fmt.Sprintf("%s/%s/%s/%s", c.GK.Group, c.GK.Kind, c.Policy.GetNamespace(), c.Policy.GetName())
}

func (c PolicyWrapper) Obj() metav1.Object {
	return c.Policy
}

func (c PolicyWrapper) Equals(in PolicyWrapper) bool {
	var versionEquals bool
	if c.Policy.GetGeneration() != 0 && in.Policy.GetGeneration() != 0 {
		versionEquals = c.Policy.GetGeneration() == in.Policy.GetGeneration()
	} else {
		versionEquals = c.Policy.GetResourceVersion() == in.Policy.GetResourceVersion()
	}

	return versionEquals && c.Policy.GetUID() == in.Policy.GetUID()
}

type Policy interface {
	Obj() metav1.Object
}

type Policies []Policy

//type AttachedPolicies map[string]Policies

type NetworkPolicy Policy
type HttpPolicy Policy
type HttpBackendPolicy Policy
type ListenerPolicy Policy

type AttachedPolicies[P Policy] struct {
	Policies map[schema.GroupKind][]P
}

type Backend struct {
	ClusterName string
	Weight      uint32

	// upstream could be nil if not found or no ref grant
	Upstream Upstream
}

/*
(aws) upstream plugin:

	ContributesPolicies map[GroupKind:"kgw/Parameters"]struct {
		AttachmentPoints          []{BackendAttachmentPoint}
		NewGatewayTranslationPass func(ctx context.Context, tctx GwTranslationCtx) ProxyTranslationPass{

		ProcessBackend: func(ctx context.Context, Backend, RefPolicy) ProxyTranslationPass{
			// check backend upstream to be aws
			// check ref policy to be aws
		}
		Policies                  krt.Collection[model.Policy]
		PoliciesFetch(name, namespace) Policy {return RefPolicy{...}}
	}

	ContributesUpstreams map[GroupKind:"kgw/Upstream"]struct {
		ProcessUpstream: func(ctx context.Context, in model.Upstream, out *envoy_config_cluster_v3.Cluster){
			ourUs, ok := in.Obj.(*kgw.Upstream)
			if !ok {
				// log - should never happen
				return
			}
			if ourUs.aws != nil {
				do stuff and update the cluster
			}
		}
		Upstreams       krt.Collection[model.Upstream]
		Endpoints       []krt.Collection[krtcollections.EndpointsForUpstream]
	}
	ContributesGwClasses map[string]translator.K8sGwTranslator
*/
type HttpBackend struct {
	Backend
	AttachedPolicies[HttpBackendPolicy]
}

type HttpRouteIR struct {
	SourceObject     client.Object
	ParentRefs       []gwv1.ParentReference
	Hostnames        []string
	AttachedPolicies AttachedPolicies[HttpPolicy]
	Rules            []HttpRouteRuleIR
}

type HttpRouteRuleIR struct {
	gwv1.HTTPRouteRule
	Parent           HttpRouteIR
	ExtensionRefs    AttachedPolicies[HttpPolicy]
	AttachedPolicies AttachedPolicies[HttpPolicy]

	Backends []HttpBackend
}

type ListenerIR struct {
	Name             string
	BindAddress      string
	BindPort         uint32
	AttachedPolicies AttachedPolicies[HttpPolicy]

	HttpFilterChain []HttpFilterChainIR
	TcpFilterChain  []TcpIR
}

type VirtualHost struct {
	Name      string
	Hostnames []string
	Rules     []HttpRouteRuleIR
}

type FilterChainMatch struct {
	SniDomains []string
}
type TlsBundle struct {
	//	CA            []byte
	PrivateKey    []byte
	CertChain     []byte
	AlpnProtocols []string
}

type FilterChainCommon struct {
	Matcher              FilterChainMatch
	FilterChainName      string
	ParentRef            gwv1.ParentReference
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
	ParentRef               gwv1.ParentReference
	AttachedPolicies        AttachedPolicies[HttpPolicy]
	AttachedNetworkPolicies AttachedPolicies[NetworkPolicy]
}

type TcpIR struct {
	FilterChainCommon
	BackendRefs []Backend
}

// this is 1:1 with envoy deployments
type GatewayIR struct {
	Listeners    []ListenerIR
	SourceObject *gwv1.Gateway

	AttachedPolicies     AttachedPolicies[ListenerPolicy]
	AttachedHttpPolicies AttachedPolicies[HttpPolicy]
}

type Extension struct {
	ContributedUpstreams map[schema.GroupKind]krt.Collection[Upstream]
}

package options

import (
	"context"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/secret/inputsecret"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

type Options struct {
	Metadata core.Metadata
	Top      Top
	Install  Install
	Gateway  Gateway
	Upgrade  Upgrade
	Create   Create
	Delete   Delete
	Get      Get
	Add      Add
	Remove   Remove
}

type Top struct {
	Interactive bool
	File        string
	Output      string
	Ctx         context.Context
}

type Install struct {
	DockerAuth struct {
		Email    string
		Username string
		Password string
		Server   string
	}
}

const (
	ClusterProvider_GKE      = "GKE"
	ClusterProvider_Minikube = "Minikube"
)

type Gateway struct {
	ClusterProvider string
	Proxy           string
	FollowLogs      bool
	DebugLogs       bool
}

type Upgrade struct {
	ReleaseTag   string
	DownloadPath string
}

type Get struct {
	Selector InputMapStringString
}

type Delete struct {
	Selector InputMapStringString
	All      bool
}

type Create struct {
	VirtualService InputVirtualService
	InputUpstream  InputUpstream
	InputSecret    inputsecret.Secret
}

type RouteMatchers struct {
	PathPrefix    string
	PathExact     string
	PathRegex     string
	Methods       []string
	HeaderMatcher InputMapStringString
}

type Add struct {
	Route InputRoute
}

type InputRoute struct {
	InsertIndex uint32
	Matcher     RouteMatchers
	Destination Destination
	// TODO: multi destination
	//Destinations []Destination
	Plugins RoutePlugins
}

type Destination struct {
	Upstream        core.ResourceRef
	DestinationSpec DestinationSpec
}

type RoutePlugins struct {
	PrefixRewrite PrefixRewrite
}

type PrefixRewrite struct {
	Value *string
}

func (p *PrefixRewrite) String() string {
	if p == nil || p.Value == nil {
		return "<nil>"
	}
	return *p.Value
}

func (p *PrefixRewrite) Set(s string) error {
	if p == nil {
		p = &PrefixRewrite{}
	}
	p.Value = &s
	return nil
}

func (p *PrefixRewrite) Type() string {
	return "string"
}

type DestinationSpec struct {
	Aws  AwsDestinationSpec
	Rest RestDestinationSpec
}

type AwsDestinationSpec struct {
	LogicalName            string
	ResponseTransformation bool
}

type RestDestinationSpec struct {
	FunctionName string
	Parameters   InputMapStringString
}

type Remove struct {
}

type InputVirtualService struct {
	Domains []string
}

const (
	UpstreamType_Aws    = "aws"
	UpstreamType_Azure  = "azure"
	UpstreamType_Consul = "consul"
	UpstreamType_Kube   = "kube"
	UpstreamType_Static = "static"
)

var UpstreamTypes = []string{
	UpstreamType_Aws,
	UpstreamType_Azure,
	UpstreamType_Consul,
	UpstreamType_Kube,
	UpstreamType_Static,
}

type InputUpstream struct {
	UpstreamType string
	Aws          InputAwsSpec
	Azure        InputAzureSpec
	Consul       InputConsulSpec
	Kube         InputKubeSpec
	Static       InputStaticSpec

	// An optional Service Spec describing the service listening on this upstream
	ServiceSpec InputServiceSpec
}

type InputAwsSpec struct {
	Region string
	Secret core.ResourceRef
}

type InputAzureSpec struct {
	FunctionAppName string
	Secret          core.ResourceRef
}

type InputConsulSpec struct {
	// The name of the Consul Service
	ServiceName string `protobuf:"bytes,1,opt,name=service_name,json=serviceName,proto3" json:"service_name,omitempty"`
	// The list of service tags Gloo should search for on a service instance
	// before deciding whether or not to include the instance as part of this
	// upstream
	ServiceTags []string `protobuf:"bytes,2,rep,name=service_tags,json=serviceTags" json:"service_tags,omitempty"`
}

type InputKubeSpec struct {
	// The name of the Kubernetes Service
	ServiceName string `protobuf:"bytes,1,opt,name=service_name,json=serviceName,proto3" json:"service_name,omitempty"`
	// The namespace where the Service lives
	ServiceNamespace string `protobuf:"bytes,2,opt,name=service_namespace,json=serviceNamespace,proto3" json:"service_namespace,omitempty"`
	// The port where the Service is listening.
	ServicePort uint32 `protobuf:"varint,3,opt,name=service_port,json=servicePort,proto3" json:"service_port,omitempty"`
	// Allows finer-grained filtering of pods for the Upstream. Gloo will select pods based on their labels if
	// any are provided here.
	// (see [Kubernetes labels and selectors](https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/)
	Selector InputMapStringString
}

type Selector struct {
	// Allows finer-grained filtering of pods for the Upstream. Gloo will select pods based on their labels if
	// any are provided here.
	// (see [Kubernetes labels and selectors](https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/)
	SelectorKeyValuePairs []string `protobuf:"bytes,4,rep,name=selector" json:"selector,omitempty" protobuf_key:"bytes,1,opt,name=key,proto3" protobuf_val:"bytes,2,opt,name=value,proto3"`
}

type InputStaticSpec struct {
	Hosts []string
	// Attempt to use outbound TLS
	// Gloo will automatically set this to true for port 443
	UseTls bool
}

const (
	ServiceType_Rest = "rest"
	ServiceType_Grpc = "grpc"
)

type InputServiceSpec struct {
	ServiceType          string
	InputRestServiceSpec InputRestServiceSpec
	InputGrpcServiceSpec InputGrpcServiceSpec
}

type InputRestServiceSpec struct {
	SwaggerUrl       string
	SwaggerDocInline string
}

type InputGrpcServiceSpec struct {
	// inline from a file
	Descriptors []byte
}

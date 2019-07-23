package options

import (
	"context"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"

	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

type Options struct {
	Metadata  core.Metadata
	Top       Top
	Install   Install
	Uninstall Uninstall
	Proxy     Proxy
	Upgrade   Upgrade
	Create    Create
	Delete    Delete
	Get       Get
	Add       Add
	Remove    Remove
}

type Top struct {
	Interactive bool
	File        string
	Output      string
	Ctx         context.Context
	Verbose     bool // currently only used by install and uninstall, sends kubectlc command output to terminal
}

type Install struct {
	DryRun            bool
	Upgrade           bool
	Namespace         string
	HelmChartOverride string
	Knative           Knative
}

type Knative struct {
	InstallKnativeVersion    string `json:"version"`
	InstallKnative           bool   `json:"-"`
	SkipGlooInstall          bool   `json:"-"`
	InstallKnativeBuild      bool   `json:"build"`
	InstallKnativeMonitoring bool   `json:"monitoring"`
	InstallKnativeEventing   bool   `json:"eventing"`
}

type Uninstall struct {
	Namespace       string
	DeleteCrds      bool
	DeleteNamespace bool
	DeleteAll       bool
}

type Proxy struct {
	LocalCluster bool
	Name         string
	Port         string
	FollowLogs   bool
	DebugLogs    bool
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
}

type RouteMatchers struct {
	PathPrefix    string
	PathExact     string
	PathRegex     string
	Methods       []string
	HeaderMatcher InputMapStringString
}

type Add struct {
	Route     InputRoute
	DryRun    bool // print resource as a kubernetes style yaml and exit without writing to storage
	PrintYaml bool // print resource as basic (non-kubernetes) yaml and exit without writing to storage
}

type InputRoute struct {
	InsertIndex uint32
	Matcher     RouteMatchers
	Destination Destination
	// TODO: multi destination
	//Destinations []Destination
	UpstreamGroup core.ResourceRef
	Plugins       RoutePlugins
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

type InputUpstreamGroup struct {
    WeightedDestinations[] v1.WeightedDestination
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
	Route RemoveRoute
}

type RemoveRoute struct {
	RemoveIndex uint32
}

type InputVirtualService struct {
	Domains     []string
	DisplayName string
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
	ServiceName string
	// The list of service tags Gloo should search for on a service instance
	// before deciding whether or not to include the instance as part of this
	// upstream
	ServiceTags []string
}

type InputKubeSpec struct {
	// The name of the Kubernetes Service
	ServiceName string
	// The namespace where the Service lives
	ServiceNamespace string
	// The port exposed by the Kubernetes Service
	ServicePort uint32
	// Allows finer-grained filtering of pods for the Upstream. Gloo will select pods based on their labels if
	// any are provided here.
	// (see [Kubernetes labels and selectors](https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/)
	Selector InputMapStringString
}

type Selector struct {
	// Allows finer-grained filtering of pods for the Upstream. Gloo will select pods based on their labels if
	// any are provided here.
	// (see [Kubernetes labels and selectors](https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/)
	SelectorKeyValuePairs []string
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

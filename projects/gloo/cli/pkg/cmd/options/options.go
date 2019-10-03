package options

import (
	"context"
	"sort"

	extauth "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/plugins/extauth/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/plugins/ratelimit"

	"github.com/hashicorp/consul/api"
	vaultapi "github.com/hashicorp/vault/api"
	printTypes "github.com/solo-io/gloo/projects/gloo/cli/pkg/printers"
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
	Edit      Edit
	Route     Route
	Get       Get
	Add       Add
	Remove    Remove
}

type Top struct {
	Interactive bool
	File        string
	Output      printTypes.OutputType
	Ctx         context.Context
	Verbose     bool   // currently only used by install and uninstall, sends kubectl command output to terminal
	KubeConfig  string // file to use for kube config, if not standard one.
	Zip         bool
	ErrorsOnly  bool
}

type Install struct {
	DryRun            bool
	Upgrade           bool
	Namespace         string
	HelmChartOverride string
	HelmChartValues   string
	Knative           Knative
	LicenseKey        string
	WithUi            bool
}

type Knative struct {
	InstallKnativeVersion         string `json:"version"`
	InstallKnative                bool   `json:"-"`
	SkipGlooInstall               bool   `json:"-"`
	InstallKnativeBuild           bool   `json:"build"`
	InstallKnativeBuildVersion    string `json:"buildVersion"`
	InstallKnativeMonitoring      bool   `json:"monitoring"`
	InstallKnativeEventing        bool   `json:"eventing"`
	InstallKnativeEventingVersion string `json:"eventingVersion"`
}

type Uninstall struct {
	Namespace       string
	DeleteCrds      bool
	DeleteNamespace bool
	DeleteAll       bool
}

type Proxy struct {
	LocalCluster     bool
	LocalClusterName string
	Name             string
	Port             string
	FollowLogs       bool
	DebugLogs        bool
}

type Upgrade struct {
	ReleaseTag   string
	DownloadPath string
}

type Get struct {
	Selector InputMapStringString
	Consul   Consul // use consul as config backend
}

type Delete struct {
	Selector InputMapStringString
	All      bool
	Consul   Consul // use consul as config backend
}

type Edit struct {
	Consul Consul // use consul as config backend
}

type Route struct {
	Consul Consul // use consul as config backend
}

type Consul struct {
	UseConsul bool // enable consul config clients
	RootKey   string
	Client    func() (*api.Client, error)
}

type Vault struct {
	UseVault bool // enable vault secret clients
	RootKey  string
	Client   func() (*vaultapi.Client, error)
}

type Create struct {
	VirtualService     InputVirtualService
	InputUpstream      InputUpstream
	InputUpstreamGroup InputUpstreamGroup
	InputSecret        Secret
	DryRun             bool   // print resource as a kubernetes style yaml and exit without writing to storage
	Consul             Consul // use consul as config backend
	Vault              Vault  // use vault as secrets backend
}

type RouteMatchers struct {
	PathPrefix            string
	PathExact             string
	PathRegex             string
	Methods               []string
	HeaderMatcher         InputMapStringString
	QueryParameterMatcher InputMapStringString
}

type Add struct {
	Route  InputRoute
	DryRun bool   // print resource as a kubernetes style yaml and exit without writing to storage
	Consul Consul // use consul as config backend
}

type InputRoute struct {
	InsertIndex uint32
	Matcher     RouteMatchers
	Destination Destination
	// TODO: multi destination
	// Destinations []Destination
	UpstreamGroup   core.ResourceRef
	Plugins         RoutePlugins
	AddToRouteTable bool // add the route to a route table rather than a virtual service
}

type Destination struct {
	Upstream        core.ResourceRef
	Delegate        core.ResourceRef
	DestinationSpec DestinationSpec
}

type RoutePlugins struct {
	PrefixRewrite PrefixRewrite
}

type PrefixRewrite struct {
	Value *string
}

type InputUpstreamGroup struct {
	WeightedDestinations InputMapStringString
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
	Route  RemoveRoute
	Consul Consul // use consul as config backend
}

type RemoveRoute struct {
	RemoveIndex uint32
}

type InputVirtualService struct {
	Domains     []string
	DisplayName string
	RateLimit   RateLimit
	OIDCAuth    OIDCAuth
	ApiKeyAuth  ApiKeyAuth
	OpaAuth     OpaAuth
}

const (
	UpstreamType_Aws    = "aws"
	UpstreamType_AwsEc2 = "ec2"
	UpstreamType_Azure  = "azure"
	UpstreamType_Consul = "consul"
	UpstreamType_Kube   = "kube"
	UpstreamType_Static = "static"
)

var UpstreamTypes = []string{
	UpstreamType_Aws,
	UpstreamType_AwsEc2,
	UpstreamType_Azure,
	UpstreamType_Consul,
	UpstreamType_Kube,
	UpstreamType_Static,
}

type InputUpstream struct {
	UpstreamType string
	Aws          InputAwsSpec
	AwsEc2       InputAwsEc2Spec
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

type InputAwsEc2Spec struct {
	Region          string
	Secret          core.ResourceRef
	Role            string
	PublicIp        bool
	Port            uint32
	KeyFilters      []string
	KeyValueFilters InputMapStringString
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

type ExtraOptions struct {
	RateLimit  RateLimit
	OIDCAuth   OIDCAuth
	ApiKeyAuth ApiKeyAuth
	OpaAuth    OpaAuth
}

var RateLimit_TimeUnits = func() []string {
	var vals []string
	for _, name := range ratelimit.RateLimit_Unit_name {
		vals = append(vals, name)
	}
	sort.Strings(vals)
	return vals
}()

type RateLimit struct {
	Enable              bool
	TimeUnit            string
	RequestsPerTimeUnit uint32
}

type Dashboard struct {
}

type OIDCAuth struct {
	Enable bool

	// Include all options from the vhost extension
	extauth.OAuth
}

type ApiKeyAuth struct {
	Enable          bool
	Labels          []string
	SecretNamespace string
	SecretName      string
}

type OIDCSettings struct {
	ExtAtuhServerUpstreamRef core.ResourceRef
}

type OpaAuth struct {
	Enable bool

	Query   string
	Modules []string
}

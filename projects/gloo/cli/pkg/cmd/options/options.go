package options

import (
	"context"
	"sort"
	"strconv"
	"time"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options/contextoptions"
	printTypes "github.com/solo-io/gloo/projects/gloo/cli/pkg/printers"

	rltypes "github.com/solo-io/solo-apis/pkg/api/ratelimit.solo.io/v1alpha1"

	vaultapi "github.com/hashicorp/vault/api"
	"github.com/rotisserie/eris"
	extauth "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
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
	Istio     Istio
	Remove    Remove
	Cluster   Cluster
	Check     Check
	CheckCRD  CheckCRD
}
type Top struct {
	contextoptions.ContextAccessible
	CheckName          []string
	Output             printTypes.OutputType
	Ctx                context.Context
	Zip                bool
	PodSelector        string   // label selector for pod scanning
	ResourceNamespaces []string // namespaces in which to check custom resources
}

type HelmInstall struct {
	CreateNamespace         bool
	Namespace               string
	HelmChartOverride       string
	HelmChartValueFileNames []string
	HelmReleaseName         string
}

type Install struct {
	Gloo        HelmInstall
	Federation  HelmInstall
	Knative     Knative
	LicenseKey  string
	WithGlooFed bool
	DryRun      bool
	Version     string
}

type Knative struct {
	InstallKnativeVersion         string `json:"version"`
	InstallKnative                bool   `json:"-"`
	SkipGlooInstall               bool   `json:"-"`
	InstallKnativeMonitoring      bool   `json:"monitoring"`
	InstallKnativeEventing        bool   `json:"eventing"`
	InstallKnativeEventingVersion string `json:"eventingVersion"`
}

type HelmUninstall struct {
	Namespace       string
	HelmReleaseName string
	DeleteCrds      bool
	DeleteNamespace bool
	DeleteAll       bool
}

type Uninstall struct {
	GlooUninstall HelmUninstall
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
}

type Delete struct {
	Selector InputMapStringString
	All      bool
}

type Edit struct {
}

type Route struct {
}

type Vault struct {
	// enable vault secret clients
	UseVault bool

	// https://learn.hashicorp.com/tutorials/vault/getting-started-secrets-engines
	// PathPrefix tells Vault which secrets engine to which it should route traffic.
	PathPrefix string

	// Secrets are persisted using a resource client constructed in solo-kit
	// https://github.com/solo-io/solo-kit/blob/1d799ae290c2f516f01fc4ad20272d7d2d5db1e7/pkg/api/v1/clients/vault/resource_client.go#L311
	// The RootKey is used to configure the path for the particular Gloo installation
	// This ensures that you can run multiple instances of Gloo against the same Consul cluster
	RootKey string
	Client  func() (*vaultapi.Client, error)
}

type Create struct {
	VirtualService     InputVirtualService
	InputUpstream      InputUpstream
	InputUpstreamGroup InputUpstreamGroup
	InputSecret        Secret
	AuthConfig         InputAuthConfig
	DryRun             bool  // print resource as a kubernetes style yaml and exit without writing to storage
	Vault              Vault // use vault as secrets backend
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
	DryRun bool // print resource as a kubernetes style yaml and exit without writing to storage
}

type Istio struct {
	Upstream              string // upstream for which we are changing the istio mTLS settings
	IncludeUpstreams      bool   // whether or not to modify upstreams when uninstalling mTLS
	Namespace             string // namespace in which istio is installed
	IstioMetaMeshId       string // IstioMetaMeshId sets ISTIO_META_MESH_ID env var
	IstioMetaClusterId    string // IstioMetaClusterId sets ISTIO_META_CLUSTER_ID env var
	IstioDiscoveryAddress string // IstioDiscoveryAddress sets discoveryAddress field within PROXY_CONFIG env var
}

type InputRoute struct {
	InsertIndex uint32
	Matcher     RouteMatchers
	Destination Destination
	// TODO: multi destination
	// Destinations []Destination
	UpstreamGroup         core.ResourceRef
	Plugins               RoutePlugins
	AddToRouteTable       bool // add the route to a route table rather than a virtual service
	ClusterScopedVsClient bool
}

type Destination struct {
	Upstream        core.ResourceRef
	Delegate        Delegate
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
		return eris.New("nil pointer")
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

type DelegateSelector struct {
	Labels     map[string]string
	Namespaces []string
}

type Delegate struct {
	Single   core.ResourceRef
	Selector DelegateSelector
}

type AwsDestinationSpec struct {
	LogicalName            string
	ResponseTransformation bool
	UnwrapAsAlb            bool
	UnwrapAsAPIGateway     bool
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
	RateLimit   RateLimit
	AuthConfig  AuthConfig
}

type InputAuthConfig struct {
	OIDCAuth   OIDCAuth
	ApiKeyAuth ApiKeyAuth
	OpaAuth    OpaAuth
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
	// If not explicitly set, Gloo will automatically set this to true for port 443
	UseTls UseTls
}

type UseTls struct {
	Value *bool
}

// methods to implement pflag Value interface https://github.com/spf13/pflag/blob/d5e0c0615acee7028e1e2740a11102313be88de1/flag.go#L187
func (u *UseTls) String() string {
	if u == nil || u.Value == nil {
		return "<nil>"
	}
	return strconv.FormatBool(*u.Value)
}

func (u *UseTls) Set(s string) error {
	if u == nil {
		return eris.New("nil pointer")
	}
	b, err := strconv.ParseBool(s)
	if err != nil {
		return err
	}
	u.Value = &b
	return nil
}

func (u *UseTls) Type() string {
	return "bool"
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
	for _, name := range rltypes.RateLimit_Unit_name {
		vals = append(vals, name)
	}
	sort.Strings(vals)
	return vals
}()

type AuthConfig struct {
	Name      string
	Namespace string
}

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
	ExtAuthServerUpstreamRef core.ResourceRef
}

type OpaAuth struct {
	Enable bool

	Query   string
	Modules []string
}

type Cluster struct {
	FederationNamespace string
	Register            Register
	Deregister          Register
}

type Register struct {
	RemoteKubeConfig           string
	RemoteContext              string
	ClusterName                string
	LocalClusterDomainOverride string
	RemoteNamespace            string
}

type Check struct {
	// The maximum length of time alloted to `glooctl check`. A value of zero means no timeout.
	CheckTimeout time.Duration
}

type CheckCRD struct {
	Version    string
	LocalChart string
	ShowYaml   bool
}

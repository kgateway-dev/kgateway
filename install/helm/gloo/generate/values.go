package generate

import (
	appsv1 "k8s.io/api/core/v1"
)

type HelmConfig struct {
	Config
	Global *Global `json:"global,omitempty"`
}

type Config struct {
	Namespace      *Namespace              `json:"namespace,omitempty"`
	Settings       *Settings               `json:"settings,omitempty"`
	Gloo           *Gloo                   `json:"gloo,omitempty"`
	Discovery      *Discovery              `json:"discovery,omitempty"`
	Gateway        *Gateway                `json:"gateway,omitempty"`
	GatewayProxies map[string]GatewayProxy `json:"gatewayProxies,omitempty"`
	Ingress        *Ingress                `json:"ingress,omitempty"`
	IngressProxy   *IngressProxy           `json:"ingressProxy,omitempty"`
	K8s            *K8s                    `json:"k8s,omitempty"`
	AccessLogger   *AccessLogger           `json:"accessLogger,omitempty"`
}

type Global struct {
	Image             *Image             `json:"image,omitempty"`
	Extensions        interface{}        `json:"extensions,omitempty"`
	GlooRbac          *Rbac              `json:"glooRbac,omitempty"`
	GlooStats         Stats              `json:"glooStats,omitempty" desc:"Config used as the default values for Prometheus stats published from Gloo Edge pods. Can be overridden by individual deployments"`
	GlooMtls          Mtls               `json:"glooMtls,omitempty" desc:"Config used to enable internal mtls authentication"`
	IstioSDS          IstioSDS           `json:"istioSDS,omitempty" desc:"Config used for installing Gloo Edge with Istio SDS cert rotation features to facilitate Istio mTLS"`
	IstioIntegration  IstioIntegration   `json:"istioIntegration,omitempty" desc:"Configs user to manage Gloo pod visibility for Istio's' automatic discovery and sidecar injection."`
	ExtraSpecs        *bool              `json:"extraSpecs,omitempty" desc:"Add additional specs to include in the settings manifest, as defined by a helm partial. Defaults to false in open source, and true in enterprise."`
	ExtauthCustomYaml *bool              `json:"extauthCustomYaml,omitempty" desc:"Inject whatever yaml exists in .Values.global.extensions.extAuth into settings.spec.extauth, instead of structured yaml (which is enterprise only). Defaults to true in open source, and false in enterprise"`
	Console           interface{}        `json:"console,omitempty" desc:"Configuration options for the Enterprise Console (UI)."`
	Graphql           interface{}        `json:"graphql,omitempty" desc:"(Enterprise Only): GraphQL configuration options."`
	ConfigMaps        []*GlobalConfigMap `json:"configMaps,omitempty" desc:"Config used to create ConfigMaps at install time to store arbitrary data."`
}

type Namespace struct {
	Create *bool `json:"create,omitempty" desc:"create the installation namespace"`
}

type Rbac struct {
	Create     *bool   `json:"create,omitempty" desc:"create rbac rules for the gloo-system service account"`
	Namespaced *bool   `json:"namespaced,omitempty" desc:"use Roles instead of ClusterRoles"`
	NameSuffix *string `json:"nameSuffix,omitempty" desc:"When nameSuffix is nonempty, append '-$nameSuffix' to the names of Gloo Edge RBAC resources; e.g. when nameSuffix is 'foo', the role 'gloo-resource-reader' will become 'gloo-resource-reader-foo'"`
}

// Common
type Image struct {
	Tag        *string `json:"tag,omitempty"  desc:"The image tag for the container."`
	Repository *string `json:"repository,omitempty"  desc:"The image repository (name) for the container."`
	Digest     *string `json:"digest,omitempty"  desc:"The hash digest of the container's image, ie. sha256:12345...."`
	FipsDigest *string `json:"fipsDigest,omitempty"  desc:"The hash digest of the container's fips image, ie. sha256:12345....  Only consumed if fips=true"`
	Registry   *string `json:"registry,omitempty" desc:"The image hostname prefix and registry, such as quay.io/solo-io."`
	PullPolicy *string `json:"pullPolicy,omitempty"  desc:"The image pull policy for the container. For default values, see the Kubernetes docs: https://kubernetes.io/docs/concepts/containers/images/#imagepullpolicy-defaulting"`
	PullSecret *string `json:"pullSecret,omitempty" desc:"The image pull secret to use for the container, in the same namespace as the container pod."`
	Extended   *bool   `json:"extended,omitempty" desc:"If true, deploys an extended version of the container with additional debug tools."`
	Fips       *bool   `json:"fips,omitempty" desc:"If true, deploys a version of the data-plane containers that is built with FIPS-compliant crypto libraries. (Enterprise-only feature.)"`
}

type ResourceAllocation struct {
	Memory *string `json:"memory,omitempty" desc:"amount of memory"`
	CPU    *string `json:"cpu,omitempty" desc:"amount of CPUs"`
}

type ResourceRequirements struct {
	Limits   *ResourceAllocation `json:"limits,omitempty" desc:"resource limits of this container"`
	Requests *ResourceAllocation `json:"requests,omitempty" desc:"resource requests of this container"`
}

type PodSpec struct {
	RestartPolicy     *string                `json:"restartPolicy,omitempty" desc:"restart policy to use when the pod exits"`
	PriorityClassName *string                `json:"priorityClassName,omitempty" desc:"name of a defined priority class"`
	NodeName          *string                `json:"nodeName,omitempty" desc:"name of node to run on"`
	NodeSelector      map[string]string      `json:"nodeSelector,omitempty" desc:"label selector for nodes"`
	Tolerations       []*appsv1.Toleration   `json:"tolerations,omitempty"`
	Affinity          map[string]interface{} `json:"affinity,omitempty"`
	HostAliases       []interface{}          `json:"hostAliases,omitempty"`
	InitContainers    []interface{}          `json:"initContainers,omitempty" desc:"[InitContainers](https://kubernetes.io/docs/reference/kubernetes-api/workload-resources/pod-v1/#containers) to be added to the array of initContainers on the deployment."`
}

type JobSpec struct {
	*PodSpec
	ActiveDeadlineSeconds   *int              `json:"activeDeadlineSeconds,omitempty" desc:"Deadline in seconds for Kubernetes jobs."`
	BackoffLimit            *int              `json:"backoffLimit,omitempty" desc:"Specifies the number of retries before marking this job failed. In kubernetes, defaults to 6"`
	Completions             *int              `json:"completions,omitempty" desc:"Specifies the desired number of successfully finished pods the job should be run with."`
	ManualSelector          *bool             `json:"manualSelector,omitempty" desc:"Controls generation of pod labels and pod selectors."`
	Parallelism             *int              `json:"parallelism,omitempty" desc:"Specifies the maximum desired number of pods the job should run at any given time."`
	TtlSecondsAfterFinished *int              `json:"ttlSecondsAfterFinished,omitempty" desc:"Clean up the finished job after this many seconds. Defaults to 300 for the rollout jobs and 60 for the rest."`
	ExtraPodLabels          map[string]string `json:"extraPodLabels,omitempty" desc:"Optional extra key-value pairs to add to the spec.template.metadata.labels data of the job."`
	ExtraPodAnnotations     map[string]string `json:"extraPodAnnotations,omitempty" desc:"Optional extra key-value pairs to add to the spec.template.metadata.annotations data of the job."`
}

type DeploymentSpecSansResources struct {
	Replicas  *int             `json:"replicas,omitempty" desc:"number of instances to deploy"`
	CustomEnv []*appsv1.EnvVar `json:"customEnv,omitempty" desc:"custom extra environment variables for the container"`
	*PodSpec
}

type DeploymentSpec struct {
	DeploymentSpecSansResources
	Resources *ResourceRequirements `json:"resources,omitempty" desc:"resources for the main pod in the deployment"`
	*KubeResourceOverride
}

// Used to override any field in generated kubernetes resources.
type KubeResourceOverride struct {
	KubeResourceOverride map[string]interface{} `json:"kubeResourceOverride,omitempty" desc:"override fields in the generated resource by specifying the yaml structure to override under the top-level key."`
}

type Integrations struct {
	Knative                 *Knative                 `json:"knative,omitempty"`
	Consul                  *Consul                  `json:"consul,omitempty" desc:"Consul settings to inject into the consul client on startup"`
	ConsulUpstreamDiscovery *ConsulUpstreamDiscovery `json:"consulUpstreamDiscovery,omitempty" desc:"Settings for Gloo Edge's behavior when discovering consul services and creating upstreams for them."`
}

type Consul struct {
	Datacenter         *string                  `json:"datacenter,omitempty" desc:"Datacenter to use. If not provided, the default agent datacenter is used."`
	Username           *string                  `json:"username,omitempty" desc:"Username to use for HTTP Basic Authentication."`
	Password           *string                  `json:"password,omitempty" desc:"Password to use for HTTP Basic Authentication."`
	Token              *string                  `json:"token,omitempty" desc:"Token is used to provide a per-request ACL token which overrides the agent's default token."`
	CaFile             *string                  `json:"caFile,omitempty" desc:"caFile is the optional path to the CA certificate used for Consul communication, defaults to the system bundle if not specified."`
	CaPath             *string                  `json:"caPath,omitempty" desc:"caPath is the optional path to a directory of CA certificates to use for Consul communication, defaults to the system bundle if not specified."`
	CertFile           *string                  `json:"certFile,omitempty" desc:"CertFile is the optional path to the certificate for Consul communication. If this is set then you need to also set KeyFile."`
	KeyFile            *string                  `json:"keyFile,omitempty" desc:"KeyFile is the optional path to the private key for Consul communication. If this is set then you need to also set CertFile."`
	InsecureSkipVerify *bool                    `json:"insecureSkipVerify,omitempty" desc:"InsecureSkipVerify if set to true will disable TLS host verification."`
	WaitTime           *string                  `json:"waitTime,omitempty" desc:"WaitTime limits how long a watches for Consul resources will block. If not provided, the agent default values will be used."`
	ServiceDiscovery   *ServiceDiscoveryOptions `json:"serviceDiscovery,omitempty" desc:"Enable Service Discovery via Consul with this field set to empty struct '{}' to enable with defaults"`
	HttpAddress        *string                  `json:"httpAddress,omitempty" desc:"The address of the Consul HTTP server. Used by service discovery and key-value storage (if-enabled). Defaults to the value of the standard CONSUL_HTTP_ADDR env if set, otherwise to 127.0.0.1:8500."`
	DnsAddress         *string                  `json:"dnsAddress,omitempty" desc:"The address of the DNS server used to resolve hostnames in the Consul service address. Used by service discovery (required when Consul service instances are stored as DNS names). Defaults to 127.0.0.1:8600. (the default Consul DNS server)"`
	DnsPollingInterval *string                  `json:"dnsPollingInterval,omitempty" desc:"The polling interval for the DNS server. If there is a Consul service address with a hostname instead of an IP, Gloo Edge will resolve the hostname with the configured frequency to update endpoints with any changes to DNS resolution. Defaults to 5s."`
}

type ServiceDiscoveryOptions struct {
	DataCenters []string `json:"dataCenters,omitempty" desc:"Use this parameter to restrict the data centers that will be considered when discovering and routing to services. If not provided, Gloo Edge will use all available data centers."`
}

type ConsulUpstreamDiscovery struct {
	UseTlsTagging    *bool        `json:"useTlsTagging,omitempty" desc:"Allow Gloo Edge to automatically apply tls to consul services that are tagged the tlsTagName value. Requires RootCaResourceNamespace and RootCaResourceName to be set if true."`
	TlsTagName       *string      `json:"tlsTagName,omitempty" desc:"The tag Gloo Edge should use to identify consul services that ought to use TLS. If splitTlsServices is true, then this tag is also used to sort serviceInstances into the tls upstream. Defaults to 'glooUseTls'."`
	SplitTlsServices *bool        `json:"splitTlsServices,omitempty" desc:"If true, then create two upstreams to be created when a consul service contains the tls tag; one with TLS and one without."`
	RootCa           *ResourceRef `json:"rootCa,omitempty" desc:"The name/namespace of the root CA needed to use TLS with consul services."`
}

// equivalent of core.solo.io.ResourceRef
type ResourceRef struct {
	Namespace *string `json:"namespace,omitempty" desc:"The namespace of this resource."`
	Name      *string `json:"name,omitempty" desc:"The name of this resource."`
}

// google.protobuf.Duration
type Duration struct {
	Seconds *int32 `json:"seconds,omitempty" desc:"The value of this duration in seconds."`
	Nanos   *int32 `json:"nanos,omitempty" desc:"The value of this duration in nanoseconds."`
}

type Knative struct {
	Enabled                         *bool             `json:"enabled,omitempty" desc:"enabled knative components"`
	Version                         *string           `json:"version,omitempty" desc:"the version of knative installed to the cluster. if using version < 0.8.0, Gloo Edge will use Knative's ClusterIngress API for configuration rather than the namespace-scoped Ingress"`
	Proxy                           *KnativeProxy     `json:"proxy,omitempty"`
	RequireIngressClass             *bool             `json:"requireIngressClass,omitempty" desc:"only serve traffic for Knative Ingress objects with the annotation 'networking.knative.dev/ingress.class: gloo.ingress.networking.knative.dev'."`
	ExtraKnativeInternalLabels      map[string]string `json:"extraKnativeInternalLabels,omitempty" desc:"Optional extra key-value pairs to add to the spec.template.metadata.labels data of the knative internal deployment."`
	ExtraKnativeInternalAnnotations map[string]string `json:"extraKnativeInternalAnnotations,omitempty" desc:"Optional extra key-value pairs to add to the spec.template.metadata.annotations data of the knative internal deployment."`
	ExtraKnativeExternalLabels      map[string]string `json:"extraKnativeExternalLabels,omitempty" desc:"Optional extra key-value pairs to add to the spec.template.metadata.labels data of the knative external deployment."`
	ExtraKnativeExternalAnnotations map[string]string `json:"extraKnativeExternalAnnotations,omitempty" desc:"Optional extra key-value pairs to add to the spec.template.metadata.annotations data of the knative external deployment."`
}

type KnativeProxy struct {
	Image                               *Image                `json:"image,omitempty"`
	HttpPort                            *int                  `json:"httpPort,omitempty" desc:"HTTP port for the proxy"`
	HttpsPort                           *int                  `json:"httpsPort,omitempty" desc:"HTTPS port for the proxy"`
	Tracing                             *string               `json:"tracing,omitempty" desc:"tracing configuration"`
	LoopBackAddress                     *string               `json:"loopBackAddress,omitempty" desc:"Name on which to bind the loop-back interface for this instance of Envoy. Defaults to 127.0.0.1, but other common values may be localhost or ::1"`
	Stats                               *bool                 `json:"stats,omitempty" desc:"Controls whether or not Envoy stats are enabled"`
	ExtraClusterIngressProxyLabels      map[string]string     `json:"extraClusterIngressProxyLabels,omitempty" desc:"Optional extra key-value pairs to add to the spec.template.metadata.labels data of the cluster ingress proxy deployment."`
	ExtraClusterIngressProxyAnnotations map[string]string     `json:"extraClusterIngressProxyAnnotations,omitempty" desc:"Optional extra key-value pairs to add to the spec.template.metadata.annotations data of the cluster ingress proxy deployment."`
	Internal                            *KnativeProxyInternal `json:"internal,omitempty" desc:"kube resource overrides for knative internal proxy resources"`
	*DeploymentSpec
	*ServiceSpec
	ConfigMap  *KubeResourceOverride `json:"configMap,omitempty"`
	Deployment *KubeResourceOverride `json:"deployment,omitempty"`
}

type KnativeProxyInternal struct {
	Deployment *KubeResourceOverride `json:"deployment,omitempty"`
	Service    *KubeResourceOverride `json:"service,omitempty"`
	ConfigMap  *KubeResourceOverride `json:"configMap,omitempty"`
}

type Settings struct {
	WatchNamespaces               []string                `json:"watchNamespaces,omitempty" desc:"whitelist of namespaces for Gloo Edge to watch for services and CRDs. Empty list means all namespaces"`
	WriteNamespace                *string                 `json:"writeNamespace,omitempty" desc:"namespace where intermediary CRDs will be written to, e.g. Upstreams written by Gloo Edge Discovery."`
	Integrations                  *Integrations           `json:"integrations,omitempty"`
	Create                        *bool                   `json:"create,omitempty" desc:"create a Settings CRD which provides bootstrap configuration to Gloo Edge controllers"`
	Extensions                    interface{}             `json:"extensions,omitempty"`
	SingleNamespace               *bool                   `json:"singleNamespace,omitempty" desc:"Enable to use install namespace as WatchNamespace and WriteNamespace"`
	InvalidConfigPolicy           *InvalidConfigPolicy    `json:"invalidConfigPolicy,omitempty" desc:"Define policies for Gloo Edge to handle invalid configuration"`
	Linkerd                       *bool                   `json:"linkerd,omitempty" desc:"Enable automatic Linkerd integration in Gloo Edge"`
	DisableProxyGarbageCollection *bool                   `json:"disableProxyGarbageCollection,omitempty" desc:"Set this option to determine the state of an Envoy listener when the corresponding Proxy resource has no routes. If false (default), Gloo Edge will propagate the state of the Proxy to Envoy, resetting the listener to a clean slate with no routes. If true, Gloo Edge will keep serving the routes from the last applied valid configuration."`
	RegexMaxProgramSize           *uint32                 `json:"regexMaxProgramSize,omitempty" desc:"Set this field to specify the RE2 default max program size which is a rough estimate of how complex the compiled regex is to evaluate. If not specified, this defaults to 1024."`
	DisableKubernetesDestinations *bool                   `json:"disableKubernetesDestinations,omitempty" desc:"Gloo Edge allows you to directly reference a Kubernetes service as a routing destination. To enable this feature, Gloo Edge scans the cluster for Kubernetes services and creates a special type of in-memory Upstream to represent them. If the cluster contains a lot of services and you do not restrict the namespaces Gloo Edge is watching, this can result in significant overhead. If you do not plan on using this feature, you can set this flag to true to turn it off."`
	Aws                           AwsSettings             `json:"aws,omitempty"`
	RateLimit                     interface{}             `json:"rateLimit,omitempty" desc:"Partial config for Gloo Edge Enterprise’s rate-limiting service, based on Envoy’s rate-limit service; supports Envoy’s rate-limit service API. (reference here: https://github.com/lyft/ratelimit#configuration) Configure rate-limit descriptors here, which define the limits for requests based on their descriptors. Configure rate-limits (composed of actions, which define how request characteristics get translated into descriptors) on the VirtualHost or its routes."`
	RatelimitServer               interface{}             `json:"ratelimitServer,omitempty" desc:"External Ratelimit Server configuration for Gloo Edge Open Sources’s rate-limiting service, based on Envoy’s rate-limit service; supports Envoy’s rate-limit service API. (reference here: https://docs.solo.io/gloo-edge/main/guides/security/rate_limiting/)"`
	CircuitBreakers               CircuitBreakersSettings `json:"circuitBreakers,omitempty" desc:"Set this to configure the circuit breaker settings for Gloo."`
	EnableRestEds                 *bool                   `json:"enableRestEds,omitempty" desc:"Whether or not to use rest xds for all EDS by default. Defaults to false."`
	DevMode                       *bool                   `json:"devMode,omitempty" desc:"Whether or not to enable dev mode. Defaults to false. Setting to true at install time will expose the gloo dev admin endpoint on port 10010. Not recommended for production."`
	SecretOptions                 SecretOptions           `json:"secretOptions,omitempty" desc:"Options for how Gloo Edge should handle secrets."`
	*KubeResourceOverride
}

type AwsSettings struct {
	EnableCredentialsDiscovery      *bool     `json:"enableCredentialsDiscovery,omitempty" desc:"Enable AWS credentials discovery in Envoy for lambda requests. If enableServiceAccountCredentials is also set, it will take precedence as only one may be enabled in Gloo Edge"`
	EnableServiceAccountCredentials *bool     `json:"enableServiceAccountCredentials,omitempty" desc:"Use ServiceAccount credentials to authenticate lambda requests. If enableCredentialsDiscovery is also set, this will take precedence as only one may be enabled in Gloo Edge"`
	StsCredentialsRegion            *string   `json:"stsCredentialsRegion,omitempty" desc:"Regional endpoint to use for AWS STS requests. If empty will default to global sts endpoint."`
	PropagateOriginalRouting        *bool     `json:"propagateOriginalRouting,omitempty" desc:"Send downstream path and method as x-envoy-original-path and x-envoy-original-method headers on the request to AWS lambda."`
	CredentialRefreshDelay          *Duration `json:"credential_refresh_delay,omitempty" desc:"Adds a timed refresh to for ServiceAccount credentials in addition to the default filewatch."`
	FallbackToFirstFunction         *bool     `json:"fallbackToFirstFunction,omitempty" desc:"It will use the first function which if discovery is enabled the first function is the first function name alphabetically from the last discovery run. Defaults to false."`
}

type SecretOptions struct {
	Sources []*SecretOptionsSource `json:"sources,omitempty" desc:"List of sources to use for secrets."`
}

type SecretOptionsSource struct {
	Kubernetes KubernetesSecrets `json:"kubernetes,omitempty" desc:"Only one of kubernetes, vault, or directory may be set"`
	Vault      VaultSecrets      `json:"vault,omitempty" desc:"Only one of kubernetes, vault, or directory may be set"`
	Directory  Directory         `json:"directory,omitempty" desc:"Only one of kubernetes, vault, or directory may be set"`
}

type KubernetesSecrets struct {
}

type VaultSecrets struct {
	Address     string         `json:"address,omitempty" desc:"Address of the Vault server. This should be a complete URL such as http://solo.io and include port if necessary (vault's default port is 8200)."`
	RootKey     string         `json:"rootKey,omitempty" desc:"All keys stored in Vault will begin with this Vault this can be used to run multiple instances of Gloo against the same Vault cluster defaults to gloo."`
	PathPrefix  string         `json:"pathPrefix,omitempty" desc:"Optional. The name of a Vault Secrets Engine to which Vault should route traffic. For more info see https://learn.hashicorp.com/tutorials/vault/getting-started-secrets-engines. Defaults to 'secret'."`
	TlsConfig   VaultTlsConfig `json:"tlsConfig,omitempty" desc:"Configure TLS options for client connection to Vault. This is only available when running Gloo Edge outside of an container orchestration tool such as Kubernetes or Nomad."`
	AccessToken string         `json:"accessToken,omitempty" desc:"Vault token to use for authentication. Only one of accessToken or aws may be set."`
	Aws         VaultAwsAuth   `json:"aws,omitempty" desc:"Only one of accessToken or aws may be set."`
}

type VaultTlsConfig struct {
	CaCert        string `json:"caCert,omitempty" desc:"Path to a PEM-encoded CA cert file to use to verify the Vault server SSL certificate."`
	CaPath        string `json:"caPath,omitempty" desc:"Path to a directory of PEM-encoded CA cert files to verify the Vault server SSL certificate."`
	ClientCert    string `json:"clientCert,omitempty" desc:"Path to the certificate for Vault communication."`
	ClientKey     string `json:"clientKey,omitempty" desc:"Path to the private key for Vault communication."`
	TlsServerName string `json:"tlsServerName,omitempty" desc:"If set, it is used to set the SNI host when connecting via TLS."`
	Insecure      bool   `json:"insecure,omitempty" desc:"Disables TLS verification when set to true."`
}

type VaultAwsAuth struct {
	VaultRole         string `json:"vaultRole,omitempty" desc:"The Vault role we are trying to authenticate to. This is not necessarily the same as the AWS role to which the Vault role is configured."`
	Region            string `json:"region,omitempty" desc:"The AWS region to use for the login attempt."`
	IamServerIdHeader string `json:"iamServerIdHeader,omitempty" desc:"The IAM Server ID Header required to be included in the request."`
	MountPath         string `json:"mountPath,omitempty" desc:"The Vault path on which the AWS auth is mounted."`
	AccessKeyID       string `json:"accessKeyID,omitempty" desc:"Optional. The Access Key ID as provided by the security credentials on the AWS IAM resource. In cases such as receiving temporary credentials through assumed roles with AWS Security Token Service (STS) or IAM Roles for Service Accounts (IRSA), this field can be omitted. https://developer.hashicorp.com/vault/docs/auth/aws#iam-authentication-inferences."`
	SecretAccessKey   string `json:"secretAccessKey,omitempty" desc:"Optional. The Secret Access Key as provided by the security credentials on the AWS IAM resource. In cases such as receiving temporary credentials through assumed roles with AWS Security Token Service (STS) or IAM Roles for Service Accounts (IRSA), this field can be omitted. https://developer.hashicorp.com/vault/docs/auth/aws#iam-authentication-inferences."`
	SessionToken      string `json:"sessionToken,omitempty" desc:"The Session Token as provided by the security credentials on the AWS IAM resource."`
}

type Directory struct {
	Directory string `json:"directory,omitempty" desc:"Directory to read secrets from."`
}

type CircuitBreakersSettings struct {
	MaxConnections     *uint32 `json:"maxConnections,omitempty" desc:"Set this field to specify the maximum number of connections that Envoy will make to the upstream cluster. If not specified, the default is 1024."`
	MaxPendingRequests *uint32 `json:"maxPendingRequests,omitempty" desc:"Set this field to specfify the maximum number of pending requests that Envoy will allow to the upstream cluster. If not specified, the default is 1024."`
	MaxRequests        *uint32 `json:"maxRequests,omitempty" desc:"Set this field to specfify the maximum number of parallel requests that Envoy will make to the upstream cluster. If not specified, the default is 1024."`
	MaxRetries         *uint32 `json:"maxRetries,omitempty" desc:"Set this field to specify the maximum number of parallel retries that Envoy will allow to the upstream cluster. If not specified, the default is 3."`
}

type InvalidConfigPolicy struct {
	ReplaceInvalidRoutes     *bool   `json:"replaceInvalidRoutes,omitempty" desc:"Rather than pausing configuration updates, in the event of an invalid Route defined on a virtual service or route table, Gloo Edge will serve the route with a predefined direct response action. This allows valid routes to be updated when other routes are invalid."`
	InvalidRouteResponseCode *int64  `json:"invalidRouteResponseCode,omitempty" desc:"the response code for the direct response"`
	InvalidRouteResponseBody *string `json:"invalidRouteResponseBody,omitempty" desc:"the response body for the direct response"`
}

type Gloo struct {
	Deployment                 *GlooDeployment `json:"deployment,omitempty"`
	ServiceAccount             `json:"serviceAccount,omitempty"`
	SplitLogOutput             *bool                 `json:"splitLogOutput,omitempty" desc:"Set to true to send debug/info/warning logs to stdout, error/fatal/panic to stderr. Set to false to send all logs to stdout"`
	GlooService                *KubeResourceOverride `json:"service,omitempty"`
	LogLevel                   *string               `json:"logLevel,omitempty" desc:"Level at which the pod should log. Options include \"info\", \"debug\", \"warn\", \"error\", \"panic\" and \"fatal\". Default level is info"`
	DisableLeaderElection      *bool                 `json:"disableLeaderElection,omitempty" desc:"Set to true to disable leader election, and ensure all running replicas are considered the leader. Do not enable this with multiple replicas of Gloo"`
	HeaderSecretRefNsMatchesUs *bool                 `json:"headerSecretRefNsMatchesUs,omitempty" desc:"Set to true to require that secrets sent in headers via headerSecretRefs come from the same namespace as the destination upstream. Default: false"`
	PodDisruptionBudget        *PodDisruptionBudget  `json:"podDisruptionBudget,omitempty"`
}

type SecurityOpts struct {
	MergePolicy *string `json:"mergePolicy,omitempty" desc:"How to combine the defined security policy with the default security policy. Valid values are \"\", \"no-merge\", and \"helm-merge\". If defined as an empty string or \"no-merge\", use the defined security context as is.  If \"helm-merge\", merge this security context with the default security context according to the logic of [the helm 'merge' function](https://helm.sh/docs/chart_template_guide/function_list/#merge-mustmerge). This is intended to be used to modify a field in a security context, while using all other default values. Please note that due to how helm's 'merge' function works, you can not override a 'true' value with a 'false' value, and for that case you will need to define the entire security context and set this values to false. Default value is \"\"."`
}
type PodSecurityContext struct {
	*appsv1.PodSecurityContext
	*SecurityOpts
}

type SecurityContext struct {
	*appsv1.SecurityContext
	*SecurityOpts
}

type GlooDeployment struct {
	XdsPort               *int                `json:"xdsPort,omitempty" desc:"port where gloo serves xDS API to Envoy."`
	RestXdsPort           *uint32             `json:"restXdsPort,omitempty" desc:"port where gloo serves REST xDS API to Envoy."`
	ValidationPort        *int                `json:"validationPort,omitempty" desc:"port where gloo serves gRPC Proxy Validation to Gateway."`
	ProxyDebugPort        *int                `json:"proxyDebugPort,omitempty" desc:"port where gloo serves gRPC Proxy contents to glooctl."`
	Stats                 *Stats              `json:"stats,omitempty" desc:"overrides for prometheus stats published by the gloo pod."`
	FloatingUserId        *bool               `json:"floatingUserId,omitempty" desc:"If true, allows the cluster to dynamically assign a user ID for the processes running in the container. If a SecurityContext is defined for the container, this value is not applied for the container."`
	RunAsUser             *float64            `json:"runAsUser,omitempty" desc:"Explicitly set the user ID for the processes in the container to run as. Default is 10101. If a SecurityContext is defined for the pod or container, this value is not applied for the pod/container."`
	ExternalTrafficPolicy *string             `json:"externalTrafficPolicy,omitempty" desc:"Set the external traffic policy on the gloo service."`
	ExtraGlooLabels       map[string]string   `json:"extraGlooLabels,omitempty" desc:"Optional extra key-value pairs to add to the spec.template.metadata.labels data of the primary gloo deployment."`
	ExtraGlooAnnotations  map[string]string   `json:"extraGlooAnnotations,omitempty" desc:"Optional extra key-value pairs to add to the spec.template.metadata.annotations data of the primary gloo deployment."`
	LivenessProbeEnabled  *bool               `json:"livenessProbeEnabled,omitempty" desc:"Set to true to enable a liveness probe for Gloo Edge (default is false)."`
	OssImageTag           *string             `json:"ossImageTag,omitempty" desc:"Used for debugging. The version of Gloo OSS that the current version of Gloo Enterprise was built with."`
	PodSecurityContext    *PodSecurityContext `json:"podSecurityContext,omitempty" desc:"podSecurityContext for the gloo deployment. If this is defined it supercedes any values set in FloatingUserId, RunAsUser, or FsGroup.  See [pod security context](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.26/#podsecuritycontext-v1-core) for details."`
	*DeploymentSpec
	*GlooDeploymentContainer
}

type GlooDeploymentContainer struct {
	Image                        *Image           `json:"image,omitempty"`
	GlooContainerSecurityContext *SecurityContext `json:"glooContainerSecurityContext,omitempty" desc:"securityContext the for gloo container. If this is defined it supercedes any values set in FloatingUserId, RunAsUser, DisableNetBind, RunUnprivileged. See [security context](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.26/#securitycontext-v1-core) for details."`
}

type Discovery struct {
	Deployment     *DiscoveryDeployment `json:"deployment,omitempty"`
	FdsMode        *string              `json:"fdsMode,omitempty" desc:"mode for function discovery (blacklist or whitelist). See more info in the settings docs"`
	UdsOptions     *UdsOptions          `json:"udsOptions,omitempty" desc:"Configuration options for the Upstream Discovery Service (UDS)."`
	FdsOptions     *FdsOptions          `json:"fdsOptions,omitempty" desc:"Configuration options for the Function Discovery Service (FDS)."`
	Enabled        *bool                `json:"enabled,omitempty" desc:"enable Discovery features"`
	ServiceAccount `json:"serviceAccount,omitempty" `
	LogLevel       *string `json:"logLevel,omitempty" desc:"Level at which the pod should log. Options include \"info\", \"debug\", \"warn\", \"error\", \"panic\" and \"fatal\". Default level is info"`
}

type DiscoveryDeployment struct {
	Image                     *Image            `json:"image,omitempty"`
	Stats                     Stats             `json:"stats,omitempty" desc:"overrides for prometheus stats published by the discovery pod"`
	FloatingUserId            *bool             `json:"floatingUserId,omitempty" desc:"If true, allows the cluster to dynamically assign a user ID for the processes running in the container."`
	RunAsUser                 *float64          `json:"runAsUser,omitempty" desc:"Explicitly set the user ID for the processes in the container to run as. Default is 10101."`
	FsGroup                   *float64          `json:"fsGroup,omitempty" desc:"Explicitly set the group ID for volume ownership. Default is 10101"`
	ExtraDiscoveryLabels      map[string]string `json:"extraDiscoveryLabels,omitempty" desc:"Optional extra key-value pairs to add to the spec.template.metadata.labels data of the gloo edge discovery deployment."`
	ExtraDiscoveryAnnotations map[string]string `json:"extraDiscoveryAnnotations,omitempty" desc:"Optional extra key-value pairs to add to the spec.template.metadata.annotations data of the gloo edge discovery deployment."`
	EnablePodSecurityContext  *bool             `json:"enablePodSecurityContext,omitempty" desc:"Whether or not to render the pod security context. Default is true"`
	*DeploymentSpec
}

// Configuration options for the Upstream Discovery Service (UDS).
type UdsOptions struct {
	Enabled     *bool             `json:"enabled,omitempty" desc:"Enable upstream discovery service. Defaults to true."`
	WatchLabels map[string]string `json:"watchLabels,omitempty" desc:"Map of labels to watch. Only services which match all of the selectors specified here will be discovered by UDS."`
}

// Configuration options for the Function Discovery Service (FDS).
type FdsOptions struct {
	GraphqlEnabled *bool `json:"graphqlEnabled,omitempty" desc:"Enable GraphQL schema generation on the function discovery service. Defaults to true."`
}

type Gateway struct {
	Enabled                        *bool             `json:"enabled,omitempty" desc:"enable Gloo Edge API Gateway features"`
	Validation                     GatewayValidation `json:"validation,omitempty" desc:"enable Validation Webhook on the Gateway. This will cause requests to modify Gateway-related Custom Resources to be validated by the Gateway."`
	CertGenJob                     *CertGenJob       `json:"certGenJob,omitempty" desc:"generate self-signed certs with this job to be used with the gateway validation webhook. this job will only run if validation is enabled for the gateway"`
	RolloutJob                     *RolloutJob       `json:"rolloutJob,omitempty" desc:"This job waits for the 'gloo' deployment to successfully roll out (if the validation webhook is enabled), and then applies the Gloo Edge custom resources."`
	CleanupJob                     *CleanupJob       `json:"cleanupJob,omitempty" desc:"This job cleans up resources that are not deleted by Helm when Gloo Edge is uninstalled."`
	UpdateValues                   *bool             `json:"updateValues,omitempty" desc:"if true, will use a provided helm helper 'gloo.updatevalues' to update values during template render - useful for plugins/extensions"`
	ProxyServiceAccount            ServiceAccount    `json:"proxyServiceAccount,omitempty" `
	ReadGatewaysFromAllNamespaces  *bool             `json:"readGatewaysFromAllNamespaces,omitempty" desc:"if true, read Gateway custom resources from all watched namespaces rather than just the namespace of the Gateway controller"`
	IsolateVirtualHostsBySslConfig *bool             `json:"isolateVirtualHostsBySslConfig,omitempty" desc:"if true, Added support for the envoy.filters.listener.tls_inspector listener_filter when using the gateway.isolateVirtualHostsBySslConfig=true global setting."`
	CompressedProxySpec            *bool             `json:"compressedProxySpec,omitempty" desc:"if true, enables compression for the Proxy CRD spec"`
	PersistProxySpec               *bool             `json:"persistProxySpec,omitempty" desc:"Enable writing Proxy CRD to etcd. Disabled by default for performance."`
	TranslateEmptyGateways         *bool             `json:"translateEmptyGateways,omitempty" desc:"This field is a no-op for now"`
	Service                        *KubeResourceOverride
}

type ServiceAccount struct {
	ExtraAnnotations map[string]string `json:"extraAnnotations,omitempty" desc:"extra annotations to add to the service account"`
	DisableAutomount *bool             `json:"disableAutomount,omitempty" desc:"disable automounting the service account to the gateway proxy. not mounting the token hardens the proxy container, but may interfere with service mesh integrations"`
	*KubeResourceOverride
}

type GatewayValidation struct {
	Enabled                          *bool    `json:"enabled,omitempty" desc:"enable Gloo Edge API Gateway validation hook (default true)"`
	AlwaysAcceptResources            *bool    `json:"alwaysAcceptResources,omitempty" desc:"unless this is set this to false in order to ensure validation webhook rejects invalid resources. by default, validation webhook will only log and report metrics for invalid resource admission without rejecting them outright."`
	AllowWarnings                    *bool    `json:"allowWarnings,omitempty" desc:"set this to false in order to ensure validation webhook rejects resources that would have warning status or rejected status, rather than just rejected."`
	ServerEnabled                    *bool    `json:"serverEnabled,omitempty" desc:"By providing the validation field (parent of this object) the user is implicitly opting into validation. This field allows the user to opt out of the validation server, while still configuring pre-existing fields such as warn_route_short_circuiting and disable_transformation_validation."`
	DisableTransformationValidation  *bool    `json:"disableTransformationValidation,omitempty" desc:"set this to true to disable transformation validation. This may bring signifigant performance benefits if using many transformations, at the cost of possibly incorrect transformations being sent to Envoy. When using this value make sure to pre-validate transformations."`
	WarnRouteShortCircuiting         *bool    `json:"warnRouteShortCircuiting,omitempty" desc:"Write a warning to route resources if validation produced a route ordering warning (defaults to false). By setting to true, this means that Gloo Edge will start assigning warnings to resources that would result in route short-circuiting within a virtual host."`
	SecretName                       *string  `json:"secretName,omitempty" desc:"Name of the Kubernetes Secret containing TLS certificates used by the validation webhook server. This secret will be created by the certGen Job if the certGen Job is enabled."`
	FailurePolicy                    *string  `json:"failurePolicy,omitempty" desc:"failurePolicy defines how unrecognized errors from the Gateway validation endpoint are handled - allowed values are 'Ignore' or 'Fail'. Defaults to Ignore "`
	Webhook                          *Webhook `json:"webhook,omitempty" desc:"webhook specific configuration"`
	ValidationServerGrpcMaxSizeBytes *int     `json:"validationServerGrpcMaxSizeBytes,omitempty" desc:"gRPC max message size in bytes for the gloo validation server"`
	LivenessProbeEnabled             *bool    `json:"livenessProbeEnabled,omitempty" desc:"Set to true to enable a liveness probe for the gateway (default is false). You must also set the 'Probes' value to true."`
}

type Webhook struct {
	Enabled                       *bool             `json:"enabled,omitempty" desc:"enable validation webhook (default true)"`
	DisableHelmHook               *bool             `json:"disableHelmHook,omitempty" desc:"do not create the webhook as helm hook (default false)"`
	TimeoutSeconds                *int              `json:"timeoutSeconds,omitempty" desc:"the timeout for the webhook, defaults to 10"`
	ExtraAnnotations              map[string]string `json:"extraAnnotations,omitempty" desc:"extra annotations to add to the webhook"`
	SkipDeleteValidationResources []string          `json:"skipDeleteValidationResources,omitempty" desc:"resource types in this list will not use webhook valdaition for DELETEs. Use '*' to skip validation for all resources. Valid values are 'virtualservices', 'routetables','upstreams', 'secrets', 'ratelimitconfigs', and '*'. Invalid values will be accepted but will not be used."`
	*KubeResourceOverride
}

type Job struct {
	Image *Image `json:"image,omitempty"`
	*JobSpec
	KubeResourceOverride     map[string]interface{} `json:"kubeResourceOverride,omitempty" desc:"override fields in the gateway-certgen job."`
	MtlsKubeResourceOverride map[string]interface{} `json:"mtlsKubeResourceOverride,omitempty" desc:"override fields in the gloo-mtls-certgen job."`
}

type CertGenJob struct {
	Job
	Enabled             *bool                 `json:"enabled,omitempty" desc:"enable the job that generates the certificates for the validating webhook at install time (default true)"`
	SetTtlAfterFinished *bool                 `json:"setTtlAfterFinished,omitempty" desc:"Set ttlSecondsAfterFinished on the job. Defaults to true"`
	FloatingUserId      *bool                 `json:"floatingUserId,omitempty" desc:"If true, allows the cluster to dynamically assign a user ID for the processes running in the container."`
	ForceRotation       *bool                 `json:"forceRotation,omitempty" desc:"If true, will create new certs even if the old one are still valid."`
	RotationDuration    *string               `json:"rotationDuration,omitempty" desc:"Time duration string indicating the (environment-specific) expected time for all pods to pick up a secret update via SDS. This is only applicable to the mTLS certgen job and cron job. If this duration is too short, secret changes may not have time to propagate to all pods, and some requests may be dropped during cert rotation. Since we do 2 secret updates during a cert rotation, the certgen job is expected to run for at least twice this amount of time. If activeDeadlineSeconds is set on the job, make sure it is at least twice as long as the rotation duration, otherwise the certgen job might time out."`
	RunAsUser           *float64              `json:"runAsUser,omitempty" desc:"Explicitly set the user ID for the processes in the container to run as. Default is 10101."`
	Resources           *ResourceRequirements `json:"resources,omitempty"`
	RunOnUpdate         *bool                 `json:"runOnUpdate,omitempty" desc:"enable to run the job also on pre-upgrade"`
	Cron                *CertGenCron          `json:"cron,omitempty" desc:"CronJob parameters"`
}

type RolloutJob struct {
	*JobSpec
	Enabled        *bool                 `json:"enabled,omitempty" desc:"Enable the job that applies default Gloo Edge custom resources at install and upgrade time (default true)."`
	Image          *Image                `json:"image,omitempty"`
	Resources      *ResourceRequirements `json:"resources,omitempty"`
	FloatingUserId *bool                 `json:"floatingUserId,omitempty" desc:"If true, allows the cluster to dynamically assign a user ID for the processes running in the container."`
	RunAsUser      *float64              `json:"runAsUser,omitempty" desc:"Explicitly set the user ID for the processes in the container to run as. Default is 10101."`
	Timeout        *int                  `json:"timeout,omitempty" desc:"Time to wait in seconds until the job has completed. If it exceeds this limit, it is deemed to have failed. Defaults to 600"`
}

type CleanupJob struct {
	*JobSpec
	Enabled        *bool                 `json:"enabled,omitempty" desc:"Enable the job that removes Gloo Edge custom resources when Gloo Edge is uninstalled (default true)."`
	Image          *Image                `json:"image,omitempty"`
	Resources      *ResourceRequirements `json:"resources,omitempty"`
	FloatingUserId *bool                 `json:"floatingUserId,omitempty" desc:"If true, allows the cluster to dynamically assign a user ID for the processes running in the container."`
	RunAsUser      *float64              `json:"runAsUser,omitempty" desc:"Explicitly set the user ID for the processes in the container to run as. Default is 10101."`
}

/*
Scheduling:
┌───────────── minute (0 - 59)
│ ┌───────────── hour (0 - 23)
│ │ ┌───────────── day of the month (1 - 31)
│ │ │ ┌───────────── month (1 - 12)
│ │ │ │ ┌───────────── day of the week (0 - 6) (Sunday to Saturday;
│ │ │ │ │                                   7 is also Sunday on some systems)
│ │ │ │ │
│ │ │ │ │
* * * * *
*/
type CertGenCron struct {
	Enabled                               *bool                  `json:"enabled,omitempty" desc:"enable the cronjob"`
	Schedule                              *string                `json:"schedule,omitempty" desc:"Cron job scheduling"`
	MtlsKubeResourceOverride              map[string]interface{} `json:"mtlsKubeResourceOverride,omitempty" desc:"override fields in the gloo-mtls-certgen cronjob."`
	ValidationWebhookKubeResourceOverride map[string]interface{} `json:"validationWebhookKubeResourceOverride,omitempty" desc:"override fields in the gateway-certgen cronjob."`
}

type GatewayProxy struct {
	Kind                           *GatewayProxyKind                `json:"kind,omitempty" desc:"value to determine how the gateway proxy is deployed"`
	Namespace                      *string                          `json:"namespace,omitempty" desc:"Namespace in which to deploy this gateway proxy. Defaults to the value of Settings.WriteNamespace"`
	PodTemplate                    *GatewayProxyPodTemplate         `json:"podTemplate,omitempty"`
	ConfigMap                      *ConfigMap                       `json:"configMap,omitempty"`
	CustomStaticLayer              interface{}                      `json:"customStaticLayer,omitempty" desc:"Configure the static layer for global overrides to Envoy behavior, as defined in the Envoy bootstrap YAML. You cannot use this setting to set overload or upstream layers. For more info, see the Envoy docs. https://www.envoyproxy.io/docs/envoy/latest/configuration/operations/runtime#config-runtime"`
	GlobalDownstreamMaxConnections *uint32                          `json:"globalDownstreamMaxConnections,omitempty" desc:"the number of concurrent connections needed. limit used to protect against exhausting file descriptors on host machine"`
	HealthyPanicThreshold          *int8                            `json:"healthyPanicThreshold,omitempty" desc:"the percentage of healthy hosts required to load balance based on health status of hosts"`
	Service                        *GatewayProxyService             `json:"service,omitempty"`
	AntiAffinity                   *bool                            `json:"antiAffinity,omitempty" desc:"configure anti affinity such that pods are preferably not co-located"`
	Affinity                       map[string]interface{}           `json:"affinity,omitempty"`
	TopologySpreadConstraints      []interface{}                    `json:"topologySpreadConstraints,omitempty" desc:"configure topologySpreadConstraints for gateway proxy pods"`
	Tracing                        *Tracing                         `json:"tracing,omitempty"`
	GatewaySettings                *GatewayProxyGatewaySettings     `json:"gatewaySettings,omitempty" desc:"settings for the helm generated gateways, leave nil to not render"`
	ExtraEnvoyArgs                 []string                         `json:"extraEnvoyArgs,omitempty" desc:"Envoy container args, (e.g. https://www.envoyproxy.io/docs/envoy/latest/operations/cli)"`
	ExtraContainersHelper          *string                          `json:"extraContainersHelper,omitempty"`
	ExtraInitContainersHelper      *string                          `json:"extraInitContainersHelper,omitempty"`
	ExtraVolumes                   []map[string]interface{}         `json:"extraVolumes,omitempty"`
	ExtraVolumeHelper              *string                          `json:"extraVolumeHelper,omitempty"`
	ExtraListenersHelper           *string                          `json:"extraListenersHelper,omitempty"`
	Stats                          *Stats                           `json:"stats,omitempty" desc:"overrides for prometheus stats published by the gateway-proxy pod"`
	ReadConfig                     *bool                            `json:"readConfig,omitempty" desc:"expose a read-only subset of the Envoy admin api"`
	ReadConfigMulticluster         *bool                            `json:"readConfigMulticluster,omitempty" desc:"expose a read-only subset of the Envoy admin api to gloo-fed"`
	ExtraProxyVolumeMounts         []map[string]interface{}         `json:"extraProxyVolumeMounts,omitempty"`
	ExtraProxyVolumeMountHelper    *string                          `json:"extraProxyVolumeMountHelper,omitempty" desc:"name of custom made named template allowing for extra volume mounts on the proxy container"`
	LoopBackAddress                *string                          `json:"loopBackAddress,omitempty" desc:"Name on which to bind the loop-back interface for this instance of Envoy. Defaults to 127.0.0.1, but other common values may be localhost or ::1"`
	Failover                       Failover                         `json:"failover,omitempty" desc:"(Enterprise Only): Failover configuration"`
	Disabled                       *bool                            `json:"disabled,omitempty" desc:"Skips creation of this gateway proxy. Used to turn off gateway proxies created by preceding configurations"`
	EnvoyApiVersion                *string                          `json:"envoyApiVersion,omitempty" desc:"Version of the Envoy API to use for the xDS transport and resources. Default is V3"`
	EnvoyBootstrapExtensions       []map[string]interface{}         `json:"envoyBootstrapExtensions,omitempty" desc:"List of bootstrap extensions to add to Envoy bootstrap config. Examples include Wasm Service (https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/wasm/v3/wasm.proto#extensions-wasm-v3-wasmservice)."`
	EnvoyOverloadManager           map[string]interface{}           `json:"envoyOverloadManager,omitempty" desc:"Overload Manager definition for Envoy bootstrap config. If enabled, a list of Resource Monitors MUST be defined in order to produce a valid Envoy config (https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/overload/v3/overload.proto#overload-manager)."`
	EnvoyStaticClusters            []map[string]interface{}         `json:"envoyStaticClusters,omitempty" desc:"List of extra static clusters to be added to Envoy bootstrap config. https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/cluster/v3/cluster.proto#envoy-v3-api-msg-config-cluster-v3-cluster"`
	HorizontalPodAutoscaler        *HorizontalPodAutoscaler         `json:"horizontalPodAutoscaler,omitempty" desc:"HorizontalPodAutoscaler for the GatewayProxy. Used only when Kind is set to Deployment. Resources must be set on the gateway-proxy deployment for HorizontalPodAutoscalers to function correctly"`
	PodDisruptionBudget            *PodDisruptionBudgetWithOverride `json:"podDisruptionBudget,omitempty" desc:"PodDisruptionBudget is an object to define the max disruption that can be caused to the gate-proxy pods"`
	IstioMetaMeshId                *string                          `json:"istioMetaMeshId,omitempty" desc:"ISTIO_META_MESH_ID Environment Variable. Defaults to \"cluster.local\""`
	IstioMetaClusterId             *string                          `json:"istioMetaClusterId,omitempty" desc:"ISTIO_META_CLUSTER_ID Environment Variable. Defaults to \"Kubernetes\""`
	IstioDiscoveryAddress          *string                          `json:"istioDiscoveryAddress,omitempty" desc:"discoveryAddress field of the PROXY_CONFIG environment variable. Defaults to \"istiod.istio-system.svc:15012\""`
	EnvoyLogLevel                  *string                          `json:"envoyLogLevel,omitempty" desc:"Level at which the pod should log. Options include \"trace\", \"info\", \"debug\", \"warn\", \"error\", \"critical\" and \"off\". Default level is info"`
	EnvoyStatsConfig               map[string]interface{}           `json:"envoyStatsConfig,omitempty" desc:"Envoy statistics configuration, such as tagging. For more info, see https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/metrics/v3/stats.proto#config-metrics-v3-statsconfig"`
	XdsServiceAddress              *string                          `json:"xdsServiceAddress,omitempty" desc:"The k8s service name for the xds server. Defaults to gloo."`
	XdsServicePort                 *uint32                          `json:"xdsServicePort,omitempty" desc:"The k8s service port for the xds server. Defaults to the value from .Values.gloo.deployment.xdsPort, but can be overridden to use, for example, xds-relay."`
	TcpKeepaliveTimeSeconds        *uint32                          `json:"tcpKeepaliveTimeSeconds,omitempty" desc:"The amount of time in seconds for connections to be idle before sending keep-alive probes. Defaults to 60. See here: https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/core/v3/address.proto#envoy-v3-api-msg-config-core-v3-tcpkeepalive"`
	DisableCoreDumps               *bool                            `json:"disableCoreDumps,omitempty" desc:"If set to true, Envoy will not generate core dumps in the event of a crash. Defaults to false"`
	*KubeResourceOverride
}

type GatewayProxyGatewaySettings struct {
	Enabled                  *bool                  `json:"enabled,omitempty" desc:"enable/disable default gateways"`
	DisableGeneratedGateways *bool                  `json:"disableGeneratedGateways,omitempty" desc:"set to true to disable the gateway generation for a gateway proxy"`
	DisableHttpGateway       *bool                  `json:"disableHttpGateway,omitempty" desc:"Set to true to disable http gateway generation."`
	DisableHttpsGateway      *bool                  `json:"disableHttpsGateway,omitempty" desc:"Set to true to disable https gateway generation."`
	IPv4Only                 *bool                  `json:"ipv4Only,omitempty" desc:"set to true if your network allows ipv4 addresses only. Sets the Gateway spec's bindAddress to 0.0.0.0 instead of ::"`
	UseProxyProto            *bool                  `json:"useProxyProto,omitempty" desc:"use proxy protocol"`
	HttpHybridGateway        map[string]interface{} `json:"httpHybridGateway,omitempty" desc:"custom yaml to use for hybrid gateway settings for the http gateway"`
	HttpsHybridGateway       map[string]interface{} `json:"httpsHybridGateway,omitempty" desc:"custom yaml to use for hybrid gateway settings for the https gateway"`
	CustomHttpGateway        map[string]interface{} `json:"customHttpGateway,omitempty" desc:"custom yaml to use for http gateway settings"`
	CustomHttpsGateway       map[string]interface{} `json:"customHttpsGateway,omitempty" desc:"custom yaml to use for https gateway settings"`
	AccessLoggingService     map[string]interface{} `json:"accessLoggingService,omitempty" desc:"custom yaml to use for access logging service (https://docs.solo.io/gloo-edge/latest/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/options/als/als.proto.sk/)"`
	GatewayOptions           map[string]interface{} `json:"options,omitempty" desc:"custom options for http(s) gateways (https://docs.solo.io/gloo-edge/latest/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/options.proto.sk/#listeneroptions)"`
	HttpGatewayKubeOverride  map[string]interface{} `json:"httpGatewayKubeOverride,omitempty"`
	HttpsGatewayKubeOverride map[string]interface{} `json:"httpsGatewayKubeOverride,omitempty"`
	*KubeResourceOverride
}

type GatewayProxyKind struct {
	Deployment *GatewayProxyDeployment `json:"deployment,omitempty" desc:"set to deploy as a kubernetes deployment, otherwise nil"`
	DaemonSet  *DaemonSetSpec          `json:"daemonSet,omitempty" desc:"set to deploy as a kubernetes daemonset, otherwise nil"`
}
type GatewayProxyDeployment struct {
	*DeploymentSpecSansResources
	*KubeResourceOverride
}

type HorizontalPodAutoscaler struct {
	ApiVersion                     *string                  `json:"apiVersion,omitempty" desc:"accepts autoscaling/v1, autoscaling/v2beta2 or autoscaling/v2. Note: autoscaling/v2beta2 is deprecated as of Kubernetes 1.26."`
	MinReplicas                    *int32                   `json:"minReplicas,omitempty" desc:"minReplicas is the lower limit for the number of replicas to which the autoscaler can scale down."`
	MaxReplicas                    *int32                   `json:"maxReplicas,omitempty" desc:"maxReplicas is the upper limit for the number of replicas to which the autoscaler can scale up. It cannot be less that minReplicas."`
	TargetCPUUtilizationPercentage *int32                   `json:"targetCPUUtilizationPercentage,omitempty" desc:"target average CPU utilization (represented as a percentage of requested CPU) over all the pods. Used only with apiVersion autoscaling/v1"`
	Metrics                        []map[string]interface{} `json:"metrics,omitempty" desc:"metrics contains the specifications for which to use to calculate the desired replica count (the maximum replica count across all metrics will be used). Used only with apiVersion autoscaling/v2beta2"`
	Behavior                       map[string]interface{}   `json:"behavior,omitempty" desc:"behavior configures the scaling behavior of the target in both Up and Down directions (scaleUp and scaleDown fields respectively). Used only with apiVersion autoscaling/v2beta2"`
	*KubeResourceOverride
}

type PodDisruptionBudget struct {
	MinAvailable   *string `json:"minAvailable,omitempty" desc:"Corresponds directly with the _minAvailable_ field in the [PodDisruptionBudgetSpec](https://kubernetes.io/docs/reference/kubernetes-api/policy-resources/pod-disruption-budget-v1/#PodDisruptionBudgetSpec). This value is mutually exclusive with _maxUnavailable_."`
	MaxUnavailable *string `json:"maxUnavailable,omitempty" desc:"Corresponds directly with the _maxUnavailable_ field in the [PodDisruptionBudgetSpec](https://kubernetes.io/docs/reference/kubernetes-api/policy-resources/pod-disruption-budget-v1/#PodDisruptionBudgetSpec). This value is mutually exclusive with _minAvailable_."`
}

type PodDisruptionBudgetWithOverride struct {
	*PodDisruptionBudget
	*KubeResourceOverride
}

type DaemonSetSpec struct {
	HostPort    *bool `json:"hostPort,omitempty" desc:"whether or not to enable host networking on the pod. Only relevant when running as a DaemonSet"`
	HostNetwork *bool `json:"hostNetwork,omitempty"`
}

type GatewayProxyPodTemplate struct {
	HttpPort                      *int                  `json:"httpPort,omitempty" desc:"HTTP port for the gateway service target port."`
	HttpsPort                     *int                  `json:"httpsPort,omitempty" desc:"HTTPS port for the gateway service target port."`
	ExtraPorts                    []interface{}         `json:"extraPorts,omitempty" desc:"extra ports for the gateway pod."`
	ExtraAnnotations              map[string]string     `json:"extraAnnotations,omitempty" desc:"extra annotations to add to the pod."`
	NodeName                      *string               `json:"nodeName,omitempty" desc:"name of node to run on."`
	NodeSelector                  map[string]string     `json:"nodeSelector,omitempty" desc:"label selector for nodes."`
	Tolerations                   []*appsv1.Toleration  `json:"tolerations,omitempty"`
	Probes                        *bool                 `json:"probes,omitempty" desc:"Set to true to enable a readiness probe (default is false). Then, you can also enable a liveness probe."`
	LivenessProbeEnabled          *bool                 `json:"livenessProbeEnabled,omitempty" desc:"Set to true to enable a liveness probe (default is false)."`
	Resources                     *ResourceRequirements `json:"resources,omitempty"`
	DisableNetBind                *bool                 `json:"disableNetBind,omitempty" desc:"don't add the NET_BIND_SERVICE capability to the pod. This means that the gateway proxy will not be able to bind to ports below 1024. If podSecurityContext is defined, this value is not applied."`
	RunUnprivileged               *bool                 `json:"runUnprivileged,omitempty" desc:"run Envoy as an unprivileged user. If a SecurityContext is defined for the pod or container, this value is not applied for the pod/container."`
	FloatingUserId                *bool                 `json:"floatingUserId,omitempty" desc:"If true, allows the cluster to dynamically assign a user ID for the processes running in the container. If podSecurityContext is defined, this value is not applied."`
	RunAsUser                     *float64              `json:"runAsUser,omitempty" desc:"Explicitly set the user ID for the processes in the container to run as. Default is 10101. If a SecurityContext is defined for the pod or container, this value is not applied for the pod/container."`
	FsGroup                       *float64              `json:"fsGroup,omitempty" desc:"Explicitly set the group ID for volume ownership. Default is 10101. If podSecurityContext is defined, this value is not applied."`
	GracefulShutdown              *GracefulShutdownSpec `json:"gracefulShutdown,omitempty"`
	TerminationGracePeriodSeconds *int                  `json:"terminationGracePeriodSeconds,omitempty" desc:"Time in seconds to wait for the pod to terminate gracefully. See [kubernetes docs](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.18/#podspec-v1-core) for more info."`
	CustomReadinessProbe          *appsv1.Probe         `json:"customReadinessProbe,omitempty"`
	CustomLivenessProbe           *appsv1.Probe         `json:"customLivenessProbe,omitempty"`
	ExtraGatewayProxyLabels       map[string]string     `json:"extraGatewayProxyLabels,omitempty" desc:"Optional extra key-value pairs to add to the spec.template.metadata.labels data of the gloo edge gateway-proxy deployment."`
	ExtraContainers               []interface{}         `json:"extraContainers,omitempty" desc:"Extra [containers](https://kubernetes.io/docs/reference/kubernetes-api/workload-resources/pod-v1/#containers) to be added to the array of containers on the gateway proxy deployment."`
	ExtraInitContainers           []interface{}         `json:"extraInitContainers,omitempty" desc:"Extra [initContainers](https://kubernetes.io/docs/reference/kubernetes-api/workload-resources/pod-v1/#containers) to be added to the array of initContainers on the gateway proxy deployment."`
	EnablePodSecurityContext      *bool                 `json:"enablePodSecurityContext,omitempty" desc:"Whether or not to render the pod security context. Default is true."`
	PodSecurityContext            *PodSecurityContext   `json:"podSecurityContext,omitempty" desc:"podSecurityContext for the gateway proxy deployment. If this is defined it supercedes any values set in FloatingUserId, RunAsUser, or FsGroup.  See [pod security context](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.26/#podsecuritycontext-v1-core) for details."`
	*GlooDeploymentContainer
}

type GracefulShutdownSpec struct {
	Enabled          *bool `json:"enabled,omitempty" desc:"Enable grace period before shutdown to finish current requests while Envoy health checks fail to e.g. notify external load balancers. *NOTE:* This will not have any effect if you have not defined health checks via the health check filter"`
	SleepTimeSeconds *int  `json:"sleepTimeSeconds,omitempty" desc:"Time (in seconds) for the preStop hook to wait before allowing Envoy to terminate"`
}

type GatewayProxyService struct {
	Type                     *string               "json:\"type,omitempty\" desc:\"gateway [service type](https://kubernetes.io/docs/concepts/services-networking/service/#publishing-services-service-types). default is `LoadBalancer`\""
	HttpPort                 *int                  `json:"httpPort,omitempty" desc:"HTTP port for the gateway service"`
	HttpsPort                *int                  `json:"httpsPort,omitempty" desc:"HTTPS port for the gateway service"`
	HttpNodePort             *int                  `json:"httpNodePort,omitempty" desc:"HTTP nodeport for the gateway service if using type NodePort"`
	HttpsNodePort            *int                  `json:"httpsNodePort,omitempty" desc:"HTTPS nodeport for the gateway service if using type NodePort"`
	ClusterIP                *string               "json:\"clusterIP,omitempty\" desc:\"static clusterIP (or `None`) when `gatewayProxies[].gatewayProxy.service.type` is `ClusterIP`\""
	ExtraAnnotations         map[string]string     `json:"extraAnnotations,omitempty"`
	ExternalTrafficPolicy    *string               `json:"externalTrafficPolicy,omitempty"`
	Name                     *string               `json:"name,omitempty" desc:"Custom name override for the service resource of the proxy"`
	HttpsFirst               *bool                 `json:"httpsFirst,omitempty" desc:"List HTTPS port before HTTP"`
	LoadBalancerIP           *string               `json:"loadBalancerIP,omitempty" desc:"IP address of the load balancer"`
	LoadBalancerSourceRanges []string              `json:"loadBalancerSourceRanges,omitempty" desc:"List of IP CIDR ranges that are allowed to access the load balancer"`
	CustomPorts              []interface{}         `json:"customPorts,omitempty" desc:"List of custom port to expose in the Envoy proxy. Each element follows conventional port syntax (port, targetPort, protocol, name)"`
	ExternalIPs              []string              `json:"externalIPs,omitempty" desc:"externalIPs is a list of IP addresses for which nodes in the cluster will also accept traffic for this service"`
	ConfigDumpService        *KubeResourceOverride `json:"configDumpService,omitempty" desc:"kube resource override for gateway proxy config dump service"`
	*KubeResourceOverride
}

type Tracing struct {
	Provider map[string]interface{}   `json:"provider,omitempty"`
	Cluster  []map[string]interface{} `json:"cluster,omitempty"`
}

type Failover struct {
	Enabled    *bool   `json:"enabled,omitempty" desc:"(Enterprise Only): Configure this proxy for failover"`
	Port       *uint   `json:"port,omitempty" desc:"(Enterprise Only): Port to use for failover Gateway Bind port, and service. Default is 15443"`
	NodePort   *uint   `json:"nodePort,omitempty" desc:"(Enterprise Only): Optional NodePort for failover Service"`
	SecretName *string `json:"secretName,omitempty" desc:"(Enterprise Only): Secret containing downstream Ssl Secrets Default is failover-downstream"`
	*KubeResourceOverride
}

type AccessLogger struct {
	Image                        *Image                `json:"image,omitempty"`
	Port                         *uint                 `json:"port,omitempty"`
	ServiceName                  *string               `json:"serviceName,omitempty"`
	Enabled                      *bool                 `json:"enabled,omitempty"`
	Stats                        *Stats                `json:"stats,omitempty" desc:"overrides for prometheus stats published by the access logging pod"`
	RunAsUser                    *float64              `json:"runAsUser,omitempty" desc:"Explicitly set the user ID for the processes in the container to run as. Default is 10101."`
	FsGroup                      *float64              `json:"fsGroup,omitempty" desc:"Explicitly set the group ID for volume ownership. Default is 10101"`
	ExtraAccessLoggerLabels      map[string]string     `json:"extraAccessLoggerLabels,omitempty" desc:"Optional extra key-value pairs to add to the spec.template.metadata.labels data of the access logger deployment."`
	ExtraAccessLoggerAnnotations map[string]string     `json:"extraAccessLoggerAnnotations,omitempty" desc:"Optional extra key-value pairs to add to the spec.template.metadata.annotations data of the access logger deployment."`
	Service                      *KubeResourceOverride `json:"service,omitempty"`
	Deployment                   *KubeResourceOverride `json:"deployment,omitempty"`
	*DeploymentSpec
}

type Ingress struct {
	Enabled             *bool              `json:"enabled,omitempty"`
	Deployment          *IngressDeployment `json:"deployment,omitempty"`
	RequireIngressClass *bool              `json:"requireIngressClass,omitempty" desc:"only serve traffic for Ingress objects with the Ingress Class annotation 'kubernetes.io/ingress.class'. By default the annotation value must be set to 'gloo', however this can be overriden via customIngressClass."`
	CustomIngress       *bool              `json:"customIngressClass,omitempty" desc:"Only relevant when requireIngressClass is set to true. Setting this value will cause the Gloo Edge Ingress Controller to process only those Ingress objects which have their ingress class set to this value (e.g. 'kubernetes.io/ingress.class=SOMEVALUE')."`
}

type IngressDeployment struct {
	Image                   *Image            `json:"image,omitempty"`
	RunAsUser               *float64          `json:"runAsUser,omitempty" desc:"Explicitly set the user ID for the processes in the container to run as. Default is 10101."`
	FloatingUserId          *bool             `json:"floatingUserId,omitempty" desc:"If true, allows the cluster to dynamically assign a user ID for the processes running in the container."`
	ExtraIngressLabels      map[string]string `json:"extraIngressLabels,omitempty" desc:"Optional extra key-value pairs to add to the spec.template.metadata.labels data of the ingress deployment."`
	ExtraIngressAnnotations map[string]string `json:"extraIngressAnnotations,omitempty" desc:"Optional extra key-value pairs to add to the spec.template.metadata.annotations data of the ingress deployment."`
	Stats                   *bool             `json:"stats,omitempty" desc:"Controls whether or not Envoy stats are enabled"`
	*DeploymentSpec
}

type IngressProxy struct {
	Deployment      *IngressProxyDeployment `json:"deployment,omitempty"`
	ConfigMap       *ConfigMap              `json:"configMap,omitempty"`
	Tracing         *string                 `json:"tracing,omitempty"`
	LoopBackAddress *string                 `json:"loopBackAddress,omitempty" desc:"Name on which to bind the loop-back interface for this instance of Envoy. Defaults to 127.0.0.1, but other common values may be localhost or ::1"`
	Label           *string                 `json:"label,omitempty" desc:"Value for label gloo. Use a unique value to use several ingress proxy instances in the same cluster. Default is ingress-proxy"`
	*ServiceSpec
}

type IngressProxyDeployment struct {
	Image                   *Image            `json:"image,omitempty"`
	HttpPort                *int              `json:"httpPort,omitempty" desc:"HTTP port for the ingress container"`
	HttpsPort               *int              `json:"httpsPort,omitempty" desc:"HTTPS port for the ingress container"`
	ExtraPorts              []interface{}     `json:"extraPorts,omitempty"`
	ExtraAnnotations        map[string]string `json:"extraAnnotations,omitempty"`
	FloatingUserId          *bool             `json:"floatingUserId,omitempty" desc:"If true, allows the cluster to dynamically assign a user ID for the processes running in the container."`
	RunAsUser               *float64          `json:"runAsUser,omitempty" desc:"Explicitly set the user ID for the pod to run as. Default is 10101"`
	ExtraIngressProxyLabels map[string]string `json:"extraIngressProxyLabels,omitempty" desc:"Optional extra key-value pairs to add to the spec.template.metadata.labels data of the ingress proxy deployment."`
	Stats                   *bool             `json:"stats,omitempty" desc:"Controls whether or not Envoy stats are enabled"`
	*DeploymentSpec
}

type ServiceSpec struct {
	Service *Service `json:"service,omitempty" desc:"K8s service configuration"`
}

type Service struct {
	Type             *string           `json:"type,omitempty" desc:"K8s service type"`
	ExtraAnnotations map[string]string `json:"extraAnnotations,omitempty" desc:"extra annotations to add to the service"`
	LoadBalancerIP   *string           `json:"loadBalancerIP,omitempty" desc:"IP address of the load balancer"`
	HttpPort         *int              `json:"httpPort,omitempty" desc:"HTTP port for the knative/ingress proxy service"`
	HttpsPort        *int              `json:"httpsPort,omitempty" desc:"HTTPS port for the knative/ingress proxy service"`
	*KubeResourceOverride
}

type GlobalConfigMap struct {
	Name      *string           `json:"name,omitempty" desc:"Name of the ConfigMap to create (required)."`
	Namespace *string           `json:"namespace,omitempty" desc:"Namespace in which to create the ConfigMap. If empty, defaults to Gloo Edge install namespace."`
	Data      map[string]string `json:"data,omitempty" desc:"Key-value pairs of ConfigMap data."`
}

type ConfigMap struct {
	Data map[string]string `json:"data,omitempty"`
	*KubeResourceOverride
}

type K8s struct {
	ClusterName *string `json:"clusterName,omitempty" desc:"cluster name to use when referencing services."`
}

type Stats struct {
	Enabled               *bool   `json:"enabled,omitempty" desc:"Controls whether or not Envoy stats are enabled"`
	RoutePrefixRewrite    *string `json:"routePrefixRewrite,omitempty" desc:"The Envoy stats endpoint to which the metrics are written"`
	SetDatadogAnnotations *bool   `json:"setDatadogAnnotations,omitempty" desc:"Sets the default datadog annotations"`
	EnableStatsRoute      *bool   `json:"enableStatsRoute,omitempty" desc:"Enables an additional route to the stats cluster defaulting to /stats"`
	StatsPrefixRewrite    *string `json:"statsPrefixRewrite,omitempty" desc:"The Envoy stats endpoint with general metrics for the additional stats route"`
	ServiceMonitorEnabled *bool   `json:"serviceMonitorEnabled,omitempty" desc:"Whether or not to expose an http-monitoring port that can be scraped by a Prometheus Service Monitor. Requires that 'enabled' is also true"`
	PodMonitorEnabled     *bool   `json:"podMonitorEnabled,omitempty" desc:"Whether or not to expose an http-monitoring port that can be scraped by a Prometheus Pod Monitor. Requires that 'enabled' is also true"`
}

type Mtls struct {
	Enabled               *bool                 `json:"enabled,omitempty" desc:"Enables internal mtls authentication"`
	Sds                   SdsContainer          `json:"sds,omitempty"`
	EnvoySidecar          EnvoySidecarContainer `json:"envoy,omitempty"`
	IstioProxy            IstioProxyContainer   `json:"istioProxy,omitempty" desc:"Istio-proxy container"`
	EnvoySidecarResources *ResourceRequirements `json:"envoySidecarResources,omitempty" desc:"Sets default resource requirements for all Envoy sidecar containers."`
	SdsResources          *ResourceRequirements `json:"sdsResources,omitempty" desc:"Sets default resource requirements for all sds containers."`
}

type SdsContainer struct {
	Image           *Image           `json:"image,omitempty"`
	SecurityContext *SecurityContext `json:"securityContext,omitempty" desc:"securityContext for sds gloo deployment container. If this is defined it supercedes any values set in FloatingUserId, RunAsUser, DisableNetBind, RunUnprivileged. See https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.26/#securitycontext-v1-core for details."`
}

type EnvoySidecarContainer struct {
	Image           *Image           `json:"image,omitempty"`
	SecurityContext *SecurityContext `json:"securityContext,omitempty" desc:"securityContext for envoy-sidecar gloo deployment container. If this is defined it supercedes any values set in FloatingUserId, RunAsUser, DisableNetBind, RunUnprivileged. See [security context](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.26/#securitycontext-v1-core) for details."`
}

type IstioProxyContainer struct {
	Image           *Image           `json:"image,omitempty" desc:"Istio-proxy image to use for mTLS"`
	SecurityContext *SecurityContext `json:"securityContext,omitempty" desc:"securityContext for istio-proxy deployment container. If this is defined it supercedes any values set in FloatingUserId, RunAsUser, DisableNetBind, RunUnprivileged. See https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.26/#securitycontext-v1-core for details."`
}

type IstioSDS struct {
	Enabled        *bool         `json:"enabled,omitempty" desc:"Enables SDS cert-rotator sidecar for istio mTLS cert rotation"`
	CustomSidecars []interface{} `json:"customSidecars,omitempty" desc:"Override the default Istio sidecar in gateway-proxy with a custom container. Ignored if IstioSDS.enabled is false"`
}

type IstioIntegration struct {
	LabelInstallNamespace       *bool   `json:"labelInstallNamespace,omitempty" desc:"If creating a namespace for Gloo, include the 'istio-injection: enabled' label (or 'istio.io/rev=' if 'istioSidecarRevTag' field is also set) to allow Istio sidecar injection for Gloo pods. Be aware that Istio's default injection behavior will auto-inject a sidecar into all pods in such a marked namespace. Disabling this behavior in Istio's configs or using gloo's global.istioIntegration.disableAutoinjection flag is recommended."`
	WhitelistDiscovery          *bool   `json:"whitelistDiscovery,omitempty" desc:"Annotate the discovery pod for Istio sidecar injection to ensure that it gets a sidecar even when namespace-wide auto-injection is disabled. Generally only needed for FDS is enabled."`
	DisableAutoinjection        *bool   `json:"disableAutoinjection,omitempty" desc:"Annotate all pods (excluding those whitelisted by other config values) to with an explicit 'do not inject' annotation to prevent Istio from adding sidecars to all pods. It's recommended that this be set to true, as some pods do not immediately work with an Istio sidecar without extra manual configuration."`
	EnableIstioSidecarOnGateway *bool   `json:"enableIstioSidecarOnGateway,omitempty" desc:"Enable Istio sidecar injection on the gateway-proxy deployment. Ignored if LabelInstallNamespace is not 'true'. Ignored if DisableAutoInjection is 'true'."`
	IstioSidecarRevTag          *string `json:"istioSidecarRevTag,omitempty" desc:"Value of revision tag for Istio sidecar injection on the gateway-proxy and discovery deployments (when enabled with LabelInstallNamespace, WhitelistDiscovery or EnableIstioSidecarOnGateway). If set, applies the label 'istio.io/rev:<rev>' instead of 'sidecar.istio.io/inject' or 'istio-injection:enabled'. Ignored if DisableAutoInjection is 'true'."`
}

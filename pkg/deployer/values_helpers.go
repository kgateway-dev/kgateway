package deployer

import (
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"slices"
	"strings"

	istioslices "istio.io/istio/pkg/slices"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/translator/listener"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/validate"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

var (
	// ErrMultipleAddresses is returned when multiple addresses are specified in Gateway.spec.addresses
	ErrMultipleAddresses = errors.New("multiple addresses given, only one address is supported")

	// ErrNoValidIPAddress is returned when no valid IP address is found in Gateway.spec.addresses
	ErrNoValidIPAddress = errors.New("IP address in Gateway.spec.addresses not valid")
)

// This file contains helper functions that generate helm values in the format needed
// by the deployer.

var ComponentLogLevelEmptyError = func(key string, value string) error {
	return fmt.Errorf("an empty key or value was provided in componentLogLevels: key=%s, value=%s", key, value)
}

// Extract the listener ports from a Gateway and corresponding listener sets. These will be used to populate:
// 1. the ports exposed on the envoy container
// 2. the ports exposed on the proxy service
func GetPortsValues(gw *ir.GatewayForDeployer, gwp *kgateway.GatewayParameters) []HelmPort {
	gwPorts := []HelmPort{}

	// Add ports from Gateway listeners
	for _, port := range gw.Ports.List() {
		portName := listener.GenerateListenerNameFromPort(port)
		if err := validate.ListenerPort(ir.Listener{Listener: gwv1.Listener{Port: port}}, port); err != nil {
			// skip invalid ports; statuses are handled in the translator
			logger.Error("skipping port", "gateway", gw.ResourceName(), "error", err)
			continue
		}
		gwPorts = AppendPortValue(gwPorts, port, portName, gwp)
	}

	// Add ports from GatewayParameters.Service.Ports
	// Merge user-defined service ports with auto-generated listener ports
	// Without this, user-specified ports would be ignored, causing service connectivity issues
	if gwp != nil && gwp.Spec.GetKube() != nil && gwp.Spec.GetKube().GetService() != nil {
		servicePorts := gwp.Spec.GetKube().GetService().GetPorts()
		for _, servicePort := range servicePorts {
			portValue := servicePort.GetPort()
			l := ir.Listener{
				Listener: gwv1.Listener{
					Port: gwv1.PortNumber(portValue),
				},
			}
			portName := listener.GenerateListenerName(l)
			gwPorts = AppendPortValue(gwPorts, portValue, portName, gwp)
		}
	}

	return gwPorts
}

func SanitizePortName(name string) string {
	nonAlphanumericRegex := regexp.MustCompile(`[^a-zA-Z0-9-]+`)
	str := nonAlphanumericRegex.ReplaceAllString(name, "-")
	doubleHyphen := regexp.MustCompile(`-{2,}`)
	str = doubleHyphen.ReplaceAllString(str, "-")

	// This is a kubernetes spec requirement.
	maxPortNameLength := 15
	if len(str) > maxPortNameLength {
		str = str[:maxPortNameLength]
	}
	return str
}

func AppendPortValue(gwPorts []HelmPort, port int32, name string, gwp *kgateway.GatewayParameters) []HelmPort {
	if istioslices.IndexFunc(gwPorts, func(p HelmPort) bool { return *p.Port == port }) != -1 {
		return gwPorts
	}

	portName := SanitizePortName(name)
	protocol := "TCP"

	// Search for static NodePort set from the GatewayParameters spec.
	// NodePort and LoadBalancer both support explicit node ports; if not set, nil renders nothing.
	var nodePort *int32 = nil
	if gwp != nil && gwp.Spec.GetKube().GetService().GetType() != nil {
		serviceType := *(gwp.Spec.GetKube().GetService().GetType())
		if serviceType == corev1.ServiceTypeNodePort || serviceType == corev1.ServiceTypeLoadBalancer {
			if idx := istioslices.IndexFunc(gwp.Spec.GetKube().GetService().GetPorts(), func(p kgateway.Port) bool {
				return p.GetPort() == port
			}); idx != -1 {
				nodePort = gwp.Spec.GetKube().GetService().GetPorts()[idx].GetNodePort()
			}
		}
	}
	return append(gwPorts, HelmPort{
		Port:       &port,
		TargetPort: &port,
		Name:       &portName,
		Protocol:   &protocol,
		NodePort:   nodePort,
	})
}

// Convert service values from GatewayParameters into helm values to be used by the deployer.
func GetServiceValues(svcConfig *kgateway.Service) *HelmService {
	// convert the service type enum to its string representation;
	// if type is not set, it will default to 0 ("ClusterIP")
	var svcType *string
	var clusterIP *string
	var extraAnnotations map[string]string
	var extraLabels map[string]string
	var externalTrafficPolicy *string
	var loadBalancerClass *string
	var loadBalancerSourceRanges []string

	if svcConfig != nil {
		if svcConfig.GetType() != nil {
			svcType = new(string(*svcConfig.GetType()))
		}
		clusterIP = svcConfig.GetClusterIP()
		extraAnnotations = svcConfig.GetExtraAnnotations()
		extraLabels = svcConfig.GetExtraLabels()
		externalTrafficPolicy = svcConfig.GetExternalTrafficPolicy()
		loadBalancerClass = svcConfig.GetLoadBalancerClass()
		loadBalancerSourceRanges = svcConfig.GetLoadBalancerSourceRanges()
	}

	return &HelmService{
		Type:                     svcType,
		ClusterIP:                clusterIP,
		ExtraAnnotations:         extraAnnotations,
		ExtraLabels:              extraLabels,
		ExternalTrafficPolicy:    externalTrafficPolicy,
		LoadBalancerClass:        loadBalancerClass,
		LoadBalancerSourceRanges: loadBalancerSourceRanges,
	}
}

// GetLoadBalancerIPFromGatewayAddresses extracts the IP address from Gateway.spec.addresses.
// Returns the IP address if exactly one valid IP address is found, nil if no addresses are specified,
// or an error if more than one address is specified or no valid IP address is found.
func GetLoadBalancerIPFromGatewayAddresses(gw *gwv1.Gateway) (*string, error) {
	ipAddresses := istioslices.MapFilter(gw.Spec.Addresses, func(addr gwv1.GatewaySpecAddress) *string {
		if addr.Type == nil || *addr.Type == gwv1.IPAddressType {
			return &addr.Value
		}
		return nil
	})

	if len(ipAddresses) == 0 && len(gw.Spec.Addresses) != 0 {
		return nil, ErrNoValidIPAddress
	}

	if len(ipAddresses) == 0 {
		return nil, nil
	}
	if len(ipAddresses) > 1 {
		return nil, fmt.Errorf("%w: gateway %s/%s has %d addresses", ErrMultipleAddresses, gw.Namespace, gw.Name, len(gw.Spec.Addresses))
	}

	addr := ipAddresses[0]

	// Validate IP format
	parsedIP, err := netip.ParseAddr(addr)
	if err == nil && parsedIP.IsValid() {
		return &addr, nil
	}
	return nil, ErrNoValidIPAddress
}

// SetLoadBalancerIPFromGateway extracts the IP address from Gateway.spec.addresses
// and sets it on the HelmService if the service type is LoadBalancer.
// Only sets the IP if exactly one valid IP address is found in Gateway.spec.addresses.
// Returns an error if more than one address is specified or no valid IP address is found.
func SetLoadBalancerIPFromGateway(gw *gwv1.Gateway, svc *HelmService) error {
	// Only extract IP if service type is LoadBalancer
	if svc.Type == nil || *svc.Type != string(corev1.ServiceTypeLoadBalancer) {
		return nil
	}

	ip, err := GetLoadBalancerIPFromGatewayAddresses(gw)
	if err != nil {
		return err
	}
	if ip != nil {
		svc.LoadBalancerIP = ip
	}
	return nil
}

// Convert service account values from GatewayParameters into helm values to be used by the deployer.
func GetServiceAccountValues(svcAccountConfig *kgateway.ServiceAccount) *HelmServiceAccount {
	return &HelmServiceAccount{
		ExtraAnnotations: svcAccountConfig.GetExtraAnnotations(),
		ExtraLabels:      svcAccountConfig.GetExtraLabels(),
	}
}

// Convert sds values from GatewayParameters into helm values to be used by the deployer.
func GetSdsContainerValues(sdsContainerConfig *kgateway.SdsContainer) *HelmSdsContainer {
	if sdsContainerConfig == nil {
		return nil
	}

	vals := &HelmSdsContainer{
		Image:           GetImageValues(sdsContainerConfig.GetImage()),
		Resources:       sdsContainerConfig.GetResources(),
		SecurityContext: sdsContainerConfig.GetSecurityContext(),
		SdsBootstrap:    &SdsBootstrap{},
	}

	if bootstrap := sdsContainerConfig.GetBootstrap(); bootstrap != nil {
		vals.SdsBootstrap = &SdsBootstrap{
			LogLevel: bootstrap.GetLogLevel(),
		}
	}

	return vals
}

func GetIstioContainerValues(config *kgateway.IstioContainer) *HelmIstioContainer {
	if config == nil {
		return nil
	}

	return &HelmIstioContainer{
		Image:                 GetImageValues(config.GetImage()),
		LogLevel:              config.GetLogLevel(),
		Resources:             config.GetResources(),
		SecurityContext:       config.GetSecurityContext(),
		IstioDiscoveryAddress: config.GetIstioDiscoveryAddress(),
		IstioMetaMeshId:       config.GetIstioMetaMeshId(),
		IstioMetaClusterId:    config.GetIstioMetaClusterId(),
	}
}

// Convert istio values from GatewayParameters into helm values to be used by the deployer.
func GetIstioValues(istioIntegrationEnabled bool, istioConfig *kgateway.IstioIntegration) *HelmIstio {
	// if istioConfig is nil, istio sds is disabled and values can be ignored
	if istioConfig == nil {
		return &HelmIstio{
			Enabled: new(istioIntegrationEnabled),
		}
	}

	return &HelmIstio{
		Enabled: new(istioIntegrationEnabled),
	}
}

// Get the image values for the envoy container in the proxy deployment.
func GetImageValues(image *kgateway.Image) *HelmImage {
	if image == nil {
		return &HelmImage{}
	}

	HelmImage := &HelmImage{
		Registry:   image.GetRegistry(),
		Repository: image.GetRepository(),
		Tag:        image.GetTag(),
		Digest:     image.GetDigest(),
	}
	if image.GetPullPolicy() != nil {
		HelmImage.PullPolicy = new(string(*image.GetPullPolicy()))
	}

	return HelmImage
}

// Get the stats values for the envoy listener in the configmap for bootstrap.
func GetStatsValues(statsConfig *kgateway.StatsConfig) *HelmStatsConfig {
	if statsConfig == nil {
		return nil
	}
	vals := &HelmStatsConfig{
		Enabled:            statsConfig.GetEnabled(),
		RoutePrefixRewrite: statsConfig.GetRoutePrefixRewrite(),
		EnableStatsRoute:   statsConfig.GetEnableStatsRoute(),
		StatsPrefixRewrite: statsConfig.GetStatsRoutePrefixRewrite(),
	}

	if m := statsConfig.GetMatcher(); m != nil {
		hm := &HelmStatsMatcher{}
		if incl := m.GetInclusionList(); len(incl) > 0 {
			hm.InclusionList = toHelmStringMatcher(incl)
		} else if excl := m.GetExclusionList(); len(excl) > 0 {
			hm.ExclusionList = toHelmStringMatcher(excl)
		}
		vals.Matcher = hm
	}

	return vals
}

func toHelmStringMatcher(l []shared.StringMatcher) []HelmStringMatcher {
	out := make([]HelmStringMatcher, 0, len(l))
	for _, sm := range l {
		out = append(out, HelmStringMatcher{
			Exact:      sm.Exact,
			Prefix:     sm.Prefix,
			Suffix:     sm.Suffix,
			Contains:   sm.Contains,
			SafeRegex:  sm.SafeRegex,
			IgnoreCase: sm.IgnoreCase,
		})
	}
	return out
}

// ApplyKubernetesProxyConfigValues maps every field of a KubernetesProxyConfig onto the
// gateway helm values. gateway.Ports must already be populated, since the security context
// defaulting depends on whether privileged ports are in use, and cfg may be mutated by that
// defaulting. Values derived from the Gateway or the environment (ports, xds, gateway
// labels/annotations, load-balancer IP) remain the caller's responsibility.
func ApplyKubernetesProxyConfigValues(gateway *HelmGateway, cfg *kgateway.KubernetesProxyConfig, istioAutoMtlsEnabled bool) error {
	// The security contexts may need to be updated if privileged ports are used.
	// This may affect both the PodSecurityContext and the SecurityContexts for the containers defined in cfg.
	// Note: this call may populate the PodSecurityContext and SecurityContext fields in cfg if they are null,
	// so this needs to happen before those fields are extracted to local variables.
	UpdateSecurityContexts(cfg, gateway.Ports)

	deployConfig := cfg.GetDeployment()
	podConfig := cfg.GetPodTemplate()
	envoyContainerConfig := cfg.GetEnvoyContainer()
	svcConfig := cfg.GetService()
	svcAccountConfig := cfg.GetServiceAccount()
	istioConfig := cfg.GetIstio()

	sdsContainerConfig := cfg.GetSdsContainer()
	statsConfig := cfg.GetStats()
	istioContainerConfig := istioConfig.GetIstioProxyContainer()

	// deployment values
	if deployConfig.GetReplicas() != nil {
		gateway.ReplicaCount = new(uint32(*deployConfig.GetReplicas())) // nolint:gosec // G115: kubebuilder validation ensures safe for uint32
	}
	gateway.Strategy = deployConfig.GetStrategy()

	// service values
	gateway.Service = GetServiceValues(svcConfig)
	// serviceaccount values
	gateway.ServiceAccount = GetServiceAccountValues(svcAccountConfig)
	// pod template values
	gateway.ExtraPodAnnotations = podConfig.GetExtraAnnotations()
	gateway.ExtraPodLabels = podConfig.GetExtraLabels()
	gateway.ImagePullSecrets = podConfig.GetImagePullSecrets()
	gateway.PodSecurityContext = podConfig.GetSecurityContext()
	gateway.NodeSelector = podConfig.GetNodeSelector()
	gateway.Affinity = podConfig.GetAffinity()
	gateway.Tolerations = podConfig.GetTolerations()
	gateway.StartupProbe = podConfig.GetStartupProbe()
	gateway.ReadinessProbe = podConfig.GetReadinessProbe()
	gateway.LivenessProbe = podConfig.GetLivenessProbe()
	gateway.GracefulShutdown = podConfig.GetGracefulShutdown()
	gateway.TerminationGracePeriodSeconds = podConfig.GetTerminationGracePeriodSeconds()
	gateway.TopologySpreadConstraints = podConfig.GetTopologySpreadConstraints()
	gateway.ExtraVolumes = podConfig.GetExtraVolumes()
	gateway.PriorityClassName = podConfig.GetPriorityClassName()

	gateway.DataPlaneType = DataPlaneEnvoy
	gateway.LogFormat = envoyContainerConfig.GetBootstrap().GetLogFormat()
	gateway.LogLevel = envoyContainerConfig.GetBootstrap().GetLogLevel()
	compLogLevels := envoyContainerConfig.GetBootstrap().GetComponentLogLevels()
	compLogLevelStr, err := ComponentLogLevelsToString(compLogLevels)
	if err != nil {
		return err
	}
	gateway.ComponentLogLevel = &compLogLevelStr

	// Extract DNS resolver configuration
	dnsResolverConfig := envoyContainerConfig.GetBootstrap().GetDnsResolver()
	if dnsResolverConfig != nil {
		var udpMaxQueries *int32
		if maybeMaxQ := ptr.Deref(dnsResolverConfig.GetUdpMaxQueries(), 0); maybeMaxQ > 0 {
			udpMaxQueries = &maybeMaxQ
		}
		gateway.DnsResolver = &HelmDnsResolver{
			UdpMaxQueries: udpMaxQueries,
		}
	}

	gateway.EnableReadinessProbeProxyProtocol = envoyContainerConfig.GetBootstrap().GetEnableReadinessProbeProxyProtocol()

	gateway.Resources = envoyContainerConfig.GetResources()
	gateway.SecurityContext = envoyContainerConfig.GetSecurityContext()
	gateway.Image = GetImageValues(envoyContainerConfig.GetImage())
	gateway.ExtraArgs = envoyContainerConfig.GetExtraArgs()
	gateway.Env = envoyContainerConfig.GetEnv()
	gateway.ExtraVolumeMounts = envoyContainerConfig.ExtraVolumeMounts

	// istio values
	gateway.Istio = GetIstioValues(istioAutoMtlsEnabled, istioConfig)
	gateway.SdsContainer = GetSdsContainerValues(sdsContainerConfig)
	gateway.IstioContainer = GetIstioContainerValues(istioContainerConfig)

	gateway.Stats = GetStatsValues(statsConfig)

	return nil
}

// ComponentLogLevelsToString converts the key-value pairs in the map into a string of the
// format: key1:value1,key2:value2,key3:value3, where the keys are sorted alphabetically.
// If an empty map is passed in, then an empty string is returned.
// Map keys and values may not be empty.
// No other validation is currently done on the keys/values.
func ComponentLogLevelsToString(vals map[string]string) (string, error) {
	if len(vals) == 0 {
		return "", nil
	}

	parts := make([]string, 0, len(vals))
	for k, v := range vals {
		if k == "" || v == "" {
			return "", ComponentLogLevelEmptyError(k, v)
		}
		parts = append(parts, fmt.Sprintf("%s:%s", k, v))
	}
	slices.Sort(parts)
	return strings.Join(parts, ","), nil
}

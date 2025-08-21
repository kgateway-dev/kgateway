package deployer

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/exp/slices"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/translator/listener"
)

// This file contains helper functions that generate helm values in the format needed
// by the deployer.

var ComponentLogLevelEmptyError = func(key string, value string) error {
	return fmt.Errorf("an empty key or value was provided in componentLogLevels: key=%s, value=%s", key, value)
}

// Extract the listener ports from a Gateway and corresponding listener sets. These will be used to populate:
// 1. the ports exposed on the envoy container
// 2. the ports exposed on the proxy service
func GetPortsValues(gw *ir.Gateway, gwp *v1alpha1.GatewayParameters) []HelmPort {
	gwPorts := []HelmPort{}

	// Add ports from Gateway listeners
	for _, l := range gw.Listeners {
		listenerPort := uint16(l.Port)
		portName := listener.GenerateListenerName(l)
		gwPorts = AppendPortValue(gwPorts, listenerPort, portName, gwp)
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

func AppendPortValue(gwPorts []HelmPort, port uint16, name string, gwp *v1alpha1.GatewayParameters) []HelmPort {
	if slices.IndexFunc(gwPorts, func(p HelmPort) bool { return *p.Port == port }) != -1 {
		return gwPorts
	}

	portName := SanitizePortName(name)
	protocol := "TCP"

	// Search for static NodePort set from the GatewayParameters spec
	// If not found the default value of `nil` will not render anything.
	var nodePort *uint16 = nil
	if gwp.Spec.GetKube().GetService().GetType() != nil && *(gwp.Spec.GetKube().GetService().GetType()) == corev1.ServiceTypeNodePort {
		if idx := slices.IndexFunc(gwp.Spec.GetKube().GetService().GetPorts(), func(p v1alpha1.Port) bool {
			return p.GetPort() == uint16(port)
		}); idx != -1 {
			nodePort = ptr.To(uint16(*gwp.Spec.GetKube().GetService().GetPorts()[idx].GetNodePort()))
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
func GetServiceValues(svcConfig *v1alpha1.Service) *HelmService {
	// convert the service type enum to its string representation;
	// if type is not set, it will default to 0 ("ClusterIP")
	var svcType *string
	if svcConfig.GetType() != nil {
		svcType = ptr.To(string(*svcConfig.GetType()))
	}
	return &HelmService{
		Type:             svcType,
		ClusterIP:        svcConfig.GetClusterIP(),
		ExtraAnnotations: svcConfig.GetExtraAnnotations(),
		ExtraLabels:      svcConfig.GetExtraLabels(),
	}
}

// Convert service account values from GatewayParameters into helm values to be used by the deployer.
func GetServiceAccountValues(svcAccountConfig *v1alpha1.ServiceAccount) *HelmServiceAccount {
	return &HelmServiceAccount{
		ExtraAnnotations: svcAccountConfig.GetExtraAnnotations(),
		ExtraLabels:      svcAccountConfig.GetExtraLabels(),
	}
}

// Convert sds values from GatewayParameters into helm values to be used by the deployer.
func GetSdsContainerValues(sdsContainerConfig *v1alpha1.SdsContainer) *HelmSdsContainer {
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

func GetIstioContainerValues(config *v1alpha1.IstioContainer) *HelmIstioContainer {
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
func GetIstioValues(istioIntegrationEnabled bool, istioConfig *v1alpha1.IstioIntegration) *HelmIstio {
	// if istioConfig is nil, istio sds is disabled and values can be ignored
	if istioConfig == nil {
		return &HelmIstio{
			Enabled: ptr.To(istioIntegrationEnabled),
		}
	}

	return &HelmIstio{
		Enabled: ptr.To(istioIntegrationEnabled),
	}
}

// Get the image values for the envoy container in the proxy deployment.
func GetImageValues(image *v1alpha1.Image) *HelmImage {
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
		HelmImage.PullPolicy = ptr.To(string(*image.GetPullPolicy()))
	}

	return HelmImage
}

// Get the stats values for the envoy listener in the configmap for bootstrap.
func GetStatsValues(statsConfig *v1alpha1.StatsConfig) *HelmStatsConfig {
	if statsConfig == nil {
		return nil
	}
	return &HelmStatsConfig{
		Enabled:            statsConfig.GetEnabled(),
		RoutePrefixRewrite: statsConfig.GetRoutePrefixRewrite(),
		EnableStatsRoute:   statsConfig.GetEnableStatsRoute(),
		StatsPrefixRewrite: statsConfig.GetStatsRoutePrefixRewrite(),
	}
}

func getTracingValues(tracingConfig *v1alpha1.AiExtensionTrace) *helmAITracing {
	if tracingConfig == nil {
		return nil
	}
	return &helmAITracing{
		EndPoint: tracingConfig.EndPoint,
		Sampler: &helmAITracingSampler{
			SamplerType: tracingConfig.GetSamplerType(),
			SamplerArg:  tracingConfig.GetSamplerArg(),
		},
		Timeout:  tracingConfig.GetTimeout(),
		Protocol: tracingConfig.GetOTLPProtocolType(),
	}
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
	sort.Strings(parts)
	return strings.Join(parts, ","), nil
}

func GetAIExtensionValues(config *v1alpha1.AiExtension) (*HelmAIExtension, error) {
	if config == nil {
		return nil, nil
	}

	// If we don't do this check, a byte array containing the characters "null" will be rendered
	// This will not be marshallable by the component so instead we render nothing.
	var byt []byte
	if config.GetStats() != nil {
		var err error
		byt, err = json.Marshal(config.GetStats())
		if err != nil {
			return nil, err
		}
	}

	// Handle Tracing with base64 encoding
	var tracingBase64 string
	if config.Tracing != nil {
		// Convert tracing config to JSON
		tracingJSON, err := json.Marshal(getTracingValues(config.Tracing))
		if err != nil {
			return nil, fmt.Errorf("failed to marshal tracing config: %w", err)
		}

		// Encode JSON to base64
		tracingBase64 = base64.StdEncoding.EncodeToString(tracingJSON)
	}

	return &HelmAIExtension{
		Enabled:         *config.GetEnabled(),
		Image:           GetImageValues(config.GetImage()),
		SecurityContext: config.GetSecurityContext(),
		Resources:       config.GetResources(),
		Env:             config.GetEnv(),
		Ports:           config.GetPorts(),
		Stats:           byt,
		Tracing:         tracingBase64,
	}, nil
}

func GetAgentGatewayValues(config *v1alpha1.AgentGateway) (*HelmAgentGateway, error) {
	if config == nil {
		return nil, nil
	}

	var logLevel string
	if config.GetLogLevel() != nil {
		logLevel = *config.GetLogLevel()
	}

	var customConfigMapName string
	if config.GetCustomConfigMapName() != nil {
		customConfigMapName = *config.GetCustomConfigMapName()
	}

	return &HelmAgentGateway{
		Enabled:             *config.GetEnabled(),
		LogLevel:            logLevel,
		CustomConfigMapName: customConfigMapName,
	}, nil
}

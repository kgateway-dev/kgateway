package deployer

import (
	"fmt"
	"sort"
	"strings"

	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/projects/gateway2/api/v1alpha1"
	"github.com/solo-io/gloo/projects/gateway2/ports"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"golang.org/x/exp/slices"
	"k8s.io/utils/ptr"
	api "sigs.k8s.io/gateway-api/apis/v1"
)

// This file contains helper functions that generate helm values in the format needed
// by the deployer.

var ComponentLogLevelEmptyError = func(key string, value string) error {
	return eris.Errorf("an empty key or value was provided in componentLogLevels: key=%s, value=%s", key, value)
}

// Extract the listener ports from a Gateway. These will be used to populate:
// 1. the ports exposed on the envoy container
// 2. the ports exposed on the proxy service
func getPortsValues(gw *api.Gateway) []helmPort {
	gwPorts := []helmPort{}
	for _, l := range gw.Spec.Listeners {
		listenerPort := uint16(l.Port)

		// only process this port if we haven't already processed a listener with the same port
		if slices.IndexFunc(gwPorts, func(p helmPort) bool { return *p.Port == listenerPort }) != -1 {
			continue
		}

		targetPort := ports.TranslatePort(listenerPort)
		portName := string(l.Name)
		protocol := "TCP"

		gwPorts = append(gwPorts, helmPort{
			Port:       &listenerPort,
			TargetPort: &targetPort,
			Name:       &portName,
			Protocol:   &protocol,
		})
	}
	return gwPorts
}

// TODO: Removing until autoscaling is re-added.
// See: https://github.com/solo-io/solo-projects/issues/5948
// Convert autoscaling values from GatewayParameters into helm values to be used by the deployer.
// func getAutoscalingValues(autoscaling *v1.Autoscaling) *helmAutoscaling {
// 	hpaConfig := autoscaling.HorizontalPodAutoscaler
// 	if hpaConfig == nil {
// 		return nil
// 	}

// 	trueVal := true
// 	autoscalingVals := &helmAutoscaling{
// 		Enabled: &trueVal,
// 	}
// 	autoscalingVals.MinReplicas = hpaConfig.MinReplicas
// 	autoscalingVals.MaxReplicas = hpaConfig.MaxReplicas
// 	autoscalingVals.TargetCPUUtilizationPercentage = hpaConfig.TargetCpuUtilizationPercentage
// 	autoscalingVals.TargetMemoryUtilizationPercentage = hpaConfig.TargetMemoryUtilizationPercentage

// 	return autoscalingVals
// }

// Convert service values from GatewayParameters into helm values to be used by the deployer.
func getServiceValues(svcConfig *v1alpha1.Service) *helmService {
	// convert the service type enum to its string representation;
	// if type is not set, it will default to 0 ("ClusterIP")
	var svcType *string
	if svcConfig.GetType() != nil {
		svcType = ptr.To(string(*svcConfig.GetType()))
	}
	return &helmService{
		Type:             svcType,
		ClusterIP:        svcConfig.GetClusterIP(),
		ExtraAnnotations: svcConfig.GetExtraAnnotations(),
		ExtraLabels:      svcConfig.GetExtraLabels(),
	}
}

// Convert sds values from GatewayParameters into helm values to be used by the deployer.
func getSdsContainerValues(sdsContainerConfig *v1alpha1.SdsContainer) *helmSdsContainer {
	if sdsContainerConfig == nil {
		return nil
	}

	vals := &helmSdsContainer{
		Image:           getImageValues(sdsContainerConfig.GetImage()),
		Resources:       sdsContainerConfig.GetResources(),
		SecurityContext: sdsContainerConfig.GetSecurityContext(),
		SdsBootstrap:    &sdsBootstrap{},
	}

	if bootstrap := sdsContainerConfig.GetBootstrap(); bootstrap != nil {
		vals.SdsBootstrap = &sdsBootstrap{
			LogLevel: bootstrap.GetLogLevel(),
		}
	}

	return vals
}

func getIstioContainerValues(config *v1alpha1.IstioContainer) *helmIstioContainer {
	if config == nil {
		return nil
	}

	return &helmIstioContainer{
		Image:                 getImageValues(config.GetImage()),
		LogLevel:              config.GetLogLevel(),
		Resources:             config.GetResources(),
		SecurityContext:       config.GetSecurityContext(),
		IstioDiscoveryAddress: config.GetIstioDiscoveryAddress(),
		IstioMetaMeshId:       config.GetIstioMetaMeshId(),
		IstioMetaClusterId:    config.GetIstioMetaClusterId(),
	}
}

// Convert istio values from GatewayParameters into helm values to be used by the deployer.
func getIstioValues(istioValues bootstrap.IstioValues, istioConfig *v1alpha1.IstioIntegration) *helmIstio {
	// if istioConfig is nil, istio sds is disabled and values can be ignored
	if istioConfig == nil {
		return &helmIstio{
			Enabled: ptr.To(istioValues.IntegrationEnabled),
		}
	}

	return &helmIstio{
		Enabled: ptr.To(istioValues.IntegrationEnabled),
	}
}

// Get the image values for the envoy container in the proxy deployment.
func getImageValues(image *v1alpha1.Image) *helmImage {
	if image == nil {
		return &helmImage{}
	}

	helmImage := &helmImage{
		Registry:   image.GetRegistry(),
		Repository: image.GetRepository(),
		Tag:        image.GetTag(),
		Digest:     image.GetDigest(),
	}
	if image.GetPullPolicy() != nil {
		helmImage.PullPolicy = ptr.To(string(*image.GetPullPolicy()))
	}

	return helmImage
}

// Get the stats values for the envoy listener in the configmap for bootstrap.
func getStatsValues(statsConfig *v1alpha1.StatsConfig) *helmStatsConfig {
	if statsConfig == nil {
		return nil
	}
	return &helmStatsConfig{
		Enabled:            statsConfig.GetEnabled(),
		RoutePrefixRewrite: statsConfig.GetRoutePrefixRewrite(),
		EnableStatsRoute:   statsConfig.GetEnableStatsRoute(),
		StatsPrefixRewrite: statsConfig.GetStatsRoutePrefixRewrite(),
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

func getAIExtensionValues(config *v1alpha1.AiExtension) *helmAIExtension {
	if config == nil {
		return nil
	}

	return &helmAIExtension{
		Enabled:         *config.GetEnabled(),
		Image:           getImageValues(config.GetImage()),
		SecurityContext: config.GetSecurityContext(),
		Resources:       config.GetResources(),
		Env:             config.GetEnv(),
		Ports:           config.GetPorts(),
	}
}

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
	corev1 "k8s.io/api/core/v1"
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
	svcType := string(svcConfig.Type)
	clusterIp := svcConfig.ClusterIP
	return &helmService{
		Type:             &svcType,
		ClusterIP:        &clusterIp,
		ExtraAnnotations: svcConfig.ExtraAnnotations,
		ExtraLabels:      svcConfig.ExtraLabels,
	}
}

// Convert sds values from GatewayParameters into helm values to be used by the deployer.
func getSdsContainerValues(sdsContainerConfig *v1alpha1.SdsContainer) *helmSdsContainer {
	if sdsContainerConfig == nil {
		return nil
	}

	vals := &helmSdsContainer{
		Image:           getImageValues(sdsContainerConfig.Image),
		Resources:       sdsContainerConfig.Resources,
		SecurityContext: sdsContainerConfig.SecurityContext,
		SdsBootstrap:    &sdsBootstrap{},
	}

	if bootstrap := sdsContainerConfig.Bootstrap; bootstrap != nil {
		vals.SdsBootstrap = &sdsBootstrap{
			LogLevel: bootstrap.LogLevel,
		}
	}

	return vals
}

func getIstioContainerValues(config *v1alpha1.IstioContainer) *helmIstioContainer {
	if config == nil {
		return nil
	}

	return &helmIstioContainer{
		Image:                 getImageValues(config.Image),
		LogLevel:              config.LogLevel,
		Resources:             config.Resources,
		SecurityContext:       config.SecurityContext,
		IstioDiscoveryAddress: config.IstioDiscoveryAddress,
		IstioMetaMeshId:       config.IstioMetaMeshId,
		IstioMetaClusterId:    config.IstioMetaClusterId,
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
		Registry:   ptr.To(image.Registry),
		Repository: ptr.To(image.Repository),
		Tag:        ptr.To(image.Tag),
		Digest:     ptr.To(image.Digest),
	}
	setPullPolicy(image.PullPolicy, helmImage)
	return helmImage
}

// Get the stats values for the envoy listener in the configmap for bootstrap.
func getStatsValues(statsConfig *v1alpha1.StatsConfig) *helmStatsConfig {
	if statsConfig == nil {
		return nil
	}
	return &helmStatsConfig{
		Enabled:            statsConfig.Enabled,
		RoutePrefixRewrite: statsConfig.RoutePrefixRewrite,
		EnableStatsRoute:   statsConfig.EnableStatsRoute,
		StatsPrefixRewrite: statsConfig.StatsRoutePrefixRewrite,
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

func setPullPolicy(pullPolicy corev1.PullPolicy, helmImage *helmImage) {
	helmImage.PullPolicy = ptr.To(string(pullPolicy))
}

func getAIExtensionValues(config *v1alpha1.AiExtension) *helmAIExtension {
	if config == nil {
		return nil
	}

	return &helmAIExtension{
		Enabled:         *config.Enabled,
		Image:           getImageValues(config.Image),
		SecurityContext: config.SecurityContext,
		Resources:       config.Resources,
		Env:             config.Env,
		Ports:           config.Ports,
	}
}

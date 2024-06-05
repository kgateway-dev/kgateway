package deployer

import (
	"fmt"
	"sort"
	"strings"

	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/projects/gateway2/pkg/api/gateway.gloo.solo.io/v1alpha1"
	v1alpha1kube "github.com/solo-io/gloo/projects/gateway2/pkg/api/gateway.gloo.solo.io/v1alpha1/kube"
	"github.com/solo-io/gloo/projects/gateway2/ports"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"golang.org/x/exp/slices"
	"k8s.io/utils/ptr"
	api "sigs.k8s.io/gateway-api/apis/v1"
)

// This file contains helper functions that generate helm values in the format needed
// by the deployer.

var (
	ComponentLogLevelEmptyError = func(key string, value string) error {
		return eris.Errorf("an empty key or value was provided in componentLogLevels: key=%s, value=%s", key, value)
	}
)

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

// Convert autoscaling values from GatewayParameters into helm values to be used by the deployer.
func getAutoscalingValues(autoscaling *v1alpha1kube.Autoscaling) *helmAutoscaling {
	hpaConfig := autoscaling.GetHorizontalPodAutoscaler()
	if hpaConfig == nil {
		return nil
	}

	trueVal := true
	autoscalingVals := &helmAutoscaling{
		Enabled: &trueVal,
	}
	if hpaConfig.GetMinReplicas() != nil {
		minReplicas := hpaConfig.GetMinReplicas().GetValue()
		autoscalingVals.MinReplicas = &minReplicas
	}
	if hpaConfig.GetMaxReplicas() != nil {
		maxReplicas := hpaConfig.GetMaxReplicas().GetValue()
		autoscalingVals.MaxReplicas = &maxReplicas
	}
	if hpaConfig.GetTargetCpuUtilizationPercentage() != nil {
		cpuPercent := hpaConfig.GetTargetCpuUtilizationPercentage().GetValue()
		autoscalingVals.TargetCPUUtilizationPercentage = &cpuPercent
	}
	if hpaConfig.GetTargetMemoryUtilizationPercentage() != nil {
		memPercent := hpaConfig.GetTargetMemoryUtilizationPercentage().GetValue()
		autoscalingVals.TargetMemoryUtilizationPercentage = &memPercent
	}

	return autoscalingVals
}

// Convert service values from GatewayParameters into helm values to be used by the deployer.
func getServiceValues(svcConfig *v1alpha1kube.Service) *helmService {
	// convert the service type enum to its string representation;
	// if type is not set, it will default to 0 ("ClusterIP")
	svcType := v1alpha1kube.Service_ServiceType_name[int32(svcConfig.GetType())]
	clusterIp := svcConfig.GetClusterIP().GetValue()
	return &helmService{
		Type:             &svcType,
		ClusterIP:        &clusterIp,
		ExtraAnnotations: svcConfig.GetExtraAnnotations(),
		ExtraLabels:      svcConfig.GetExtraLabels(),
	}
}

// Convert sds values from GatewayParameters into helm values to be used by the deployer.
func getSdsContainerValues(sdsContainerConfig *v1alpha1.SdsContainer) *helmSdsContainer {
	if sdsContainerConfig == nil {
		return nil
	}

	sdsConfigImage := sdsContainerConfig.GetImage()
	sdsImage := &helmImage{
		Registry:   ptr.To(sdsConfigImage.GetRegistry().GetValue()),
		Repository: ptr.To(sdsConfigImage.GetRepository().GetValue()),
		Tag:        ptr.To(sdsConfigImage.GetTag().GetValue()),
		Digest:     ptr.To(sdsConfigImage.GetDigest().GetValue()),
		PullPolicy: ptr.To(sdsConfigImage.GetPullPolicy().String()),
	}
	return &helmSdsContainer{
		Image:           sdsImage,
		Resources:       sdsContainerConfig.GetResources(),
		SecurityContext: sdsContainerConfig.GetSecurityContext(),
		SdsBootstrap: &sdsBootstrap{
			LogLevel: ptr.To(sdsContainerConfig.GetBootstrap().GetLogLevel().GetValue()),
		},
	}
}

func getIstioContainerValues(istioContainerConfig *v1alpha1.IstioContainer) *helmIstioContainer {
	if istioContainerConfig == nil {
		return nil
	}

	istioConfigImage := istioContainerConfig.GetImage()
	istioImage := &helmImage{
		Registry:   ptr.To(istioConfigImage.GetRegistry().GetValue()),
		Repository: ptr.To(istioConfigImage.GetRepository().GetValue()),
		Tag:        ptr.To(istioConfigImage.GetTag().GetValue()),
		Digest:     ptr.To(istioConfigImage.GetDigest().GetValue()),
		PullPolicy: ptr.To(istioConfigImage.GetPullPolicy().String()),
	}

	return &helmIstioContainer{
		Image:           istioImage,
		LogLevel:        ptr.To(istioContainerConfig.GetLogLevel().GetValue()),
		Resources:       istioContainerConfig.GetResources(),
		SecurityContext: istioContainerConfig.GetSecurityContext(),
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
		Enabled:               ptr.To(istioValues.IntegrationEnabled),
		IstioDiscoveryAddress: ptr.To(istioConfig.GetIstioProxyContainer().GetIstioDiscoveryAddress().GetValue()),
		IstioMetaMeshId:       ptr.To(istioConfig.GetIstioProxyContainer().GetIstioMetaMeshId().GetValue()),
		IstioMetaClusterId:    ptr.To(istioConfig.GetIstioProxyContainer().GetIstioMetaClusterId().GetValue()),
	}
}

// Get the image values for the envoy container in the proxy deployment. This is done by:
// 1. getting the image values from a GatewayParameter
// 2. for values not provided, fall back to the defaults (if any) from the k8s gw extensions
func getEnvoyImageValues(envoyImage *v1alpha1kube.Image) *helmImage {
	return &helmImage{
		Registry:   ptr.To(envoyImage.GetRegistry().GetValue()),
		Repository: ptr.To(envoyImage.GetRepository().GetValue()),
		Tag:        ptr.To(envoyImage.GetTag().GetValue()),
		Digest:     ptr.To(envoyImage.GetDigest().GetValue()),
		PullPolicy: ptr.To(envoyImage.GetPullPolicy().String()),
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

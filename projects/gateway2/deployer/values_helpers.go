package deployer

import (
	"fmt"
	"sort"
	"strings"

	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/projects/gateway2/extensions"
	v1alpha1kube "github.com/solo-io/gloo/projects/gateway2/pkg/api/gateway.gloo.solo.io/v1alpha1/kube"
	"github.com/solo-io/gloo/projects/gateway2/ports"
	"golang.org/x/exp/slices"
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
	clusterIp := svcConfig.GetClusterIP()
	return &helmService{
		Type:             &svcType,
		ClusterIP:        &clusterIp,
		ExtraAnnotations: svcConfig.GetExtraAnnotations(),
		ExtraLabels:      svcConfig.GetExtraLabels(),
	}
}

// Get the default image values for the envoy container in the proxy deployment.
// Typically this is a gloo envoy wrapper image.
func getDefaultEnvoyImageValues(image extensions.Image) *helmImage {
	// Get the envoy repo and tag from the k8s gw extensions.
	// The other default values (registry and pullPolicy) are statically defined in the deployer
	// helm chart.
	return &helmImage{
		Repository: &image.Repository,
		Tag:        &image.Tag,
	}
}

// Get the image values for the envoy container in the proxy deployment. This is done by:
// 1. getting the image values from a GatewayParameter
// 2. for values not provided, fall back to the defaults (if any) from the k8s gw extensions
func getMergedEnvoyImageValues(defaultImage extensions.Image, overrideImage *v1alpha1kube.Image) *helmImage {
	// if no overrides are provided, use the default values
	if overrideImage == nil {
		return getDefaultEnvoyImageValues(defaultImage)
	}

	// for repo and tag, fall back to defaults if not provided
	repository := overrideImage.GetRepository()
	if repository == "" {
		repository = defaultImage.Repository
	}
	tag := overrideImage.GetTag()
	if tag == "" {
		tag = defaultImage.Tag
	}

	registry := overrideImage.GetRegistry()
	digest := overrideImage.GetDigest()

	// get the string representation of pull policy, unless it's unspecified, in which case we
	// leave it empty to fall back to the default value
	pullPolicy := ""
	if overrideImage.GetPullPolicy() != v1alpha1kube.Image_Unspecified {
		pullPolicy = v1alpha1kube.Image_PullPolicy_name[int32(overrideImage.GetPullPolicy())]
	}

	return &helmImage{
		Registry:   &registry,
		Repository: &repository,
		Tag:        &tag,
		Digest:     &digest,
		PullPolicy: &pullPolicy,
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

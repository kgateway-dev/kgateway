package deployer

import (
	"fmt"
	"sort"
	"strings"

	"github.com/rotisserie/eris"
	v1alpha1kube "github.com/solo-io/gloo/projects/gateway2/api/v1alpha1"
	"github.com/solo-io/gloo/projects/gateway2/extensions"
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
	hpaConfig := autoscaling.HorizontalPodAutoscaler
	if hpaConfig == nil {
		return nil
	}

	trueVal := true
	autoscalingVals := &helmAutoscaling{
		Enabled: &trueVal,
	}
	autoscalingVals.MinReplicas = hpaConfig.MinReplicas
	autoscalingVals.MaxReplicas = hpaConfig.MaxReplicas
	autoscalingVals.TargetCPUUtilizationPercentage = hpaConfig.TargetCpuUtilizationPercentage
	autoscalingVals.TargetMemoryUtilizationPercentage = hpaConfig.TargetMemoryUtilizationPercentage

	return autoscalingVals
}

// Convert service values from GatewayParameters into helm values to be used by the deployer.
func getServiceValues(svcConfig *v1alpha1kube.Service) *helmService {
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
	repository := overrideImage.Repository
	if repository == "" {
		repository = defaultImage.Repository
	}
	tag := overrideImage.Tag
	if tag == "" {
		tag = defaultImage.Tag
	}

	registry := overrideImage.Registry
	digest := overrideImage.Digest

	// get the string representation of pull policy, unless it's unspecified, in which case we
	// leave it empty to fall back to the default value
	pullPolicy := string(overrideImage.PullPolicy)

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

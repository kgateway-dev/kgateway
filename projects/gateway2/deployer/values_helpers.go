package deployer

import (
	"context"
	"os"
	"strings"

	v1alpha1kube "github.com/solo-io/gloo/projects/gateway2/pkg/api/gateway.gloo.solo.io/v1alpha1/kube"
	"github.com/solo-io/gloo/projects/gateway2/ports"
	"github.com/solo-io/gloo/projects/gloo/constants"
	"golang.org/x/exp/slices"
	"sigs.k8s.io/controller-runtime/pkg/log"
	api "sigs.k8s.io/gateway-api/apis/v1"
)

// This file contains helper functions that generate helm values in the format needed
// by the deployer.

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

// Get the image that the envoy container in the proxy deployment should use (typically a gloo envoy wrapper image).
func getDeployerImageValues(ctx context.Context) *helmImage {
	image := os.Getenv(constants.GlooGatewayDeployerImage)
	defaultImageValues := &helmImage{
		// If tag is not defined, we fall back to the default behavior, which is to use that Chart version
	}

	if image == "" {
		// If the env is not defined, return the default
		return defaultImageValues
	}

	imageParts := strings.Split(image, ":")
	if len(imageParts) != 2 {
		// If the user provided an invalid override, fallback to the default
		log.FromContext(ctx).Info("invalid image override provided, falling back to default", "image", image)

		return defaultImageValues
	}
	return &helmImage{
		Repository: &imageParts[0],
		Tag:        &imageParts[1],
	}
}

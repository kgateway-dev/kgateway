package deployer

import (
	"encoding/json"

	v1alpha1kube "github.com/solo-io/gloo/projects/gateway2/pkg/api/gateway.gloo.solo.io/v1alpha1/kube"
	"github.com/solo-io/gloo/projects/gateway2/ports"
	"golang.org/x/exp/slices"
	api "sigs.k8s.io/gateway-api/apis/v1"
)

// This file contains helper functions that generate helm values in the format needed
// by the deployer.

func GetPortsValues(gw *api.Gateway) ([]any, error) {
	gwPorts := []gatewayPort{}
	for _, l := range gw.Spec.Listeners {
		listenerPort := uint16(l.Port)
		if slices.IndexFunc(gwPorts, func(p gatewayPort) bool { return p.Port == listenerPort }) != -1 {
			continue
		}
		var port gatewayPort
		port.Port = listenerPort
		port.TargetPort = ports.TranslatePort(listenerPort)
		port.Name = string(l.Name)
		port.Protocol = "TCP"
		gwPorts = append(gwPorts, port)
	}

	// convert to json for helm (otherwise go template fails, as the field names are uppercase)
	var portsVals []any
	err := jsonConvertPorts(gwPorts, &portsVals)
	if err != nil {
		return nil, err
	}
	return portsVals, nil
}

func GetAutoscalingValues(autoscaling *v1alpha1kube.Autoscaling) map[string]any {
	hpaConfig := autoscaling.GetHorizontalPodAutoscaler()
	var autoscalingVals map[string]any
	if hpaConfig != nil {
		autoscalingVals = map[string]any{
			"enabled": true,
		}
		if hpaConfig.GetMinReplicas() != nil {
			autoscalingVals["minReplicas"] = hpaConfig.GetMinReplicas().GetValue()
		}
		if hpaConfig.GetMaxReplicas() != nil {
			autoscalingVals["maxReplicas"] = hpaConfig.GetMaxReplicas().GetValue()
		}
		if hpaConfig.GetTargetCpuUtilizationPercentage() != nil {
			autoscalingVals["targetCPUUtilizationPercentage"] = hpaConfig.GetTargetCpuUtilizationPercentage().GetValue()
		}
		if hpaConfig.GetTargetMemoryUtilizationPercentage() != nil {
			autoscalingVals["targetMemoryUtilizationPercentage"] = hpaConfig.GetTargetMemoryUtilizationPercentage().GetValue()
		}
	}
	return autoscalingVals
}

func jsonConvertPorts(in []gatewayPort, out interface{}) error {
	b, err := json.Marshal(in)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}

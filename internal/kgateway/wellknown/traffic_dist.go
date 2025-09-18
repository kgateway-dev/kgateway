package wellknown

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

type TrafficDistribution int

const (
	// TrafficDistributionAny allows any destination
	TrafficDistributionAny TrafficDistribution = iota
	// TrafficDistributionPreferSameZone prefers traffic in same zone, failing over to same region and then network.
	TrafficDistributionPreferSameZone
	// TrafficDistributionPreferSameNode prefers traffic in same node, failing over to same subzone, then zone, region, and network.
	TrafficDistributionPreferSameNode
	// TrafficDistributionPreferSameNetwork prefers traffic in same network.
	TrafficDistributionPreferSameNetwork
)

func ParseTrafficDistribution(value string) TrafficDistribution {
	value = strings.ToLower(value)
	switch value {
	// k8s Service PreferSameZone is an alias for PreferClose and still an alpha feature
	case strings.ToLower(corev1.ServiceTrafficDistributionPreferClose), strings.ToLower(corev1.ServiceTrafficDistributionPreferSameZone):
		return TrafficDistributionPreferSameZone
	// k8s Service PreferSameNode is an alpha feature
	case strings.ToLower(corev1.ServiceTrafficDistributionPreferSameNode):
		return TrafficDistributionPreferSameNode
	case strings.ToLower("PreferSameNetwork"):
		return TrafficDistributionPreferSameNetwork
	default:
		if value != "" {
			fmt.Printf("Unknown traffic distribution annotation value: %s, defaulting to any", value)
		}
		return TrafficDistributionAny
	}
}

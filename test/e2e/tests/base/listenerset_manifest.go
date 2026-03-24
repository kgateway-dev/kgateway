//go:build e2e

package base

import "strings"

func CurrentGwApiChannel() GwApiChannel {
	return currentGwApiChannel
}

func CurrentGwApiVersion() GwApiVersion {
	if currentGwApiVersion == nil {
		return GwApiVersion{}
	}
	return GwApiVersion{Version: *currentGwApiVersion}
}

func UsesLegacyXListenerSet() bool {
	version := CurrentGwApiVersion()
	if version.Version.String() == "" {
		return false
	}

	return CurrentGwApiChannel() == GwApiChannelExperimental &&
		version.LessThan(&GwApiV1_5_1.Version)
}

func TransformListenerSetManifest(content string) string {
	if !UsesLegacyXListenerSet() {
		return content
	}

	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == "kind: ListenerSet" {
			lines[i] = strings.Replace(line, "ListenerSet", "XListenerSet", 1)
		}
	}
	for i, line := range lines {
		if strings.TrimSpace(line) == "apiVersion: gateway.networking.k8s.io/v1" &&
			i+1 < len(lines) &&
			strings.TrimSpace(lines[i+1]) == "kind: XListenerSet" {
			lines[i] = strings.Replace(line, "gateway.networking.k8s.io/v1", "gateway.networking.x-k8s.io/v1alpha1", 1)
		}
	}
	for i, line := range lines {
		if strings.TrimSpace(line) != "group: gateway.networking.k8s.io" {
			continue
		}

		hasXListenerSetNeighbor := i > 0 && strings.TrimSpace(lines[i-1]) == "kind: XListenerSet" ||
			i+1 < len(lines) && strings.TrimSpace(lines[i+1]) == "kind: XListenerSet"
		if hasXListenerSetNeighbor {
			lines[i] = strings.Replace(line, "gateway.networking.k8s.io", "gateway.networking.x-k8s.io", 1)
		}
	}

	return strings.Join(lines, "\n")
}

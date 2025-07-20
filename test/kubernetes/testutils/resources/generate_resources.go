//go:build ignore

package resources

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

var (
	K8sGatewayGvk = schema.GroupVersionKind{
		Group:   "gateway.networking.k8s.io",
		Version: "v1",
		Kind:    "Gateway",
	}

	HTTPRouteGvk = schema.GroupVersionKind{
		Group:   "gateway.networking.k8s.io",
		Version: "v1",
		Kind:    "HTTPRoute",
	}
)

func WriteResourcesToFile(resources []client.Object, fileName string) error {
	// Marshal resources to YAML
	outputResourceManifest := &bytes.Buffer{}
	for _, resource := range resources {
		yamlData, err := objectToYaml(resource)
		if err != nil {
			return fmt.Errorf("can marshal resources to YAML: %w", err)
		}

		outputResourceManifest.Write(yamlData)

		// Separate resources with '---'
		outputResourceManifest.WriteString("\n---\n")
	}

	// Write YAML data to file
	manifestFile, err := os.Create(fileName)
	if err != nil {
		return fmt.Errorf("can create generated file: %w", err)
	}
	defer manifestFile.Close()

	_, err = manifestFile.Write(outputResourceManifest.Bytes())
	if err != nil {
		return fmt.Errorf("can write resources to file: %w", err)
	}
	return nil
}

func objectToYaml(obj client.Object) ([]byte, error) {
	jsonBytes, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal resource to JSON: %w", err)
	}

	yamlBytes, err := yaml.JSONToYAML(jsonBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to convert resource JSON to YAML: %w", err)
	}

	return cleanUp(yamlBytes), nil
}

func cleanUp(objYaml []byte) []byte {
	var lines []string
	scan := bufio.NewScanner(bytes.NewBuffer(objYaml))
	for scan.Scan() {
		line := scan.Text()
		if isNullCreationTime(line) {
			continue
		}

		// Skip status lines when rendering resources
		if isStatusLine(line) {
			break
		}

		lines = append(lines, line)
	}

	if len(lines) == 0 {
		return nil
	}

	return []byte(strings.Join(lines, "\n"))
}

func isNullCreationTime(line string) bool {
	return strings.TrimSpace(line) == "creationTimestamp: null"
}

func isStatusLine(line string) bool {
	return strings.Contains(strings.TrimRight(line, " "), "status")
}

package istio

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options/contextoptions"

	versioncmd "github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/version"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/prerun"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"

	"github.com/ghodss/yaml"
	"github.com/golang/protobuf/jsonpb"

	envoy_config_bootstrap "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func envoyConfigFromString(config string) (envoy_config_bootstrap.Bootstrap, error) {
	var bootstrapConfig envoy_config_bootstrap.Bootstrap
	bootstrapConfig, err := unmarshalYAMLConfig(config)
	return bootstrapConfig, err
}

func getIstiodContainer(ctx context.Context, namespace string) (corev1.Container, error) {
	var c corev1.Container
	kubecontext := contextoptions.KubecontextFrom(ctx)

	client := helpers.MustKubeClientWithKubecontext(kubecontext)
	_, err := client.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if err != nil {
		return c, err
	}
	deployments, err := client.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return c, err
	}

	for _, deployment := range deployments.Items {
		if deployment.Name == "istiod" {
			containers := deployment.Spec.Template.Spec.Containers
			for _, container := range containers {
				if container.Name == "discovery" {
					return container, nil
				}
			}

		}
	}
	return c, ErrIstioVerUndetermined

}

// getImageVersion gets the tag from the image of the given container
func getImageVersion(container corev1.Container) (string, error) {
	img := strings.SplitAfter(container.Image, ":")
	if len(img) != 2 {
		return "", ErrImgVerUndetermined
	}
	return img[1], nil
}

// getIstioMetaMeshID returns the set value or default value 'cluster.local' if unset
func getIstioMetaMeshID(istioMetaMeshID string) string {
	var result = ""

	if istioMetaMeshID == "" {
		result = "cluster.local"
	} else {
		result = istioMetaMeshID
	}

	return result
}

// getIstioMetaClusterID returns the set value or default value 'Kubernetes' if unset
func getIstioMetaClusterID(istioMetaClusterID string) string {
	var result = ""

	if istioMetaClusterID == "" {
		result = "Kubernetes"
	} else {
		result = istioMetaClusterID
	}

	return result
}

// getIstioDiscoveryAddress returns the value set for it, or defaults to "istiod.istio-system.svc:15012" if unset
func getIstioDiscoveryAddress(discoveryAddress string) string {
	if discoveryAddress != "" {
		return discoveryAddress
	}

	return "istiod.istio-system.svc:15012"
}

// getJWTPolicy gets the JWT policy from istiod
func getJWTPolicy(pilotContainer corev1.Container) string {
	for _, env := range pilotContainer.Env {
		if env.Name == "JWT_POLICY" {
			return env.Value
		}
	}
	// Default to third-party if not found
	fmt.Println("Warning: unable to determine Istio JWT Policy, defaulting to third party")
	return "third-party-jwt"
}

// GetGlooVersion gets the version of gloo currently running
// in the given namespace, by checking the gloo deployment.
func GetGlooVersion(ctx context.Context, namespace string) (string, error) {
	kubecontext := contextoptions.KubecontextFrom(ctx)
	sv := versioncmd.NewKube(namespace, kubecontext)
	server, err := sv.Get(ctx)
	if err != nil {
		return "", err
	}
	openSourceVersions, err := prerun.GetOpenSourceVersions(server)
	if err != nil {
		return "", err
	}
	if len(openSourceVersions) == 0 {
		return "", ErrGlooVerUndetermined
	}
	// There shouldn't be multiple gloo versions in a single namespace but it's also probably fine
	if len(openSourceVersions) > 1 {
		fmt.Printf("Found multiple gloo versions, picking %s", openSourceVersions[0].String())
	}
	return openSourceVersions[0].String(), nil
}

// GetGlooVersionWithoutV mirrors the above function but returns the version without the leading 'v'
func GetGlooVersionWithoutV(ctx context.Context, namespace string) (string, error) {
	version, err := GetGlooVersion(ctx, namespace)
	if err == nil {
		return version[1:], nil
	}
	return version, err
}

// unmarshalYAMLConfig converts from an envoy
// bootstrap yaml into a bootstrapConfig struct
func unmarshalYAMLConfig(configYAML string) (envoy_config_bootstrap.Bootstrap, error) {
	var bootstrapConfig envoy_config_bootstrap.Bootstrap
	// first step - serialize yaml to json
	jsondata, err := yaml.YAMLToJSON([]byte(configYAML))
	if err != nil {
		return bootstrapConfig, err
	}

	// second step - unmarshal from json into a bootstrapConfig object
	jsonreader := bytes.NewReader(jsondata)
	var unmarshaler jsonpb.Unmarshaler
	err = unmarshaler.Unmarshal(jsonreader, &bootstrapConfig)
	return bootstrapConfig, err
}

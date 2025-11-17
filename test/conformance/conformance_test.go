package conformance_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/gateway-api/conformance"
	"sigs.k8s.io/gateway-api/conformance/utils/suite"
	"sigs.k8s.io/gateway-api/pkg/features"

	"encoding/json"

	"github.com/kgateway-dev/kgateway/v2/pkg/deployer"
)

func TestConformance(t *testing.T) {
	options := conformance.DefaultOptions(t)

	// Auto-detect the Gateway API channel by checking installed CRDs
	channel, err := detectGatewayAPIChannel()
	if err != nil {
		t.Logf("Failed to detect Gateway API channel, defaulting to experimental: %v", err)
		channel = features.FeatureChannelExperimental
	} else {
		t.Logf("Detected Gateway API channel: %s", channel)
	}

	// Configure profiles and exempt features based on detected channel
	profiles := suite.ParseConformanceProfiles("GATEWAY-HTTP,GATEWAY-GRPC")
	if channel == features.FeatureChannelExperimental {
		profiles.Insert("GATEWAY-TLS")
	}
	options.ConformanceProfiles = profiles

	exemptFeatures := deployer.GetCommonExemptFeatures()

	if channel == features.FeatureChannelStandard {
		exemptExperimentalFeatures(exemptFeatures)
	}

	exemptFeatureString := suite.ParseSupportedFeatures(featureSetToCommaSeparatedString(exemptFeatures))
	options.ExemptFeatures = suite.FeaturesSet(exemptFeatureString)

	t.Logf("Running conformance tests with\nprofiles: %+v\nexempt features: %+v\n", profiles, exemptFeatures)
	conformance.RunConformanceWithOptions(t, options)
}

type CRD struct {
	Metadata struct {
		Annotations map[string]string `json:"annotations"`
	} `json:"metadata"`
}

// detectGatewayAPIChannel checks which Gateway API CRDs are installed to determine the channel
func detectGatewayAPIChannel() (string, error) {
	cfg, err := config.GetConfig()
	if err != nil {
		return "", err
	}
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return "", err
	}

	// Check the gateway.networking.k8s.io/channel annotation on HTTPRoute CRD
	crd, err := clientset.RESTClient().
		Get().
		AbsPath("/apis/apiextensions.k8s.io/v1/customresourcedefinitions/httproutes.gateway.networking.k8s.io").
		DoRaw(context.Background())
	if err != nil {
		return "", err
	}

	var crdObj CRD
	if err := json.Unmarshal(crd, &crdObj); err != nil {
		return "", err
	}

	channel := crdObj.Metadata.Annotations["gateway.networking.k8s.io/channel"]

	if channel == "" {
		return "", fmt.Errorf("gateway.networking.k8s.io/channel annotation not found on HTTPRoute CRD")
	}

	return channel, nil
}

// exemptExperimentalFeatures exempts all experimental features from the exemptFeatures set. Modifies the set in place.
func exemptExperimentalFeatures(exemptFeatures sets.Set[features.Feature]) {
	for _, feature := range features.AllFeatures.UnsortedList() {
		if feature.Channel == features.FeatureChannelExperimental {
			exemptFeatures.Insert(feature)
		}
	}
}

func featureSetToCommaSeparatedString(featureSet sets.Set[features.Feature]) string {
	features := []string{}
	for _, feature := range featureSet.UnsortedList() {
		features = append(features, string(feature.Name))
	}
	return strings.Join(features, ",")
}

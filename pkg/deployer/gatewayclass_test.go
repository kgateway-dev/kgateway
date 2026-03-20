package deployer

import (
	"testing"

	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	"sigs.k8s.io/gateway-api/pkg/features"
)

func supportedFeatureSet(featuresList []gwv1.SupportedFeature) map[gwv1.FeatureName]struct{} {
	supportedNames := make(map[gwv1.FeatureName]struct{}, len(featuresList))
	for _, feature := range featuresList {
		supportedNames[feature.Name] = struct{}{}
	}
	return supportedNames
}

func TestGetSupportedFeaturesForStandardGatewayExcludesKnownUnsupportedV15Features(t *testing.T) {
	t.Helper()

	supportedNames := supportedFeatureSet(GetSupportedFeaturesForStandardGateway(true))

	if _, ok := supportedNames[gwv1.FeatureName(features.SupportHTTPRouteCORS)]; ok {
		t.Fatalf("expected %q to be exempted from supported features", features.SupportHTTPRouteCORS)
	}
	if _, ok := supportedNames[gwv1.FeatureName(features.SupportTLSRoute)]; !ok {
		t.Fatalf("expected %q to remain supported when experimental Gateway API features are enabled", features.SupportTLSRoute)
	}
	if _, ok := supportedNames[gwv1.FeatureName(features.SupportGatewayFrontendClientCertificateValidation)]; !ok {
		t.Fatalf("expected %q to remain supported", features.SupportGatewayFrontendClientCertificateValidation)
	}
}

func TestGetSupportedFeaturesForStandardGatewayExcludesTLSRouteWhenExperimentalDisabled(t *testing.T) {
	t.Helper()

	supportedNames := supportedFeatureSet(GetSupportedFeaturesForStandardGateway(false))

	if _, ok := supportedNames[gwv1.FeatureName(features.SupportTLSRoute)]; ok {
		t.Fatalf("expected %q to be exempted when experimental Gateway API features are disabled", features.SupportTLSRoute)
	}
	if _, ok := supportedNames[gwv1.FeatureName(features.SupportTLSRouteModeTerminate)]; ok {
		t.Fatalf("expected %q to be exempted when experimental Gateway API features are disabled", features.SupportTLSRouteModeTerminate)
	}
	if _, ok := supportedNames[gwv1.FeatureName(features.SupportTLSRouteModeMixed)]; ok {
		t.Fatalf("expected %q to be exempted when experimental Gateway API features are disabled", features.SupportTLSRouteModeMixed)
	}
}

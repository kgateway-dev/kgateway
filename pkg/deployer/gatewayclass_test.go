package deployer

import (
	"testing"

	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	"sigs.k8s.io/gateway-api/pkg/features"
)

func TestGetSupportedFeaturesForStandardGatewayExcludesKnownUnsupportedV15Features(t *testing.T) {
	t.Helper()

	supported := GetSupportedFeaturesForStandardGateway()
	supportedNames := make(map[gwv1.FeatureName]struct{}, len(supported))
	for _, feature := range supported {
		supportedNames[feature.Name] = struct{}{}
	}

	if _, ok := supportedNames[gwv1.FeatureName(features.SupportHTTPRouteCORS)]; ok {
		t.Fatalf("expected %q to be exempted from supported features", features.SupportHTTPRouteCORS)
	}
	if _, ok := supportedNames[gwv1.FeatureName(features.SupportListenerSet)]; ok {
		t.Fatalf("expected %q to be exempted from supported features", features.SupportListenerSet)
	}
	if _, ok := supportedNames[gwv1.FeatureName(features.SupportTLSRoute)]; !ok {
		t.Fatalf("expected %q to remain supported", features.SupportTLSRoute)
	}
	if _, ok := supportedNames[gwv1.FeatureName(features.SupportGatewayFrontendClientCertificateValidation)]; !ok {
		t.Fatalf("expected %q to remain supported", features.SupportGatewayFrontendClientCertificateValidation)
	}
}

package deployer

import (
	"cmp"
	"slices"

	"k8s.io/apimachinery/pkg/util/sets"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	"sigs.k8s.io/gateway-api/pkg/features"
)

// GatewayClassInfo describes the desired configuration for a GatewayClass.
type GatewayClassInfo struct {
	// Description is a human-readable description of the GatewayClass.
	Description string
	// Labels are the labels to be added to the GatewayClass.
	Labels map[string]string
	// Annotations are the annotations to be added to the GatewayClass.
	Annotations map[string]string
	// ParametersRef is the reference to the GatewayParameters object.
	ParametersRef *gwv1.ParametersReference
	// ControllerName is the name of the controller that is managing the GatewayClass.
	ControllerName string
	// SupportedFeatures is the list of Gateway API features supported by this GatewayClass.
	// This will be populated in the GatewayClass status.supportedFeatures field.
	SupportedFeatures []gwv1.SupportedFeature
}

// GetSupportedFeaturesForStandardGateway returns the supported features for the standard Gateway class.
// This is derived from the conformance test configuration where we exempt certain features, and from
// whether experimental Gateway API features are enabled in the running controller.
func GetSupportedFeaturesForStandardGateway(enableExperimentalGatewayAPIFeatures bool) []gwv1.SupportedFeature {
	exemptFeatures := GetCommonExemptFeatures()
	// backfill individual features that we don't support yet.
	exemptFeatures.Insert(
		features.GatewayHTTPListenerIsolationFeature,
		// The new Gateway API v1.5.x CORS conformance coverage does not pass yet.
		features.HTTPRouteCORS,
		// Gateway.spec.tls.backend.clientCertificateRef is not translated yet.
		features.GatewayBackendClientCertificateFeature,
		// We support the AllowInsecureFallback traffic behavior, but do not yet publish the
		// required InsecureFrontendValidationMode Gateway status condition.
		features.GatewayFrontendClientCertificateValidationInsecureFallbackFeature,
		// We do not yet implement the 421 misdirected-request behavior across HTTPS listeners
		// sharing the same port.
		features.GatewayHTTPSListenerDetectMisdirectedRequestsFeature,
		// ListenerSet status and attachment conformance is not complete yet.
		features.ListenerSetFeature,
	)
	if !enableExperimentalGatewayAPIFeatures {
		// TLSRoute processing is behind the experimental Gateway API feature flag.
		// Standard conformance runs disable that flag, so the standard GatewayClass must not
		// advertise TLSRoute support in that mode.
		exemptFeatures.Insert(
			features.TLSRouteFeature,
			features.TLSRouteModeTerminateFeature,
			features.TLSRouteModeMixedFeature,
		)
	}

	// we don't support the BackendTLSPolicy feature at all.
	for _, feature := range features.BackendTLSPolicyCoreFeatures.UnsortedList() {
		exemptFeatures.Insert(feature)
	}
	for _, feature := range features.BackendTLSPolicyExtendedFeatures.UnsortedList() {
		exemptFeatures.Insert(feature)
	}
	return getSupportedFeatures(exemptFeatures)
}

// GetSupportedFeaturesForWaypointGateway returns the supported features for the waypoint Gateway class.
// Waypoint gateways have similar support to standard gateways but may have some differences.
func GetSupportedFeaturesForWaypointGateway(enableExperimentalGatewayAPIFeatures bool) []gwv1.SupportedFeature {
	// For now, waypoint gateways support the same features as standard gateways
	return GetSupportedFeaturesForStandardGateway(enableExperimentalGatewayAPIFeatures)
}

// GetCommonExemptFeatures returns the set of features that are commonly unsupported across all gateway classes.
// Exported for use in the conformance test suite.
func GetCommonExemptFeatures() sets.Set[features.Feature] {
	exemptFeatures := sets.New[features.Feature]()
	// we don't support any mesh features at all.
	for _, feature := range features.MeshCoreFeatures.UnsortedList() {
		exemptFeatures.Insert(feature)
	}
	for _, feature := range features.MeshExtendedFeatures.UnsortedList() {
		exemptFeatures.Insert(feature)
	}
	return exemptFeatures
}

// getSupportedFeatures builds a sorted list of supported features, excluding the provided exempt features.
func getSupportedFeatures(exemptFeatures sets.Set[features.Feature]) []gwv1.SupportedFeature {
	var allSupportedFeatures []gwv1.SupportedFeature
	for _, feature := range features.AllFeatures.UnsortedList() {
		if exemptFeatures.Has(feature) {
			continue
		}
		allSupportedFeatures = append(allSupportedFeatures, gwv1.SupportedFeature{
			Name: gwv1.FeatureName(feature.Name),
		})
	}
	slices.SortFunc(allSupportedFeatures, func(a, b gwv1.SupportedFeature) int {
		return cmp.Compare(a.Name, b.Name)
	})
	return allSupportedFeatures
}

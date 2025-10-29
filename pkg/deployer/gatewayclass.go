package deployer

import (
	"cmp"
	"slices"

	"k8s.io/apimachinery/pkg/util/sets"
	apiv1 "sigs.k8s.io/gateway-api/apis/v1"
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
	ParametersRef *apiv1.ParametersReference
	// ControllerName is the name of the controller that is managing the GatewayClass.
	ControllerName string
	// SupportedFeatures is the list of Gateway API features supported by this GatewayClass.
	// This will be populated in the GatewayClass status.supportedFeatures field.
	SupportedFeatures []apiv1.SupportedFeature
}

// GetSupportedFeaturesForStandardGateway returns the supported features for the standard Gateway class.
// This is derived from the conformance test configuration where we exempt certain features.
func GetSupportedFeaturesForStandardGateway() []apiv1.SupportedFeature {
	exemptFeatures := getCommonExemptFeatures()
	// backfill individual features that we don't support yet.
	exemptFeatures.Insert(
		features.GatewayStaticAddressesFeature,
		features.GatewayHTTPListenerIsolationFeature,
		features.GatewayPort8080Feature,
	)
	return getSupportedFeatures(exemptFeatures)
}

// GetSupportedFeaturesForWaypointGateway returns the supported features for the waypoint Gateway class.
// Waypoint gateways have similar support to standard gateways but may have some differences.
func GetSupportedFeaturesForWaypointGateway() []apiv1.SupportedFeature {
	// For now, waypoint gateways support the same features as standard gateways
	return GetSupportedFeaturesForStandardGateway()
}

// GetSupportedFeaturesForAgentGateway returns the supported features for the agent Gateway class.
// Agent gateways support additional features beyond the standard gateway class.
func GetSupportedFeaturesForAgentGateway() []apiv1.SupportedFeature {
	exemptFeatures := getCommonExemptFeatures()
	// Agent gateways support GatewayHTTPListenerIsolation and GatewayPort8080,
	// but still don't support GatewayStaticAddresses.
	exemptFeatures.Insert(features.GatewayStaticAddressesFeature)
	return getSupportedFeatures(exemptFeatures)
}

// getCommonExemptFeatures returns the set of features that are commonly unsupported across all gateway classes.
func getCommonExemptFeatures() sets.Set[features.Feature] {
	exemptFeatures := sets.New[features.Feature]()
	// we don't support any mesh features at all.
	for _, feature := range features.MeshCoreFeatures.UnsortedList() {
		exemptFeatures.Insert(feature)
	}
	for _, feature := range features.MeshExtendedFeatures.UnsortedList() {
		exemptFeatures.Insert(feature)
	}
	// we don't support the BackendTLSPolicy feature at all.
	for _, feature := range features.BackendTLSPolicyCoreFeatures.UnsortedList() {
		exemptFeatures.Insert(feature)
	}
	for _, feature := range features.BackendTLSPolicyExtendedFeatures.UnsortedList() {
		exemptFeatures.Insert(feature)
	}
	return exemptFeatures
}

// getSupportedFeatures builds a sorted list of supported features, excluding the provided exempt features.
func getSupportedFeatures(exemptFeatures sets.Set[features.Feature]) []apiv1.SupportedFeature {
	var allSupportedFeatures []apiv1.SupportedFeature
	for _, feature := range features.AllFeatures.UnsortedList() {
		if exemptFeatures.Has(feature) {
			continue
		}
		allSupportedFeatures = append(allSupportedFeatures, apiv1.SupportedFeature{
			Name: apiv1.FeatureName(feature.Name),
		})
	}
	slices.SortFunc(allSupportedFeatures, func(a, b apiv1.SupportedFeature) int {
		return cmp.Compare(a.Name, b.Name)
	})
	return allSupportedFeatures
}

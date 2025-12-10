package utils

import (
	"fmt"
	"maps"
	"strconv"

	"k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func TargetRefsToPolicyRefs(
	targetRefs []shared.LocalPolicyTargetReference,
	targetSelectors []shared.LocalPolicyTargetSelector,
) []ir.PolicyRef {
	targetRefsWithSectionName := make([]shared.LocalPolicyTargetReferenceWithSectionName, 0, len(targetRefs))
	for _, targetRef := range targetRefs {
		targetRefsWithSectionName = append(targetRefsWithSectionName, shared.LocalPolicyTargetReferenceWithSectionName{
			LocalPolicyTargetReference: targetRef,
			SectionName:                nil,
		})
	}
	targetSelectorsWithSectionName := make([]shared.LocalPolicyTargetSelectorWithSectionName, 0, len(targetSelectors))
	for _, targetSelector := range targetSelectors {
		targetSelectorsWithSectionName = append(targetSelectorsWithSectionName, shared.LocalPolicyTargetSelectorWithSectionName{
			LocalPolicyTargetSelector: targetSelector,
			SectionName:               nil,
		})
	}
	return TargetRefsToPolicyRefsWithSectionName(targetRefsWithSectionName, targetSelectorsWithSectionName)
}

func TargetRefsToPolicyRefsWithSectionName(
	targetRefs []shared.LocalPolicyTargetReferenceWithSectionName,
	targetSelectors []shared.LocalPolicyTargetSelectorWithSectionName,
) []ir.PolicyRef {
	refs := make([]ir.PolicyRef, 0, len(targetRefs)+len(targetSelectors))
	for _, targetRef := range targetRefs {
		refs = append(refs, ir.PolicyRef{
			Group:       string(targetRef.Group),
			Kind:        string(targetRef.Kind),
			Name:        string(targetRef.Name),
			SectionName: string(ptr.Deref(targetRef.SectionName, "")),
		})
	}
	for _, targetSelector := range targetSelectors {
		refs = append(refs, ir.PolicyRef{
			Group: string(targetSelector.Group),
			Kind:  string(targetSelector.Kind),
			// Clone to avoid mutating the original map
			MatchLabels: maps.Clone(targetSelector.MatchLabels),
			SectionName: string(ptr.Deref(targetSelector.SectionName, "")),
		})
	}
	return refs
}

func TargetRefsToPolicyRefsWithSectionNameV1(targetRefs []gwv1.LocalPolicyTargetReferenceWithSectionName) []ir.PolicyRef {
	refs := make([]ir.PolicyRef, 0, len(targetRefs))
	for _, targetRef := range targetRefs {
		refs = append(refs, ir.PolicyRef{
			Group:       string(targetRef.Group),
			Kind:        string(targetRef.Kind),
			Name:        string(targetRef.Name),
			SectionName: string(ptr.Deref(targetRef.SectionName, "")),
		})
	}

	return refs
}

func TargetRefsToPolicyRefsWithSectionNameV1Alpha2(targetRefs []gwv1a2.LocalPolicyTargetReferenceWithSectionName) []ir.PolicyRef {
	refs := make([]ir.PolicyRef, 0, len(targetRefs))
	for _, targetRef := range targetRefs {
		refs = append(refs, ir.PolicyRef{
			Group:       string(targetRef.Group),
			Kind:        string(targetRef.Kind),
			Name:        string(targetRef.Name),
			SectionName: string(ptr.Deref(targetRef.SectionName, "")),
		})
	}

	return refs
}

// ParsePrecedenceWeightAnnotation parses the given route/policy weight value from the given annotations and key
func ParsePrecedenceWeightAnnotation(
	annotations map[string]string,
	key string,
) (int32, error) {
	val, ok := annotations[key]
	if !ok {
		return 0, nil
	}
	weight, err := strconv.ParseInt(val, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid value for annotation %s: %s; must be a valid integer", key, val)
	}
	return int32(weight), nil
}

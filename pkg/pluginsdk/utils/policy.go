package utils

import (
	"maps"

	"k8s.io/utils/ptr"
	v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
)

func TargetRefsToPolicyRefs(
	targetRefs []v1alpha1.LocalPolicyTargetReference,
	targetSelectors []v1alpha1.LocalPolicyTargetSelector,
) []ir.PolicyRef {
	targetRefsWithSectionName := make([]v1alpha1.LocalPolicyTargetReferenceWithSectionName, 0, len(targetRefs))
	for _, targetRef := range targetRefs {
		targetRefsWithSectionName = append(targetRefsWithSectionName, v1alpha1.LocalPolicyTargetReferenceWithSectionName{
			LocalPolicyTargetReference: targetRef,
			SectionName:                nil,
		})
	}
	targetSelectorsWithSectionName := make([]v1alpha1.LocalPolicyTargetSelectorWithSectionName, 0, len(targetSelectors))
	for _, targetSelector := range targetSelectors {
		targetSelectorsWithSectionName = append(targetSelectorsWithSectionName, v1alpha1.LocalPolicyTargetSelectorWithSectionName{
			LocalPolicyTargetSelector: targetSelector,
			SectionName:               nil,
		})
	}
	return TargetRefsToPolicyRefsWithSectionName(targetRefsWithSectionName, targetSelectorsWithSectionName)
}

func TargetRefsToPolicyRefsWithSectionName(
	targetRefs []v1alpha1.LocalPolicyTargetReferenceWithSectionName,
	targetSelectors []v1alpha1.LocalPolicyTargetSelectorWithSectionName,
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

func TargetRefsToPolicyRefsWithSectionNameV1Alpha2(targetRefs []v1alpha2.LocalPolicyTargetReferenceWithSectionName) []ir.PolicyRef {
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

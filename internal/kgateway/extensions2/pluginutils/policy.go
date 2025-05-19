package pluginutils

import (
	"maps"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
)

func TargetRefsToPolicyRefs(
	targetRefs []v1alpha1.LocalPolicyTargetReference,
	targetSelectors []v1alpha1.LocalPolicyTargetSelector,
) []ir.PolicyRef {
	refs := make([]ir.PolicyRef, 0, len(targetRefs)+len(targetSelectors))
	for _, targetRef := range targetRefs {
		refs = append(refs, ir.PolicyRef{
			Group: string(targetRef.Group),
			Kind:  string(targetRef.Kind),
			Name:  string(targetRef.Name),
		})
	}
	for _, targetSelector := range targetSelectors {
		refs = append(refs, ir.PolicyRef{
			Group: string(targetSelector.Group),
			Kind:  string(targetSelector.Kind),
			// Clone to avoid mutating the original map
			MatchLabels: maps.Clone(targetSelector.MatchLabels),
		})
	}
	return refs
}

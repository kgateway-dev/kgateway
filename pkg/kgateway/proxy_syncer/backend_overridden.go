package proxy_syncer

import (
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	reportssdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
)

// invalidWinnerSuffix is appended when the policy that took a field is invalid: translation
// then blackholes the cluster instead of applying either policy's field.
const invalidWinnerSuffix = ", which is invalid; requests to this backend fail until it is fixed or removed"

// overriddenCondition returns the Overridden condition for a policy attached to a backend
// whose fields in dropped do not take effect, or ok=false when every field it sets applies.
//
// The reason names the kind that took the fields (ConflictedWith<Kind>), preferring another
// kind over the policy's own: a field superseded by another kind is the one a user is least
// likely to expect. The message lists each dropped field and the policy that supplies it.
func overriddenCondition(policy ir.PolicyAtt, dropped []ir.DroppedField) (reportssdk.PolicyCondition, bool) {
	if len(dropped) == 0 {
		return reportssdk.PolicyCondition{}, false
	}
	reasonKind := dropped[0].Kind.Kind
	for _, d := range dropped {
		if d.Reason == ir.DroppedBySupersede {
			reasonKind = d.Kind.Kind
			break
		}
	}
	msgs := make([]string, 0, len(dropped))
	for _, d := range dropped {
		by := "another " + d.Kind.Kind
		if d.By != nil {
			by = fmt.Sprintf("%s %s/%s", d.Kind.Kind, d.By.Namespace, d.By.Name)
		}
		msg := fmt.Sprintf("spec.%s overridden by %s", d.Field, by)
		if !d.ByValid {
			msg += invalidWinnerSuffix
		}
		msgs = append(msgs, msg)
	}
	return reportssdk.PolicyCondition{
		Type:               reportssdk.PolicyConditionOverridden,
		Status:             metav1.ConditionTrue,
		Reason:             "ConflictedWith" + reasonKind,
		Message:            strings.Join(msgs, "; "),
		ObservedGeneration: policy.Generation,
	}, true
}

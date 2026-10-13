package pluginutils

import (
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
)

// BuildCondition builds a Backend's "Accepted" condition from its errors: True/Accepted when
// there are none, False/Invalid otherwise, with the errors aggregated into the message.
func BuildCondition(resource string, errs []error) metav1.Condition {
	return buildAcceptedCondition(resource,
		string(kgateway.BackendConditionAccepted),
		string(kgateway.BackendReasonAccepted),
		string(kgateway.BackendReasonInvalid),
		errs)
}

// BuildGatewayExtensionCondition is BuildCondition for a GatewayExtension. It mirrors the
// Backend vocabulary: Accepted=True/Accepted when the extension translated, and
// Accepted=False/Invalid with the translation error otherwise.
func BuildGatewayExtensionCondition(resource string, errs []error) metav1.Condition {
	return buildAcceptedCondition(resource,
		string(kgateway.GatewayExtensionConditionAccepted),
		string(kgateway.GatewayExtensionReasonAccepted),
		string(kgateway.GatewayExtensionReasonInvalid),
		errs)
}

// buildAcceptedCondition builds an "Accepted" condition from errs, parameterized by the
// condition vocabulary so each resource kind reports under its own declared type and reasons.
func buildAcceptedCondition(resource, condType, reasonAccepted, reasonInvalid string, errs []error) metav1.Condition {
	if len(errs) == 0 {
		return metav1.Condition{
			Type:    condType,
			Status:  metav1.ConditionTrue,
			Reason:  reasonAccepted,
			Message: resource + " accepted",
		}
	}
	var aggErrs strings.Builder
	var prologue string
	if len(errs) == 1 {
		prologue = resource + " error:"
	} else {
		prologue = fmt.Sprintf("%s has %d errors:", resource, len(errs))
	}
	aggErrs.Write([]byte(prologue))
	for _, err := range errs {
		aggErrs.Write([]byte(` "`))
		aggErrs.Write([]byte(err.Error()))
		aggErrs.Write([]byte(`"`))
	}
	return metav1.Condition{
		Type:    condType,
		Status:  metav1.ConditionFalse,
		Reason:  reasonInvalid,
		Message: aggErrs.String(),
	}
}

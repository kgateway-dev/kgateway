package pluginutils

import (
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
)

// BuildCondition builds a resource's "Accepted" condition from its errors: True/Accepted when
// there are none, False/Invalid otherwise, with the errors aggregated into the message.
func BuildCondition(resource string, errs []error) metav1.Condition {
	if len(errs) == 0 {
		return metav1.Condition{
			Type:    string(kgateway.BackendConditionAccepted),
			Status:  metav1.ConditionTrue,
			Reason:  string(kgateway.BackendReasonAccepted),
			Message: fmt.Sprintf("%s accepted", resource),
		}
	}
	var aggErrs strings.Builder
	var prologue string
	if len(errs) == 1 {
		prologue = fmt.Sprintf("%s error:", resource)
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
		Type:    string(kgateway.BackendConditionAccepted),
		Status:  metav1.ConditionFalse,
		Reason:  string(kgateway.BackendReasonInvalid),
		Message: aggErrs.String(),
	}
}

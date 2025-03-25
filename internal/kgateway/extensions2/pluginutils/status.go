package pluginutils

import (
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func BuildCondition(resource string, errs []error) metav1.Condition {
	if len(errs) == 0 {
		return metav1.Condition{
			Type:    "Accepted",
			Status:  metav1.ConditionTrue,
			Reason:  "Accepted",
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
		Type:    "Accepted",
		Status:  metav1.ConditionFalse,
		Reason:  "Invalid",
		Message: aggErrs.String(),
	}
}

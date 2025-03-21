package backend

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/avast/retry-go"
	"github.com/solo-io/go-utils/contextutils"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
)

func buildProcessStatus(cl client.Client) func(ctx context.Context, backendObj ir.BackendObjectIR) {
	return func(ctx context.Context, backendObj ir.BackendObjectIR) {
		ctx = contextutils.WithLogger(ctx, "backendStatus")
		logger := contextutils.LoggerFrom(ctx)

		res := v1alpha1.Backend{}
		resNN := types.NamespacedName{
			Name:      backendObj.Name,
			Namespace: backendObj.Namespace,
		}
		err := retry.Do(
			func() error {
				err := cl.Get(ctx, resNN, &res)
				if err != nil {
					logger.Error("error getting backend: ", err.Error())
					return err
				}

				ir, ok := backendObj.ObjIr.(*BackendIr)
				if !ok {
					// FIXME
					return nil
				}
				newCondition := buildBackendCondition(ir.Errors)

				found := meta.FindStatusCondition(res.Status.Conditions, string(gwv1a2.PolicyConditionAccepted))
				if found != nil {
					typeEq := found.Type == newCondition.Type
					statusEq := found.Status == newCondition.Status
					reasonEq := found.Reason == newCondition.Reason
					messageEq := found.Message == newCondition.Message
					if typeEq && statusEq && reasonEq && messageEq {
						// condition is already up-to-date, nothing to do
						return nil
					}
				}

				conditions := make([]metav1.Condition, 0, 1)
				meta.SetStatusCondition(&conditions, newCondition)
				res.Status.Conditions = conditions
				if err := cl.Status().Patch(ctx, &res, client.Merge); err != nil {
					logger.Error(err)
					return err
				}
				return nil
			},
			retry.Attempts(5),
			retry.Delay(100*time.Millisecond),
			retry.DelayType(retry.BackOffDelay),
		)
		if err != nil {
			logger.Errorw(
				"all attempts failed updating backend status",
				"Backend",
				resNN.String(),
				"error",
				err,
			)
		}
	}
}

func buildBackendCondition(errs []error) metav1.Condition {
	if len(errs) == 0 {
		return metav1.Condition{
			Type:    "Accepted",
			Status:  metav1.ConditionTrue,
			Reason:  "Accepted",
			Message: "Backend accepted",
		}
	}
	var aggErrs strings.Builder
	var prologue string
	if len(errs) == 1 {
		prologue = "Backend error:"
	} else {
		prologue = fmt.Sprintf("Backend has %d errors:", len(errs))
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

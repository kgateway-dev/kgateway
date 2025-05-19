package backend

import (
	"context"
	"time"

	"github.com/avast/retry-go"
	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/pluginutils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
)

func buildRegisterCallback(
	ctx context.Context,
	cl client.Client,
	bcol krt.Collection[ir.BackendObjectIR],
) func() {
	return func() {
		bcol.Register(func(o krt.Event[ir.BackendObjectIR]) {
			if o.Event == controllers.EventDelete {
				return
			}

			in := o.Latest()
			ir, ok := in.ObjIr.(*BackendIr)
			if !ok {
				return
			}

			resNN := types.NamespacedName{
				Name:      in.ObjectSource.Name,
				Namespace: in.ObjectSource.Namespace,
			}
			res := v1alpha1.Backend{}
			err := retry.Do(
				func() error {
					err := cl.Get(ctx, resNN, &res)
					if err != nil {
						logger.Error("error getting backend", "error", err)
						return err
					}

					newCondition := pluginutils.BuildCondition("Backend", ir.Errors)

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
						logger.Error("error updating backend status", "error", err)
						return err
					}
					return nil
				},
				retry.Attempts(5),
				retry.Delay(100*time.Millisecond),
				retry.DelayType(retry.BackOffDelay),
			)
			if err != nil {
				logger.Error(
					"all attempts failed updating backend status",
					"backend", resNN.String(),
					"error", err,
				)
			}
		})
	}
}

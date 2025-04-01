package httplistenerpolicy

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

	"github.com/solo-io/go-utils/contextutils"
)

func buildRegisterCallback(
	ctx context.Context,
	cl client.Client,
	col krt.Collection[ir.PolicyWrapper],
) func() {
	return func() {
		logger := contextutils.LoggerFrom(ctx)
		col.Register(func(o krt.Event[ir.PolicyWrapper]) {
			if o.Event == controllers.EventDelete {
				return
			}

			in := o.Latest()
			routePolIr, ok := in.PolicyIR.(*httpListenerPolicy)
			if !ok {
				return
			}

			resNN := types.NamespacedName{
				Name:      in.ObjectSource.Name,
				Namespace: in.ObjectSource.Namespace,
			}
			res := v1alpha1.HTTPListenerPolicy{}
			err := retry.Do(
				func() error {
					err := cl.Get(ctx, resNN, &res)
					if err != nil {
						logger.Error("error getting httplistenerpolicy: ", err.Error())
						return err
					}

					newCondition := pluginutils.BuildCondition("Policy", routePolIr.errors)

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
					"all attempts failed updating httplistenerpolicy status",
					"Policy",
					resNN.String(),
					"error",
					err,
				)
			}
		})
	}
}

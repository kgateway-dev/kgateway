package backend

import (
	"context"
	"fmt"
	"time"

	"github.com/avast/retry-go/v4"
	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/pluginutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func buildRegisterCallback(
	ctx context.Context,
	cl kclient.Client[*v1alpha1.Backend],
	bcol krt.Collection[ir.BackendObjectIR],
) func() {
	return func() {
		bcol.Register(func(o krt.Event[ir.BackendObjectIR]) {
			if o.Event == controllers.EventDelete {
				return
			}
			in := o.Latest()
			ir, ok := in.ObjIr.(*backendIr)
			if !ok {
				return
			}

			resNN := types.NamespacedName{
				Name:      in.ObjectSource.Name,
				Namespace: in.ObjectSource.Namespace,
			}

			err := retry.Do(
				func() error {
					cur := cl.Get(resNN.Name, resNN.Namespace)
					if cur == nil {
						logger.Error("error getting backend", "ref", resNN, "error", pluginsdk.ErrNotFound)
						return pluginsdk.ErrNotFound
					}

					newCondition := pluginutils.BuildCondition("Backend", ir.errors)

					found := meta.FindStatusCondition(cur.Status.Conditions, string(gwv1.PolicyConditionAccepted))
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
					if _, err := cl.UpdateStatus(&v1alpha1.Backend{
						ObjectMeta: pluginsdk.CloneObjectMetaForStatus(cur.ObjectMeta),
						Status: v1alpha1.BackendStatus{
							Conditions: conditions,
						},
					}); err != nil {
						if errors.IsConflict(err) {
							logger.Debug("error updating stale status", "ref", resNN, "error", err)
							return nil // let the conflicting Status update trigger a KRT event to requeue the updated object
						}
						return fmt.Errorf("error updating status for Backend %s: %w", resNN, err)
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

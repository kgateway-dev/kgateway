package backend

import (
	"context"
	"sync"
	"time"

	"github.com/avast/retry-go"
	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/pluginutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

// backendStatusClient is used to update Backend CRD status when runtime translation errors occur
var (
	backendStatusClient   client.Client
	backendStatusClientMu sync.RWMutex
)

// setBackendStatusClient sets the backend status client in a thread-safe manner
func setBackendStatusClient(c client.Client) {
	backendStatusClientMu.Lock()
	defer backendStatusClientMu.Unlock()
	backendStatusClient = c
}

// getBackendStatusClient gets the backend status client in a thread-safe manner
func getBackendStatusClient() client.Client {
	backendStatusClientMu.RLock()
	defer backendStatusClientMu.RUnlock()
	return backendStatusClient
}

// deduplicateErrors removes duplicate errors based on their error message strings.
func deduplicateErrors(errs []error) []error {
	seen := make(map[string]bool, len(errs))
	result := make([]error, 0, len(errs))

	for _, err := range errs {
		if err == nil {
			continue
		}
		msg := err.Error()
		if !seen[msg] {
			seen[msg] = true
			result = append(result, err)
		}
	}

	return result
}

// updateBackendStatus updates the Backend CRD status with the given errors.
func updateBackendStatus(ctx context.Context, namespace, name string, errs []error) {
	cl := getBackendStatusClient()
	if cl == nil {
		logger.Error("error updating backend status: client not initialized")
		return
	}

	// dedup needed to avoid multiple update error called same time
	errs = deduplicateErrors(errs)

	resNN := types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}
	res := v1alpha1.Backend{}
	err := retry.Do(
		func() error {
			err := cl.Get(ctx, resNN, &res)
			if err != nil {
				logger.Error("error getting backend", "error", err)
				return err
			}

			newCondition := pluginutils.BuildCondition("Backend", errs)

			found := meta.FindStatusCondition(res.Status.Conditions, string(gwv1.PolicyConditionAccepted))
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
			if err = cl.Status().Patch(ctx, &res, client.Merge); err != nil {
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
}

func buildRegisterCallback(ctx context.Context, bcol krt.Collection[ir.BackendObjectIR]) func() {
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

			updateBackendStatus(ctx, in.ObjectSource.Namespace, in.ObjectSource.Name, ir.errors)
		})
	}
}

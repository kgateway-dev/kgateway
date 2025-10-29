package backend

import (
	"context"
	"fmt"
	"sync"
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

// backendStatusClient is used to update Backend CRD status when runtime translation errors occur
var (
	backendStatusClient   kclient.Client[*v1alpha1.Backend]
	backendStatusClientMu sync.RWMutex
)

// setBackendStatusClient sets the backend status client in a thread-safe manner
func setBackendStatusClient(c kclient.Client[*v1alpha1.Backend]) {
	backendStatusClientMu.Lock()
	defer backendStatusClientMu.Unlock()
	backendStatusClient = c
}

// getBackendStatusClient gets the backend status client in a thread-safe manner
func getBackendStatusClient() kclient.Client[*v1alpha1.Backend] {
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
	err := retry.Do(
		func() error {
			cur := cl.Get(resNN.Name, resNN.Namespace)
			if cur == nil {
				logger.Error("error getting backend", "ref", resNN, "error", pluginsdk.ErrNotFound)
				return pluginsdk.ErrNotFound
			}

			newCondition := pluginutils.BuildCondition("Backend", errs)

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

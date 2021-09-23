package ratelimit

import (
	"reflect"

	"github.com/hashicorp/go-multierror"
	"github.com/solo-io/solo-kit/pkg/utils/statusutils"

	"github.com/solo-io/solo-kit/pkg/api/v1/resources"

	skres "github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/crd/solo.io/v1"
	"github.com/solo-io/solo-kit/pkg/utils/protoutils"

	types "github.com/solo-io/solo-apis/pkg/api/ratelimit.solo.io/v1alpha1"

	"github.com/solo-io/solo-apis/pkg/api/ratelimit.solo.io/v1alpha1"

	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/utils/kubeutils"
)

var _ resources.CustomInputResource = &RateLimitConfig{}

type RateLimitConfig v1alpha1.RateLimitConfig

func (r *RateLimitConfig) GetMetadata() *core.Metadata {
	return kubeutils.FromKubeMeta(r.ObjectMeta, true)
}

func (r *RateLimitConfig) SetMetadata(meta *core.Metadata) {
	r.ObjectMeta = kubeutils.ToKubeMeta(meta)
}

func (r *RateLimitConfig) Equal(that interface{}) bool {
	return reflect.DeepEqual(r, that)
}

func (r *RateLimitConfig) Clone() *RateLimitConfig {
	ci := v1alpha1.RateLimitConfig(*r)
	ciCopy := ci.DeepCopy()
	newCi := RateLimitConfig(*ciCopy)
	return &newCi
}

func (r *RateLimitConfig) UnmarshalSpec(spec skres.Spec) error {
	return protoutils.UnmarshalMapToProto(spec, &r.Spec)
}

func (r *RateLimitConfig) MarshalSpec() (skres.Spec, error) {
	return protoutils.MarshalMapFromProto(&r.Spec)
}

func (r *RateLimitConfig) UnmarshalStatus(status skres.Status, unmarshaler resources.StatusUnmarshaler) error {
	// First, attempt to unmarshal the status as a map of statuses
	// We do this first, since we will persist the status as a map moving forward
	namespacedStatuses := v1alpha1.RateLimitConfigNamespacedStatuses{}
	namespacedStatusesErr := protoutils.UnmarshalMapToProto(status, &namespacedStatuses)
	if namespacedStatusesErr == nil {
		r.Status = namespacedStatuses
	}

	// If it failed, try to unmarshal as a single status
	singleStatus := v1alpha1.RateLimitConfigStatus{}
	singleStatusErr := protoutils.UnmarshalMapToProto(status, &singleStatus)
	if singleStatusErr == nil {
		// Handle the case where we have a single status, and we need to convert this resource
		// to use a map. This should only happen one time
		// We intentionally do not set the status, so the controller assumes the resource
		// needs to be re-synced
		return nil
	}

	// There's actually something wrong if either status can't be unmarshalled.
	var multiErr *multierror.Error
	multiErr = multierror.Append(multiErr, namespacedStatusesErr)
	multiErr = multierror.Append(multiErr, singleStatusErr)
	return multiErr
}

func (r *RateLimitConfig) MarshalStatus() (skres.Status, error) {
	return protoutils.MarshalMapFromProto(&r.Status)
}

// Deprecated
func (r *RateLimitConfig) GetStatus() *core.Status {
	return statusutils.GetSingleStatusInNamespacedStatuses(r)
}

// Deprecated
func (r *RateLimitConfig) SetStatus(status *core.Status) {
	statusutils.SetSingleStatusInNamespacedStatuses(r, status)
}

func (r *RateLimitConfig) GetNamespacedStatuses() *core.NamespacedStatuses {
	// TODO (sam-heilbron): Everytime we get/set the status, we convert it between the solo-kit and rate-limit types
	// We could speed this up by storing an in memory reference of the solo-kit type and only convert
	// it during marshaling and unmarshaling
	return r.convertRateLimitConfigToSoloKitNamespacedStatuses(&r.Status)
}

func (r *RateLimitConfig) SetNamespacedStatuses(namespacedStatuses *core.NamespacedStatuses) {
	r.Status = *r.convertSoloKitToRateLimitConfigNamespacedStatuses(namespacedStatuses)
}

func (r *RateLimitConfig) convertSoloKitToRateLimitConfigNamespacedStatuses(namespacedStatuses *core.NamespacedStatuses) *v1alpha1.RateLimitConfigNamespacedStatuses {
	if namespacedStatuses == nil {
		return nil
	}

	statuses := map[string]*v1alpha1.RateLimitConfigStatus{}
	for ns, status := range namespacedStatuses.GetStatuses() {
		statuses[ns] = r.convertSoloKitToRateLimitConfigStatus(status)
	}

	return &v1alpha1.RateLimitConfigNamespacedStatuses{
		Statuses: statuses,
	}
}

func (r *RateLimitConfig) convertSoloKitToRateLimitConfigStatus(status *core.Status) *v1alpha1.RateLimitConfigStatus {
	if status == nil {
		return nil
	}

	var outputState types.RateLimitConfigStatus_State
	switch status.GetState() {
	case core.Status_Pending:
		outputState = types.RateLimitConfigStatus_PENDING
	case core.Status_Accepted:
		outputState = types.RateLimitConfigStatus_ACCEPTED
	case core.Status_Rejected:
		outputState = types.RateLimitConfigStatus_REJECTED
	case core.Status_Warning:
		// should lever happen
		panic("cannot set WARNING status on RateLimitConfig resources")
	}

	return &v1alpha1.RateLimitConfigStatus{
		State:              outputState,
		Message:            status.GetReason(),
		ObservedGeneration: r.GetGeneration(),
	}
}

func (r *RateLimitConfig) convertRateLimitConfigToSoloKitNamespacedStatuses(namespacedStatuses *v1alpha1.RateLimitConfigNamespacedStatuses) *core.NamespacedStatuses {
	if namespacedStatuses == nil {
		return nil
	}

	statuses := map[string]*core.Status{}
	for ns, status := range namespacedStatuses.GetStatuses() {
		statuses[ns] = r.convertRateLimitConfigStatusToSoloKitStatus(status)
	}

	return &core.NamespacedStatuses{
		Statuses: statuses,
	}
}

func (r *RateLimitConfig) convertRateLimitConfigStatusToSoloKitStatus(status *v1alpha1.RateLimitConfigStatus) *core.Status {
	if status == nil {
		return nil
	}

	var outputState core.Status_State
	switch status.GetState() {
	case types.RateLimitConfigStatus_PENDING:
		outputState = core.Status_Pending
	case types.RateLimitConfigStatus_ACCEPTED:
		outputState = core.Status_Accepted
	case types.RateLimitConfigStatus_REJECTED:
		outputState = core.Status_Rejected
	}

	return &core.Status{
		State:  outputState,
		Reason: status.GetMessage(),
	}
}

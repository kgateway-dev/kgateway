package ratelimit

import (
	"reflect"

	skres "github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/crd/solo.io/v1"
	"github.com/solo-io/solo-kit/pkg/utils/protoutils"

	"github.com/solo-io/solo-apis/pkg/api/ratelimit.solo.io/v1alpha1/types"

	"github.com/solo-io/solo-apis/pkg/api/ratelimit.solo.io/v1alpha1"

	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/utils/kubeutils"
)

type RateLimitConfig v1alpha1.RateLimitConfig

func (r *RateLimitConfig) GetMetadata() core.Metadata {
	return kubeutils.FromKubeMeta(r.ObjectMeta)
}

func (r *RateLimitConfig) SetMetadata(meta core.Metadata) {
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

func (r *RateLimitConfig) UnmarshalStatus(status skres.Status) error {
	return protoutils.UnmarshalMapToProto(status, &r.Status)
}

func (r *RateLimitConfig) MarshalSpec() (skres.Spec, error) {
	return protoutils.MarshalMapFromProto(&r.Spec)
}

func (r *RateLimitConfig) MarshalStatus() (skres.Status, error) {
	return protoutils.MarshalMapFromProto(&r.Status)
}

func (r *RateLimitConfig) GetStatus() core.Status {

	var outputState core.Status_State

	switch r.Status.GetState() {
	case types.RateLimitConfigStatus_PENDING:
		outputState = core.Status_Pending
	case types.RateLimitConfigStatus_ACCEPTED:
		outputState = core.Status_Accepted
	case types.RateLimitConfigStatus_REJECTED:
		outputState = core.Status_Rejected
	}

	return core.Status{
		State:  outputState,
		Reason: r.Status.GetMessage(),
	}
}

func (r *RateLimitConfig) SetStatus(status core.Status) {

	var outputState types.RateLimitConfigStatus_State

	switch status.State {
	case core.Status_Pending:
		outputState = types.RateLimitConfigStatus_PENDING
	case core.Status_Accepted:
		outputState = types.RateLimitConfigStatus_ACCEPTED
	case core.Status_Rejected:
		outputState = types.RateLimitConfigStatus_REJECTED
	case core.Status_Warning:
		// TODO(marco): is this mapping ok?
		outputState = types.RateLimitConfigStatus_ACCEPTED
	}

	r.Status.State = outputState
	r.Status.Message = status.Reason
	r.Status.ObservedGeneration = r.Generation
}

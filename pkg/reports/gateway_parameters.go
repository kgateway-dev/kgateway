package reports

import (
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
)

// GatewayParametersReport is the Gateway deployer's verdict on one generation of a Gateway's
// infrastructure parameters: whether its objects could be built from them. It reaches the
// Gateway status writer as a status contribution of its own source kind, which keeps that
// writer the only writer of the Gateway's Accepted condition.
type GatewayParametersReport struct {
	// observedGeneration is the Gateway generation the deployer reconciled.
	observedGeneration int64
	// err is why the objects could not be built; empty when they could.
	err string
}

// ObservedGeneration is the Gateway generation the deployer reconciled.
func (r *GatewayParametersReport) ObservedGeneration() int64 {
	return r.observedGeneration
}

// Err is why the Gateway's objects could not be built, or empty if they could.
func (r *GatewayParametersReport) Err() string {
	return r.err
}

func gatewayParametersReportEqual(a, b *GatewayParametersReport) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.observedGeneration == b.observedGeneration && a.err == b.err
}

func cloneGatewayParametersReport(in *GatewayParametersReport) *GatewayParametersReport {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

var gatewayParametersSource = StatusSource{Kind: GatewayParametersStatusSource, Name: "deployer"}

func gatewayParametersContribution(gw types.NamespacedName, report *GatewayParametersReport) StatusContribution {
	return StatusContribution{
		Target:       StatusKey{GroupKind: wellknown.GatewayGVK.GroupKind(), NamespacedName: gw},
		Source:       gatewayParametersSource,
		StatusReport: StatusReport{GatewayParameters: report},
	}
}

// GatewayParametersStatus is where the Gateway deployer publishes its verdict on each
// Gateway it reconciles. Its Contributions join the status contribution pipeline.
//
// A Gateway without an entry has not been reconciled by this replica's deployer yet; the
// status writer then keeps whatever InvalidParameters verdict the Gateway already carries
// rather than clearing it. That is what lets a new leader's status writer start before the
// deployer's first pass without the two disagreeing, and without either waiting on the other.
//
// All methods are no-ops on a nil receiver, so a deployer built without one reports nothing.
type GatewayParametersStatus struct {
	col krt.StaticCollection[StatusContribution]
}

func NewGatewayParametersStatus(opts ...krt.CollectionOption) *GatewayParametersStatus {
	return &GatewayParametersStatus{col: krt.NewMutableCollection[StatusContribution](nil, nil, opts...)}
}

// Contributions returns the deployer's status contributions. It is always synced: an entry
// that has not been published yet is handled by the status writer, not by a sync barrier.
func (s *GatewayParametersStatus) Contributions() krt.Collection[StatusContribution] {
	return s.col.AsCollection()
}

// Set records the deployer's verdict on generation of gw: err is why its objects could not be
// built from its parameters, or nil if they could. Setting an unchanged verdict is a no-op, so
// deployer retries do not reach the status writer.
func (s *GatewayParametersStatus) Set(gw types.NamespacedName, generation int64, err error) {
	if s == nil {
		return
	}
	report := &GatewayParametersReport{observedGeneration: generation}
	if err != nil {
		report.err = err.Error()
	}
	s.col.ConditionalUpdateObject(gatewayParametersContribution(gw, report))
}

// Remove forgets gw, for a Gateway that is gone or no longer ours.
func (s *GatewayParametersStatus) Remove(gw types.NamespacedName) {
	if s == nil {
		return
	}
	s.col.DeleteObject(gatewayParametersContribution(gw, nil).ResourceName())
}

// gatewayAccepted applies the deployer's verdict to the Accepted condition the report would
// otherwise publish, in this order of precedence:
//
//  1. The deployer has reconciled this generation (or a later one) and failed:
//     Accepted=False/InvalidParameters, at the generation the deployer reconciled. This wins
//     over every Accepted the report or BuildGWStatus derived, including ListenersNotValid.
//  2. The deployer has reconciled this generation and succeeded: the report's own Accepted.
//  3. The deployer has not reconciled this generation yet (a new leader before its first pass,
//     or a spec edit it has not caught up with): an InvalidParameters verdict already on the
//     Gateway is kept as it is, generation included, until the deployer says otherwise.
//
// In cases 1 and 3 the condition carries its own ObservedGeneration, which BuildGWStatus keeps.
func gatewayAccepted(
	conditions []metav1.Condition,
	gw *gwv1.Gateway,
	reportGeneration int64,
	params *GatewayParametersReport,
) []metav1.Condition {
	if params != nil && params.observedGeneration >= reportGeneration {
		if params.err != "" {
			meta.SetStatusCondition(&conditions, metav1.Condition{
				Type:               string(gwv1.GatewayConditionAccepted),
				Status:             metav1.ConditionFalse,
				Reason:             string(gwv1.GatewayReasonInvalidParameters),
				Message:            params.err,
				ObservedGeneration: params.observedGeneration,
			})
		}
		return conditions
	}
	live := meta.FindStatusCondition(gw.Status.Conditions, string(gwv1.GatewayConditionAccepted))
	if !isInvalidParameters(live) {
		return conditions
	}
	meta.SetStatusCondition(&conditions, *live)
	return conditions
}

func isInvalidParameters(cond *metav1.Condition) bool {
	return cond != nil &&
		cond.Status == metav1.ConditionFalse &&
		cond.Reason == string(gwv1.GatewayReasonInvalidParameters)
}

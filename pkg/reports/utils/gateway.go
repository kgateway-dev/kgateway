package utils

import (
	"fmt"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	sdkreporter "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

const (
	// ListenerOmittedMessage is a fallback message for when no specific reason was provided in translation
	ListenerOmittedMessage                      = "Listener could not be generated for data plane for unknown reason"
	GatewayAcceptedListenersOmittedMessage      = "Listeners not accepted: %s"
	GatewayProgrammedListenersOmittedMessage    = "Listeners not programmed: %s"
	GatewayProgrammedAllListenersOmittedMessage = "No Listeners programmed. " + GatewayProgrammedListenersOmittedMessage
	GatewayAcceptedAllListenersOmittedMessage   = "All Listeners not accepted: %s"
)

var logger = logging.New("reports/utils/gateway")

// ReportGatewayStatusForInvalidListeners sets gateway conditions when listeners were omitted
// notAcceptedListeners should be a subset of notProgrammedListeners, but that is not validated
func ReportGatewayStatusForInvalidListeners(
	gw ir.GatewayIR,
	reporter sdkreporter.Reporter,
	notAcceptedListeners []string,
	notProgrammedListeners []string,
) {
	// If there are no listeners that were not programmed, there is nothing to report
	if len(notProgrammedListeners) == 0 {
		return
	}

	gwreporter := reporter.Gateway(gw.SourceObject.Obj)

	// Sort for idempotency
	sort.Strings(notAcceptedListeners)
	sort.Strings(notProgrammedListeners)

	// Set Accepted condition - if there are listeners that were explicitly not accepted,
	// mark gateway as not accepted. Otherwise, mark as accepted but mention omitted listeners.
	if len(notAcceptedListeners) > 0 {
		var (
			acceptedMessage string
			status          metav1.ConditionStatus
		)
		if len(notAcceptedListeners) == len(gw.Listeners) {
			acceptedMessage = fmt.Sprintf(GatewayAcceptedAllListenersOmittedMessage, strings.Join(notAcceptedListeners, ", "))
			status = metav1.ConditionFalse
		} else {
			acceptedMessage = fmt.Sprintf(GatewayAcceptedListenersOmittedMessage, strings.Join(notAcceptedListeners, ", "))
			status = metav1.ConditionTrue
		}

		acceptedCondition := sdkreporter.GatewayCondition{
			Type:    gwv1.GatewayConditionAccepted,
			Status:  status,
			Reason:  gwv1.GatewayReasonListenersNotValid,
			Message: acceptedMessage,
		}
		gwreporter.SetCondition(acceptedCondition)
	} else {
		// Gateway is accepted, but some listeners were omitted (just not programmed)
		acceptedCondition := sdkreporter.GatewayCondition{
			Type:    gwv1.GatewayConditionAccepted,
			Status:  metav1.ConditionTrue,
			Reason:  gwv1.GatewayReasonListenersNotValid,
			Message: fmt.Sprintf(GatewayAcceptedListenersOmittedMessage, strings.Join(notProgrammedListeners, ", ")),
		}
		gwreporter.SetCondition(acceptedCondition)
	}

	// Set Programmed condition based on whether any listeners were programmed
	var programmedMessage string
	if len(notProgrammedListeners) == len(gw.Listeners) {
		programmedMessage = fmt.Sprintf(GatewayProgrammedAllListenersOmittedMessage, strings.Join(notProgrammedListeners, ", "))
	} else {
		programmedMessage = fmt.Sprintf(GatewayProgrammedListenersOmittedMessage, strings.Join(notProgrammedListeners, ", "))
	}

	if len(notProgrammedListeners) == len(gw.Listeners) {
		programmedCondition := sdkreporter.GatewayCondition{
			Type:    gwv1.GatewayConditionProgrammed,
			Status:  metav1.ConditionFalse,
			Reason:  gwv1.GatewayReasonListenersNotValid,
			Message: programmedMessage,
		}
		gwreporter.SetCondition(programmedCondition)
	} else {
		programmedCondition := sdkreporter.GatewayCondition{
			Type:    gwv1.GatewayConditionProgrammed,
			Status:  metav1.ConditionTrue,
			Reason:  gwv1.GatewayReasonProgrammed,
			Message: programmedMessage,
		}
		gwreporter.SetCondition(programmedCondition)
	}
}

// CheckInvalidListenerProgrammedCondition checks for a Programmed condition of status false.
// If the condition exists, return true. If the condition doesn't exist, report the default
// programmedCondition and return false.
func CheckInvalidListenerProgrammedCondition(
	listenerReporter sdkreporter.ListenerReporter,
) bool {
	if lr, ok := listenerReporter.(*reports.ListenerReport); ok {
		programmedCondition := meta.FindStatusCondition(lr.Status.Conditions, string(gwv1.ListenerConditionProgrammed))

		// If programmed condition exists and is false, return true
		if programmedCondition != nil && programmedCondition.Status == metav1.ConditionFalse {
			return true
		}

		// If programmed condition doesn't exist, set default and return false
		if programmedCondition == nil {
			listenerCondition := sdkreporter.ListenerCondition{
				Type:    gwv1.ListenerConditionProgrammed,
				Status:  metav1.ConditionFalse,
				Reason:  gwv1.ListenerReasonInvalid,
				Message: ListenerOmittedMessage,
			}
			listenerReporter.SetCondition(listenerCondition)
			return false
		}

		// Programmed condition exists and is true, return true (shouldn't happen in invalid listener context)
		return true
	} else {
		logger.Error("listener reporter type not supported", "reporter", fmt.Sprintf("%T", listenerReporter))
		return false
	}
}

// ListenerAccepted returns true if there is no Accepted condition for the listener.
func ListenerAccepted(listenerReporter sdkreporter.ListenerReporter) bool {
	if lr, ok := listenerReporter.(*reports.ListenerReport); ok {
		acceptedCondition := meta.FindStatusCondition(lr.Status.Conditions, string(gwv1.ListenerConditionAccepted))

		// Return true if no accepted condition exists or if it exists and is True
		return acceptedCondition == nil || acceptedCondition.Status == metav1.ConditionTrue
	} else {
		logger.Error("listener reporter type not supported", "reporter", fmt.Sprintf("%T", listenerReporter))
		return false
	}
}

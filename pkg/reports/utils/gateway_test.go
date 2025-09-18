package utils

import (
	"testing"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	sdkreporter "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

func TestCheckInvalidListenerProgrammedCondition(t *testing.T) {
	tests := []struct {
		name                      string
		setupListenerReport       func() *reports.ListenerReport
		expectedResult            bool
		expectedProgrammedPresent bool
		expectedProgrammedStatus  metav1.ConditionStatus
		expectedProgrammedReason  string
		expectedProgrammedMessage string
	}{
		{
			name: "programmed condition exists and is false",
			setupListenerReport: func() *reports.ListenerReport {
				lr := reports.NewListenerReport("test-listener")
				programmedCondition := metav1.Condition{
					Type:    string(gwv1.ListenerConditionProgrammed),
					Status:  metav1.ConditionFalse,
					Reason:  string(gwv1.ListenerReasonUnsupportedProtocol),
					Message: "Custom message",
				}
				meta.SetStatusCondition(&lr.Status.Conditions, programmedCondition)
				return lr
			},
			expectedResult:            true,
			expectedProgrammedPresent: true,
			expectedProgrammedStatus:  metav1.ConditionFalse,
			expectedProgrammedReason:  string(gwv1.ListenerReasonUnsupportedProtocol),
			expectedProgrammedMessage: "Custom message",
		},
		{
			name: "programmed condition exists and is true",
			setupListenerReport: func() *reports.ListenerReport {
				lr := reports.NewListenerReport("test-listener")
				programmedCondition := metav1.Condition{
					Type:    string(gwv1.ListenerConditionProgrammed),
					Status:  metav1.ConditionTrue,
					Reason:  string(gwv1.ListenerReasonProgrammed),
					Message: "Already programmed",
				}
				meta.SetStatusCondition(&lr.Status.Conditions, programmedCondition)
				return lr
			},
			expectedResult:            true,
			expectedProgrammedPresent: true,
			expectedProgrammedStatus:  metav1.ConditionTrue,
			expectedProgrammedReason:  string(gwv1.ListenerReasonProgrammed),
			expectedProgrammedMessage: "Already programmed",
		},
		{
			name: "no programmed condition - sets default",
			setupListenerReport: func() *reports.ListenerReport {
				return reports.NewListenerReport("test-listener")
			},
			expectedResult:            false,
			expectedProgrammedPresent: true,
			expectedProgrammedStatus:  metav1.ConditionFalse,
			expectedProgrammedReason:  string(gwv1.ListenerReasonInvalid),
			expectedProgrammedMessage: ListenerOmittedMessage,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			listenerReport := tt.setupListenerReport()

			result := CheckInvalidListenerProgrammedCondition(listenerReport)

			if result != tt.expectedResult {
				t.Errorf("CheckInvalidListenerProgrammedCondition() result = %v, want %v", result, tt.expectedResult)
			}

			programmedCondition := meta.FindStatusCondition(listenerReport.Status.Conditions, string(gwv1.ListenerConditionProgrammed))

			if tt.expectedProgrammedPresent {
				if programmedCondition == nil {
					t.Fatalf("Expected programmed condition to be present, but it was nil")
				}

				if programmedCondition.Status != tt.expectedProgrammedStatus {
					t.Errorf("Programmed condition status = %v, want %v", programmedCondition.Status, tt.expectedProgrammedStatus)
				}

				if programmedCondition.Reason != tt.expectedProgrammedReason {
					t.Errorf("Programmed condition reason = %v, want %v", programmedCondition.Reason, tt.expectedProgrammedReason)
				}

				if programmedCondition.Message != tt.expectedProgrammedMessage {
					t.Errorf("Programmed condition message = %v, want %v", programmedCondition.Message, tt.expectedProgrammedMessage)
				}
			} else if programmedCondition != nil {
				t.Errorf("Expected no programmed condition, but got one")
			}
		})
	}
}

func TestCheckInvalidListenerProgrammedCondition_NonListenerReport(t *testing.T) {
	mockReporter := &mockListenerReporter{}

	result := CheckInvalidListenerProgrammedCondition(mockReporter)

	if result != false {
		t.Errorf("Expected result to be false for non-ListenerReport, got %v", result)
	}
}

func TestListenerAccepted(t *testing.T) {
	tests := []struct {
		name                string
		setupListenerReport func() *reports.ListenerReport
		expectedResult      bool
	}{
		{
			name: "no accepted condition - listener accepted by default",
			setupListenerReport: func() *reports.ListenerReport {
				return reports.NewListenerReport("test-listener")
			},
			expectedResult: true,
		},
		{
			name: "accepted condition exists and is true",
			setupListenerReport: func() *reports.ListenerReport {
				lr := reports.NewListenerReport("test-listener")
				acceptedCondition := metav1.Condition{
					Type:   string(gwv1.ListenerConditionAccepted),
					Status: metav1.ConditionTrue,
					Reason: string(gwv1.ListenerReasonAccepted),
				}
				meta.SetStatusCondition(&lr.Status.Conditions, acceptedCondition)
				return lr
			},
			expectedResult: true, // Function returns true if accepted condition is True
		},
		{
			name: "accepted condition exists and is false",
			setupListenerReport: func() *reports.ListenerReport {
				lr := reports.NewListenerReport("test-listener")
				acceptedCondition := metav1.Condition{
					Type:   string(gwv1.ListenerConditionAccepted),
					Status: metav1.ConditionFalse,
					Reason: string(gwv1.ListenerReasonInvalid),
				}
				meta.SetStatusCondition(&lr.Status.Conditions, acceptedCondition)
				return lr
			},
			expectedResult: false, // Function returns false only if accepted condition is False
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			listenerReport := tt.setupListenerReport()

			result := ListenerAccepted(listenerReport)

			if result != tt.expectedResult {
				t.Errorf("ListenerAccepted() result = %v, want %v", result, tt.expectedResult)
			}
		})
	}
}

func TestListenerAccepted_NonListenerReport(t *testing.T) {
	mockReporter := &mockListenerReporter{}

	result := ListenerAccepted(mockReporter)

	if result != false {
		t.Errorf("Expected result to be false for non-ListenerReport, got %v", result)
	}
}

func TestConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{
			name:     "ListenerOmittedMessage",
			constant: ListenerOmittedMessage,
			expected: "Listener could not be generated for data plane for unknown reason",
		},
		{
			name:     "GatewayAcceptedListenersOmittedMessage",
			constant: GatewayAcceptedListenersOmittedMessage,
			expected: "Listeners not accepted: %s",
		},
		{
			name:     "GatewayProgrammedListenersOmittedMessage",
			constant: GatewayProgrammedListenersOmittedMessage,
			expected: "Listeners not programmed: %s",
		},
		{
			name:     "GatewayProgrammedAllListenersOmittedMessage",
			constant: GatewayProgrammedAllListenersOmittedMessage,
			expected: "No Listeners programmed. Listeners not programmed: %s",
		},
		{
			name:     "GatewayAcceptedAllListenersOmittedMessage",
			constant: GatewayAcceptedAllListenersOmittedMessage,
			expected: "All Listeners not accepted: %s",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant != tt.expected {
				t.Errorf("Constant %s = %v, want %v", tt.name, tt.constant, tt.expected)
			}
		})
	}
}

type mockListenerReporter struct {
	conditions []sdkreporter.ListenerCondition
}

func (m *mockListenerReporter) SetCondition(condition sdkreporter.ListenerCondition) {
	m.conditions = append(m.conditions, condition)
}

func (m *mockListenerReporter) SetSupportedKinds([]gwv1.RouteGroupKind) {}

func (m *mockListenerReporter) SetAttachedRoutes(n uint) {}

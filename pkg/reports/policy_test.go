package reports

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
)

func TestPolicyStatusReport(t *testing.T) {
	tests := []struct {
		name            string
		fakeTranslation func(a *assert.Assertions, reporter reporter.Reporter)
		key             reporter.PolicyKey
		currentStatus   gwv1alpha2.PolicyStatus
		controller      string
		wantStatus      *gwv1alpha2.PolicyStatus
	}{
		{
			name: "empty status on current object and no status updates during translation",
			fakeTranslation: func(a *assert.Assertions, statusReporter reporter.Reporter) {
				policyReport := statusReporter.Policy(reporter.PolicyKey{
					Group:     "example.com",
					Kind:      "Policy",
					Namespace: "default",
					Name:      "example",
				}, 1)
				a.NotNil(policyReport)
				// during gw-1 translation, reporter will default to positive conditions
				policyReport.AncestorRef(gwv1.ParentReference{
					Group:     ptr.To(gwv1.Group("gateway.networking.k8s.io")),
					Kind:      ptr.To(gwv1.Kind("Gateway")),
					Namespace: ptr.To(gwv1.Namespace("default")),
					Name:      gwv1.ObjectName("gw-1"),
				})
				// during gw-2 translation, reporter will default to positive conditions
				policyReport.AncestorRef(gwv1.ParentReference{
					Group:     ptr.To(gwv1.Group("gateway.networking.k8s.io")),
					Kind:      ptr.To(gwv1.Kind("Gateway")),
					Namespace: ptr.To(gwv1.Namespace("default")),
					Name:      gwv1.ObjectName("gw-2"),
				})
			},
			key: reporter.PolicyKey{
				Group:     "example.com",
				Kind:      "Policy",
				Namespace: "default",
				Name:      "example",
			},
			controller: "example-controller",
			wantStatus: &gwv1alpha2.PolicyStatus{
				Ancestors: []gwv1alpha2.PolicyAncestorStatus{
					{
						AncestorRef: gwv1.ParentReference{
							Group:     ptr.To(gwv1.Group("gateway.networking.k8s.io")),
							Kind:      ptr.To(gwv1.Kind("Gateway")),
							Namespace: ptr.To(gwv1.Namespace("default")),
							Name:      gwv1.ObjectName("gw-1"),
						},
						ControllerName: "example-controller",
						Conditions: []metav1.Condition{
							{
								ObservedGeneration: 1,
								Type:               string(v1alpha1.PolicyConditionAccepted),
								Status:             metav1.ConditionFalse,
								Reason:             string(v1alpha1.PolicyReasonPending),
							},
							{
								ObservedGeneration: 1,
								Type:               string(v1alpha1.PolicyConditionAttached),
								Status:             metav1.ConditionFalse,
								Reason:             string(v1alpha1.PolicyReasonPending),
							},
						},
					},
					{
						AncestorRef: gwv1.ParentReference{
							Group:     ptr.To(gwv1.Group("gateway.networking.k8s.io")),
							Kind:      ptr.To(gwv1.Kind("Gateway")),
							Namespace: ptr.To(gwv1.Namespace("default")),
							Name:      gwv1.ObjectName("gw-2"),
						},
						ControllerName: "example-controller",
						Conditions: []metav1.Condition{
							{
								ObservedGeneration: 1,
								Type:               string(v1alpha1.PolicyConditionAccepted),
								Status:             metav1.ConditionFalse,
								Reason:             string(v1alpha1.PolicyReasonPending),
							},
							{
								ObservedGeneration: 1,
								Type:               string(v1alpha1.PolicyConditionAttached),
								Status:             metav1.ConditionFalse,
								Reason:             string(v1alpha1.PolicyReasonPending),
							},
						},
					},
				},
			},
		},
		{
			name: "status on existing object and status updates during translation",
			fakeTranslation: func(a *assert.Assertions, statusReporter reporter.Reporter) {
				policyReport := statusReporter.Policy(reporter.PolicyKey{
					Group:     "example.com",
					Kind:      "Policy",
					Namespace: "default",
					Name:      "example",
				}, 2)
				a.NotNil(policyReport)
				// during gw-1 translation, add PolicyReasonValid
				policyReport.AncestorRef(gwv1.ParentReference{
					Group:     ptr.To(gwv1.Group("gateway.networking.k8s.io")),
					Kind:      ptr.To(gwv1.Kind("Gateway")),
					Namespace: ptr.To(gwv1.Namespace("default")),
					Name:      gwv1.ObjectName("gw-1"),
				}).SetCondition(reporter.PolicyCondition{
					Type:   string(v1alpha1.PolicyConditionAccepted),
					Status: metav1.ConditionTrue,
					Reason: string(v1alpha1.PolicyReasonValid),
				})
				// during gw-1 translation, add PolicyReasonAttached
				policyReport.AncestorRef(gwv1.ParentReference{
					Group:     ptr.To(gwv1.Group("gateway.networking.k8s.io")),
					Kind:      ptr.To(gwv1.Kind("Gateway")),
					Namespace: ptr.To(gwv1.Namespace("default")),
					Name:      gwv1.ObjectName("gw-1"),
				}).SetAttachmentState(reporter.PolicyAttachmentStateAttached)
				// during gw-2 translation, add PolicyReasonInvalid
				policyReport.AncestorRef(gwv1.ParentReference{
					Group:     ptr.To(gwv1.Group("gateway.networking.k8s.io")),
					Kind:      ptr.To(gwv1.Kind("Gateway")),
					Namespace: ptr.To(gwv1.Namespace("default")),
					Name:      gwv1.ObjectName("gw-2"),
				}).SetCondition(reporter.PolicyCondition{
					Type:   string(v1alpha1.PolicyConditionAccepted),
					Status: metav1.ConditionFalse,
					Reason: string(v1alpha1.PolicyReasonInvalid),
				})
			},
			key: reporter.PolicyKey{
				Group:     "example.com",
				Kind:      "Policy",
				Namespace: "default",
				Name:      "example",
			},
			controller: "example-controller",
			currentStatus: gwv1alpha2.PolicyStatus{
				Ancestors: []gwv1alpha2.PolicyAncestorStatus{
					// No existing status for gw-1 but test with an existing status for gw-2
					{
						AncestorRef: gwv1.ParentReference{
							Group:     ptr.To(gwv1.Group("gateway.networking.k8s.io")),
							Kind:      ptr.To(gwv1.Kind("Gateway")),
							Namespace: ptr.To(gwv1.Namespace("default")),
							Name:      gwv1.ObjectName("gw-2"),
						},
						ControllerName: "example-controller",
						Conditions: []metav1.Condition{
							{
								ObservedGeneration: 1,
								Type:               string(v1alpha1.PolicyConditionAccepted),
								Status:             metav1.ConditionTrue,
								Reason:             string(v1alpha1.PolicyReasonValid),
							},
						},
					},
				},
			},
			wantStatus: &gwv1alpha2.PolicyStatus{
				Ancestors: []gwv1alpha2.PolicyAncestorStatus{
					{
						AncestorRef: gwv1.ParentReference{
							Group:     ptr.To(gwv1.Group("gateway.networking.k8s.io")),
							Kind:      ptr.To(gwv1.Kind("Gateway")),
							Namespace: ptr.To(gwv1.Namespace("default")),
							Name:      gwv1.ObjectName("gw-1"),
						},
						ControllerName: "example-controller",
						Conditions: []metav1.Condition{
							{
								ObservedGeneration: 2,
								Type:               string(v1alpha1.PolicyConditionAccepted),
								Status:             metav1.ConditionTrue,
								Reason:             string(v1alpha1.PolicyReasonValid),
							},
							{
								ObservedGeneration: 2,
								Type:               string(v1alpha1.PolicyConditionAttached),
								Status:             metav1.ConditionTrue,
								Reason:             string(v1alpha1.PolicyReasonAttached),
								Message:            reporter.PolicyAttachedMsg,
							},
						},
					},
					{
						AncestorRef: gwv1.ParentReference{
							Group:     ptr.To(gwv1.Group("gateway.networking.k8s.io")),
							Kind:      ptr.To(gwv1.Kind("Gateway")),
							Namespace: ptr.To(gwv1.Namespace("default")),
							Name:      gwv1.ObjectName("gw-2"),
						},
						ControllerName: "example-controller",
						Conditions: []metav1.Condition{
							{
								ObservedGeneration: 2,
								Type:               string(v1alpha1.PolicyConditionAccepted),
								Status:             metav1.ConditionFalse,
								Reason:             string(v1alpha1.PolicyReasonInvalid),
							},
							{
								ObservedGeneration: 2,
								Type:               string(v1alpha1.PolicyConditionAttached),
								Status:             metav1.ConditionFalse,
								Reason:             string(v1alpha1.PolicyReasonPending),
							},
						},
					},
				},
			},
		},
		{
			name: "preserve ancestor status belonging to external controllers",
			fakeTranslation: func(a *assert.Assertions, statusReporter reporter.Reporter) {
				policyReport := statusReporter.Policy(reporter.PolicyKey{
					Group:     "example.com",
					Kind:      "Policy",
					Namespace: "default",
					Name:      "example",
				}, 2)
				a.NotNil(policyReport)
				// during gw-1 translation, add PolicyReasonValid
				policyReport.AncestorRef(gwv1.ParentReference{
					Group:     ptr.To(gwv1.Group("gateway.networking.k8s.io")),
					Kind:      ptr.To(gwv1.Kind("Gateway")),
					Namespace: ptr.To(gwv1.Namespace("default")),
					Name:      gwv1.ObjectName("gw-1"),
				}).SetCondition(reporter.PolicyCondition{
					Type:   string(v1alpha1.PolicyConditionAccepted),
					Status: metav1.ConditionTrue,
					Reason: string(v1alpha1.PolicyReasonValid),
				})
				// during gw-2 translation, add PolicyReasonInvalid
				policyReport.AncestorRef(gwv1.ParentReference{
					Group:     ptr.To(gwv1.Group("gateway.networking.k8s.io")),
					Kind:      ptr.To(gwv1.Kind("Gateway")),
					Namespace: ptr.To(gwv1.Namespace("default")),
					Name:      gwv1.ObjectName("gw-2"),
				}).SetCondition(reporter.PolicyCondition{
					Type:   string(v1alpha1.PolicyConditionAccepted),
					Status: metav1.ConditionFalse,
					Reason: string(v1alpha1.PolicyReasonInvalid),
				})
			},
			key: reporter.PolicyKey{
				Group:     "example.com",
				Kind:      "Policy",
				Namespace: "default",
				Name:      "example",
			},
			controller: "example-controller",
			currentStatus: gwv1alpha2.PolicyStatus{
				Ancestors: []gwv1alpha2.PolicyAncestorStatus{
					{
						AncestorRef: gwv1.ParentReference{
							Group:     ptr.To(gwv1.Group("gateway.networking.k8s.io")),
							Kind:      ptr.To(gwv1.Kind("Gateway")),
							Namespace: ptr.To(gwv1.Namespace("default")),
							Name:      gwv1.ObjectName("gw-3"),
						},
						ControllerName: "not-our-controller", // not our controller
						Conditions: []metav1.Condition{
							{
								ObservedGeneration: 1,
								Type:               "ExternalType",
								Status:             metav1.ConditionFalse,
								Reason:             "ExternalReason",
							},
						},
					},
					{
						AncestorRef: gwv1.ParentReference{
							Group:     ptr.To(gwv1.Group("gateway.networking.k8s.io")),
							Kind:      ptr.To(gwv1.Kind("Gateway")),
							Namespace: ptr.To(gwv1.Namespace("default")),
							Name:      gwv1.ObjectName("gw-1"),
						},
						ControllerName: "example-controller",
						Conditions: []metav1.Condition{
							{
								ObservedGeneration: 1,
								Type:               string(v1alpha1.PolicyConditionAccepted),
								Status:             metav1.ConditionFalse,
								Reason:             string(v1alpha1.PolicyReasonInvalid),
							},
						},
					},
					{
						AncestorRef: gwv1.ParentReference{
							Group:     ptr.To(gwv1.Group("gateway.networking.k8s.io")),
							Kind:      ptr.To(gwv1.Kind("Gateway")),
							Namespace: ptr.To(gwv1.Namespace("default")),
							Name:      gwv1.ObjectName("gw-2"),
						},
						ControllerName: "example-controller",
						Conditions: []metav1.Condition{
							{
								ObservedGeneration: 1,
								Type:               string(v1alpha1.PolicyConditionAccepted),
								Status:             metav1.ConditionFalse,
								Reason:             string(v1alpha1.PolicyReasonPending),
							},
						},
					},
				},
			},
			wantStatus: &gwv1alpha2.PolicyStatus{
				Ancestors: []gwv1alpha2.PolicyAncestorStatus{
					{
						AncestorRef: gwv1.ParentReference{
							Group:     ptr.To(gwv1.Group("gateway.networking.k8s.io")),
							Kind:      ptr.To(gwv1.Kind("Gateway")),
							Namespace: ptr.To(gwv1.Namespace("default")),
							Name:      gwv1.ObjectName("gw-1"),
						},
						ControllerName: "example-controller",
						Conditions: []metav1.Condition{
							{
								ObservedGeneration: 2,
								Type:               string(v1alpha1.PolicyConditionAccepted),
								Status:             metav1.ConditionTrue,
								Reason:             string(v1alpha1.PolicyReasonValid),
							},
							{
								ObservedGeneration: 2,
								Type:               string(v1alpha1.PolicyConditionAttached),
								Status:             metav1.ConditionFalse,
								Reason:             string(v1alpha1.PolicyReasonPending),
							},
						},
					},
					{
						AncestorRef: gwv1.ParentReference{
							Group:     ptr.To(gwv1.Group("gateway.networking.k8s.io")),
							Kind:      ptr.To(gwv1.Kind("Gateway")),
							Namespace: ptr.To(gwv1.Namespace("default")),
							Name:      gwv1.ObjectName("gw-2"),
						},
						ControllerName: "example-controller",
						Conditions: []metav1.Condition{
							{
								ObservedGeneration: 2,
								Type:               string(v1alpha1.PolicyConditionAccepted),
								Status:             metav1.ConditionFalse,
								Reason:             string(v1alpha1.PolicyReasonInvalid),
							},
							{
								ObservedGeneration: 2,
								Type:               string(v1alpha1.PolicyConditionAttached),
								Status:             metav1.ConditionFalse,
								Reason:             string(v1alpha1.PolicyReasonPending),
							},
						},
					},
					{
						AncestorRef: gwv1.ParentReference{
							Group:     ptr.To(gwv1.Group("gateway.networking.k8s.io")),
							Kind:      ptr.To(gwv1.Kind("Gateway")),
							Namespace: ptr.To(gwv1.Namespace("default")),
							Name:      gwv1.ObjectName("gw-3"),
						},
						ControllerName: "not-our-controller", // not our controller
						Conditions: []metav1.Condition{
							{
								ObservedGeneration: 1,
								Type:               "ExternalType",
								Status:             metav1.ConditionFalse,
								Reason:             "ExternalReason",
							},
						},
					},
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := assert.New(t)

			rm := NewReportMap()
			reporter := NewReporter(&rm)
			if tc.fakeTranslation != nil {
				tc.fakeTranslation(a, reporter)
			}

			gotStatus := rm.BuildPolicyStatus(t.Context(), tc.key, tc.controller, tc.currentStatus)
			diff := cmp.Diff(tc.wantStatus, gotStatus, cmpopts.IgnoreFields(metav1.Condition{}, "LastTransitionTime"))
			a.Empty(diff)
		})
	}
}

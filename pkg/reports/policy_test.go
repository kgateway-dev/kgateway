package reports

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
)

func TestPolicyStatusReport(t *testing.T) {
	tests := []struct {
		name            string
		fakeTranslation func(a *assert.Assertions, reporter reporter.Reporter)
		key             reporter.PolicyKey
		currentStatus   gwv1.PolicyStatus
		controller      string
		wantStatus      *gwv1.PolicyStatus
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
					Group:     new(gwv1.Group("gateway.networking.k8s.io")),
					Kind:      new(gwv1.Kind("Gateway")),
					Namespace: new(gwv1.Namespace("default")),
					Name:      gwv1.ObjectName("gw-1"),
				})
				// during gw-2 translation, reporter will default to positive conditions
				policyReport.AncestorRef(gwv1.ParentReference{
					Group:     new(gwv1.Group("gateway.networking.k8s.io")),
					Kind:      new(gwv1.Kind("Gateway")),
					Namespace: new(gwv1.Namespace("default")),
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
			wantStatus: &gwv1.PolicyStatus{
				Ancestors: []gwv1.PolicyAncestorStatus{
					{
						AncestorRef: gwv1.ParentReference{
							Group:     new(gwv1.Group("gateway.networking.k8s.io")),
							Kind:      new(gwv1.Kind("Gateway")),
							Namespace: new(gwv1.Namespace("default")),
							Name:      gwv1.ObjectName("gw-1"),
						},
						ControllerName: "example-controller",
						Conditions: []metav1.Condition{
							{
								ObservedGeneration: 1,
								Type:               string(shared.PolicyConditionAccepted),
								Status:             metav1.ConditionFalse,
								Reason:             string(shared.PolicyReasonPending),
							},
							{
								ObservedGeneration: 1,
								Type:               string(shared.PolicyConditionAttached),
								Status:             metav1.ConditionFalse,
								Reason:             string(shared.PolicyReasonPending),
							},
						},
					},
					{
						AncestorRef: gwv1.ParentReference{
							Group:     new(gwv1.Group("gateway.networking.k8s.io")),
							Kind:      new(gwv1.Kind("Gateway")),
							Namespace: new(gwv1.Namespace("default")),
							Name:      gwv1.ObjectName("gw-2"),
						},
						ControllerName: "example-controller",
						Conditions: []metav1.Condition{
							{
								ObservedGeneration: 1,
								Type:               string(shared.PolicyConditionAccepted),
								Status:             metav1.ConditionFalse,
								Reason:             string(shared.PolicyReasonPending),
							},
							{
								ObservedGeneration: 1,
								Type:               string(shared.PolicyConditionAttached),
								Status:             metav1.ConditionFalse,
								Reason:             string(shared.PolicyReasonPending),
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
					Group:     new(gwv1.Group("gateway.networking.k8s.io")),
					Kind:      new(gwv1.Kind("Gateway")),
					Namespace: new(gwv1.Namespace("default")),
					Name:      gwv1.ObjectName("gw-1"),
				}).SetCondition(reporter.PolicyCondition{
					Type:   string(shared.PolicyConditionAccepted),
					Status: metav1.ConditionTrue,
					Reason: string(shared.PolicyReasonValid),
				})
				// during gw-1 translation, add PolicyReasonAttached
				policyReport.AncestorRef(gwv1.ParentReference{
					Group:     new(gwv1.Group("gateway.networking.k8s.io")),
					Kind:      new(gwv1.Kind("Gateway")),
					Namespace: new(gwv1.Namespace("default")),
					Name:      gwv1.ObjectName("gw-1"),
				}).SetAttachmentState(reporter.PolicyAttachmentStateAttached)
				// during gw-2 translation, add PolicyReasonInvalid
				policyReport.AncestorRef(gwv1.ParentReference{
					Group:     new(gwv1.Group("gateway.networking.k8s.io")),
					Kind:      new(gwv1.Kind("Gateway")),
					Namespace: new(gwv1.Namespace("default")),
					Name:      gwv1.ObjectName("gw-2"),
				}).SetCondition(reporter.PolicyCondition{
					Type:   string(shared.PolicyConditionAccepted),
					Status: metav1.ConditionFalse,
					Reason: string(shared.PolicyReasonInvalid),
				})
			},
			key: reporter.PolicyKey{
				Group:     "example.com",
				Kind:      "Policy",
				Namespace: "default",
				Name:      "example",
			},
			controller: "example-controller",
			currentStatus: gwv1.PolicyStatus{
				Ancestors: []gwv1.PolicyAncestorStatus{
					// No existing status for gw-1 but test with an existing status for gw-2
					{
						AncestorRef: gwv1.ParentReference{
							Group:     new(gwv1.Group("gateway.networking.k8s.io")),
							Kind:      new(gwv1.Kind("Gateway")),
							Namespace: new(gwv1.Namespace("default")),
							Name:      gwv1.ObjectName("gw-2"),
						},
						ControllerName: "example-controller",
						Conditions: []metav1.Condition{
							{
								ObservedGeneration: 1,
								Type:               string(shared.PolicyConditionAccepted),
								Status:             metav1.ConditionTrue,
								Reason:             string(shared.PolicyReasonValid),
							},
						},
					},
				},
			},
			wantStatus: &gwv1.PolicyStatus{
				Ancestors: []gwv1.PolicyAncestorStatus{
					{
						AncestorRef: gwv1.ParentReference{
							Group:     new(gwv1.Group("gateway.networking.k8s.io")),
							Kind:      new(gwv1.Kind("Gateway")),
							Namespace: new(gwv1.Namespace("default")),
							Name:      gwv1.ObjectName("gw-1"),
						},
						ControllerName: "example-controller",
						Conditions: []metav1.Condition{
							{
								ObservedGeneration: 2,
								Type:               string(shared.PolicyConditionAccepted),
								Status:             metav1.ConditionTrue,
								Reason:             string(shared.PolicyReasonValid),
							},
							{
								ObservedGeneration: 2,
								Type:               string(shared.PolicyConditionAttached),
								Status:             metav1.ConditionTrue,
								Reason:             string(shared.PolicyReasonAttached),
								Message:            reporter.PolicyAttachedMsg,
							},
						},
					},
					{
						AncestorRef: gwv1.ParentReference{
							Group:     new(gwv1.Group("gateway.networking.k8s.io")),
							Kind:      new(gwv1.Kind("Gateway")),
							Namespace: new(gwv1.Namespace("default")),
							Name:      gwv1.ObjectName("gw-2"),
						},
						ControllerName: "example-controller",
						Conditions: []metav1.Condition{
							{
								ObservedGeneration: 2,
								Type:               string(shared.PolicyConditionAccepted),
								Status:             metav1.ConditionFalse,
								Reason:             string(shared.PolicyReasonInvalid),
							},
							{
								ObservedGeneration: 2,
								Type:               string(shared.PolicyConditionAttached),
								Status:             metav1.ConditionFalse,
								Reason:             string(shared.PolicyReasonPending),
							},
						},
					},
				},
			},
		},
		{
			name: "policy partially applied to ancestor (attached on one listener, overridden on another) reports Attached=True with reason Merged",
			fakeTranslation: func(a *assert.Assertions, statusReporter reporter.Reporter) {
				policyReport := statusReporter.Policy(reporter.PolicyKey{
					Group:     "gateway.kgateway.dev",
					Kind:      "ListenerPolicy",
					Namespace: "default",
					Name:      "gateway-wide",
				}, 1)
				a.NotNil(policyReport)
				gwRef := gwv1.ParentReference{
					Group:     new(gwv1.Group("gateway.networking.k8s.io")),
					Kind:      new(gwv1.Kind("Gateway")),
					Namespace: new(gwv1.Namespace("default")),
					Name:      gwv1.ObjectName("gw-1"),
				}
				ancestor := policyReport.AncestorRef(gwRef)
				ancestor.SetCondition(reporter.PolicyCondition{
					Type:   string(shared.PolicyConditionAccepted),
					Status: metav1.ConditionTrue,
					Reason: string(shared.PolicyReasonValid),
				})
				// listener-1 overrides the policy
				ancestor.SetAttachmentState(reporter.PolicyAttachmentStateOverridden)
				// listener-2 applies the policy
				ancestor.SetAttachmentState(reporter.PolicyAttachmentStateAttached)
			},
			key: reporter.PolicyKey{
				Group:     "gateway.kgateway.dev",
				Kind:      "ListenerPolicy",
				Namespace: "default",
				Name:      "gateway-wide",
			},
			controller: "kgateway.dev/kgateway",
			wantStatus: &gwv1.PolicyStatus{
				Ancestors: []gwv1.PolicyAncestorStatus{
					{
						AncestorRef: gwv1.ParentReference{
							Group:     new(gwv1.Group("gateway.networking.k8s.io")),
							Kind:      new(gwv1.Kind("Gateway")),
							Namespace: new(gwv1.Namespace("default")),
							Name:      gwv1.ObjectName("gw-1"),
						},
						ControllerName: "kgateway.dev/kgateway",
						Conditions: []metav1.Condition{
							{
								ObservedGeneration: 1,
								Type:               string(shared.PolicyConditionAccepted),
								Status:             metav1.ConditionTrue,
								Reason:             string(shared.PolicyReasonValid),
							},
							{
								ObservedGeneration: 1,
								Type:               string(shared.PolicyConditionAttached),
								Status:             metav1.ConditionTrue,
								Reason:             string(shared.PolicyReasonMerged),
								Message:            reporter.PolicyMergedMsg,
							},
						},
					},
				},
			},
		},
		{
			name: "status on existing object and report map with empty policy entry during translation",
			fakeTranslation: func(a *assert.Assertions, statusReporter reporter.Reporter) {
				// Policy is added to report map but no ancestor refs are added
				policyReport := statusReporter.Policy(reporter.PolicyKey{
					Group:     "example.com",
					Kind:      "Policy",
					Namespace: "default",
					Name:      "example",
				}, 2)
				a.NotNil(policyReport)
			},
			key: reporter.PolicyKey{
				Group:     "example.com",
				Kind:      "Policy",
				Namespace: "default",
				Name:      "example",
			},
			controller: "example-controller",
			currentStatus: gwv1.PolicyStatus{
				Ancestors: []gwv1.PolicyAncestorStatus{
					// Existing stale status for gw-1 that should be cleared
					{
						AncestorRef: gwv1.ParentReference{
							Group:     new(gwv1.Group("gateway.networking.k8s.io")),
							Kind:      new(gwv1.Kind("Gateway")),
							Namespace: new(gwv1.Namespace("default")),
							Name:      gwv1.ObjectName("gw-1"),
						},
						ControllerName: "example-controller",
						Conditions: []metav1.Condition{
							{
								ObservedGeneration: 1,
								Type:               string(shared.PolicyConditionAccepted),
								Status:             metav1.ConditionTrue,
								Reason:             string(shared.PolicyReasonValid),
							},
						},
					},
				},
			},
			wantStatus: &gwv1.PolicyStatus{
				Ancestors: []gwv1.PolicyAncestorStatus{},
			},
		},
		{
			// Foreign ancestors present in currentStatus are excluded from the desired
			// status: statussync.MergePolicyAncestorStatuses re-adds them at write time from
			// its own authoritative read. currentStatus is still consulted here for
			// LastTransitionTime and observedGeneration continuity on the ancestors we own.
			name: "exclude ancestor status belonging to external controllers",
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
					Group:     new(gwv1.Group("gateway.networking.k8s.io")),
					Kind:      new(gwv1.Kind("Gateway")),
					Namespace: new(gwv1.Namespace("default")),
					Name:      gwv1.ObjectName("gw-1"),
				}).SetCondition(reporter.PolicyCondition{
					Type:   string(shared.PolicyConditionAccepted),
					Status: metav1.ConditionTrue,
					Reason: string(shared.PolicyReasonValid),
				})
				// during gw-2 translation, add PolicyReasonInvalid
				policyReport.AncestorRef(gwv1.ParentReference{
					Group:     new(gwv1.Group("gateway.networking.k8s.io")),
					Kind:      new(gwv1.Kind("Gateway")),
					Namespace: new(gwv1.Namespace("default")),
					Name:      gwv1.ObjectName("gw-2"),
				}).SetCondition(reporter.PolicyCondition{
					Type:   string(shared.PolicyConditionAccepted),
					Status: metav1.ConditionFalse,
					Reason: string(shared.PolicyReasonInvalid),
				})
			},
			key: reporter.PolicyKey{
				Group:     "example.com",
				Kind:      "Policy",
				Namespace: "default",
				Name:      "example",
			},
			controller: "example-controller",
			currentStatus: gwv1.PolicyStatus{
				Ancestors: []gwv1.PolicyAncestorStatus{
					{
						AncestorRef: gwv1.ParentReference{
							Group:     new(gwv1.Group("gateway.networking.k8s.io")),
							Kind:      new(gwv1.Kind("Gateway")),
							Namespace: new(gwv1.Namespace("default")),
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
							Group:     new(gwv1.Group("gateway.networking.k8s.io")),
							Kind:      new(gwv1.Kind("Gateway")),
							Namespace: new(gwv1.Namespace("default")),
							Name:      gwv1.ObjectName("gw-1"),
						},
						ControllerName: "example-controller",
						Conditions: []metav1.Condition{
							{
								ObservedGeneration: 1,
								Type:               string(shared.PolicyConditionAccepted),
								Status:             metav1.ConditionFalse,
								Reason:             string(shared.PolicyReasonInvalid),
							},
						},
					},
					{
						AncestorRef: gwv1.ParentReference{
							Group:     new(gwv1.Group("gateway.networking.k8s.io")),
							Kind:      new(gwv1.Kind("Gateway")),
							Namespace: new(gwv1.Namespace("default")),
							Name:      gwv1.ObjectName("gw-2"),
						},
						ControllerName: "example-controller",
						Conditions: []metav1.Condition{
							{
								ObservedGeneration: 1,
								Type:               string(shared.PolicyConditionAccepted),
								Status:             metav1.ConditionFalse,
								Reason:             string(shared.PolicyReasonPending),
							},
						},
					},
				},
			},
			wantStatus: &gwv1.PolicyStatus{
				Ancestors: []gwv1.PolicyAncestorStatus{
					{
						AncestorRef: gwv1.ParentReference{
							Group:     new(gwv1.Group("gateway.networking.k8s.io")),
							Kind:      new(gwv1.Kind("Gateway")),
							Namespace: new(gwv1.Namespace("default")),
							Name:      gwv1.ObjectName("gw-1"),
						},
						ControllerName: "example-controller",
						Conditions: []metav1.Condition{
							{
								ObservedGeneration: 2,
								Type:               string(shared.PolicyConditionAccepted),
								Status:             metav1.ConditionTrue,
								Reason:             string(shared.PolicyReasonValid),
							},
							{
								ObservedGeneration: 2,
								Type:               string(shared.PolicyConditionAttached),
								Status:             metav1.ConditionFalse,
								Reason:             string(shared.PolicyReasonPending),
							},
						},
					},
					{
						AncestorRef: gwv1.ParentReference{
							Group:     new(gwv1.Group("gateway.networking.k8s.io")),
							Kind:      new(gwv1.Kind("Gateway")),
							Namespace: new(gwv1.Namespace("default")),
							Name:      gwv1.ObjectName("gw-2"),
						},
						ControllerName: "example-controller",
						Conditions: []metav1.Condition{
							{
								ObservedGeneration: 2,
								Type:               string(shared.PolicyConditionAccepted),
								Status:             metav1.ConditionFalse,
								Reason:             string(shared.PolicyReasonInvalid),
							},
							{
								ObservedGeneration: 2,
								Type:               string(shared.PolicyConditionAttached),
								Status:             metav1.ConditionFalse,
								Reason:             string(shared.PolicyReasonPending),
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

			gotStatus := rm.BuildPolicyStatus(tc.key, tc.controller, tc.currentStatus)
			diff := cmp.Diff(tc.wantStatus, gotStatus, cmpopts.IgnoreFields(metav1.Condition{}, "LastTransitionTime"))
			a.Empty(diff)
		})
	}
}

// The Gateway API cap lives in statussync.MergePolicyAncestorStatuses, which is the only
// layer with an authoritative read of the live ancestors it has to cap alongside ours.
// The builder publishes every ancestor it translated, uncapped, so the merge decides which
// entries survive with the whole list in hand.
func TestBuildPolicyStatusPublishesEveryTranslatedAncestorUncapped(t *testing.T) {
	rm := NewReportMap()
	statusReporter := NewReporter(&rm)
	key := reporter.PolicyKey{
		Group:     "example.com",
		Kind:      "Policy",
		Namespace: "default",
		Name:      "example",
	}

	const ancestors = MaxPolicyStatusAncestors + 1
	policyReporter := statusReporter.Policy(key, 1)
	for i := range ancestors {
		policyReporter.AncestorRef(gwv1.ParentReference{
			Group:     new(gwv1.Group("gateway.networking.k8s.io")),
			Kind:      new(gwv1.Kind("Gateway")),
			Namespace: new(gwv1.Namespace("default")),
			Name:      gwv1.ObjectName(fmt.Sprintf("gw-%02d", i)),
		}).SetCondition(reporter.PolicyCondition{
			Type:   string(shared.PolicyConditionAccepted),
			Status: metav1.ConditionTrue,
			Reason: string(shared.PolicyReasonValid),
		})
	}

	gotStatus := rm.BuildPolicyStatus(key, "example-controller", gwv1.PolicyStatus{})
	require.NotNil(t, gotStatus)
	require.Len(t, gotStatus.Ancestors, ancestors)
}

// The builder publishes only the ancestors we own. Foreign ancestors are re-derived by the
// merge from its own read of the live object, so preserving them here would only produce
// entries the merge discards.
func TestBuildPolicyStatusExcludesForeignAncestors(t *testing.T) {
	rm := NewReportMap()
	statusReporter := NewReporter(&rm)
	key := reporter.PolicyKey{
		Group:     "example.com",
		Kind:      "Policy",
		Namespace: "default",
		Name:      "example",
	}
	statusReporter.Policy(key, 1).AncestorRef(gwv1.ParentReference{Name: "our-gw"}).
		SetCondition(reporter.PolicyCondition{
			Type:   string(shared.PolicyConditionAccepted),
			Status: metav1.ConditionTrue,
			Reason: string(shared.PolicyReasonValid),
		})

	currentStatus := gwv1.PolicyStatus{Ancestors: []gwv1.PolicyAncestorStatus{{
		AncestorRef:    gwv1.ParentReference{Name: "their-gw"},
		ControllerName: "other.example/controller",
	}}}

	gotStatus := rm.BuildPolicyStatus(key, "example-controller", currentStatus)

	require.NotNil(t, gotStatus)
	require.Len(t, gotStatus.Ancestors, 1)
	require.Equal(t, gwv1.GatewayController("example-controller"), gotStatus.Ancestors[0].ControllerName)
}

func TestAddAttachmentCondition(t *testing.T) {
	tests := []struct {
		name            string
		attachmentState reporter.PolicyAttachmentState
		wantStatus      metav1.ConditionStatus
		wantReason      string
		wantMessage     string
		expectModified  bool
	}{
		{
			name:            "pending does not modify conditions",
			attachmentState: reporter.PolicyAttachmentStatePending,
			expectModified:  false,
		},
		{
			name:            "attached only reports Attached",
			attachmentState: reporter.PolicyAttachmentStateAttached,
			wantStatus:      metav1.ConditionTrue,
			wantReason:      string(shared.PolicyReasonAttached),
			wantMessage:     reporter.PolicyAttachedMsg,
			expectModified:  true,
		},
		{
			name:            "merged only reports Merged",
			attachmentState: reporter.PolicyAttachmentStateMerged,
			wantStatus:      metav1.ConditionTrue,
			wantReason:      string(shared.PolicyReasonMerged),
			wantMessage:     reporter.PolicyMergedMsg,
			expectModified:  true,
		},
		{
			name:            "overridden only reports Overridden",
			attachmentState: reporter.PolicyAttachmentStateOverridden,
			wantStatus:      metav1.ConditionFalse,
			wantReason:      string(shared.PolicyReasonOverridden),
			wantMessage:     reporter.PolicyOverriddenMsg,
			expectModified:  true,
		},
		{
			name:            "attached and overridden (partially applied) reports Merged",
			attachmentState: reporter.PolicyAttachmentStateAttached | reporter.PolicyAttachmentStateOverridden,
			wantStatus:      metav1.ConditionTrue,
			wantReason:      string(shared.PolicyReasonMerged),
			wantMessage:     reporter.PolicyMergedMsg,
			expectModified:  true,
		},
		{
			name:            "merged and overridden reports Merged",
			attachmentState: reporter.PolicyAttachmentStateMerged | reporter.PolicyAttachmentStateOverridden,
			wantStatus:      metav1.ConditionTrue,
			wantReason:      string(shared.PolicyReasonMerged),
			wantMessage:     reporter.PolicyMergedMsg,
			expectModified:  true,
		},
		{
			name:            "attached, merged, and overridden reports Merged",
			attachmentState: reporter.PolicyAttachmentStateAttached | reporter.PolicyAttachmentStateMerged | reporter.PolicyAttachmentStateOverridden,
			wantStatus:      metav1.ConditionTrue,
			wantReason:      string(shared.PolicyReasonMerged),
			wantMessage:     reporter.PolicyMergedMsg,
			expectModified:  true,
		},
		{
			name:            "attached and merged reports Merged",
			attachmentState: reporter.PolicyAttachmentStateAttached | reporter.PolicyAttachmentStateMerged,
			wantStatus:      metav1.ConditionTrue,
			wantReason:      string(shared.PolicyReasonMerged),
			wantMessage:     reporter.PolicyMergedMsg,
			expectModified:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			initialConditions := []metav1.Condition{
				{
					Type:   string(shared.PolicyConditionAccepted),
					Status: metav1.ConditionTrue,
					Reason: string(shared.PolicyReasonValid),
				},
			}
			gotConditions := addAttachmentCondition(initialConditions, tc.attachmentState)
			if !tc.expectModified {
				require.Equal(t, initialConditions, gotConditions)
				return
			}
			cond := meta.FindStatusCondition(gotConditions, string(shared.PolicyConditionAttached))
			require.NotNil(t, cond)
			require.Equal(t, tc.wantStatus, cond.Status)
			require.Equal(t, tc.wantReason, cond.Reason)
			require.Equal(t, tc.wantMessage, cond.Message)
		})
	}
}

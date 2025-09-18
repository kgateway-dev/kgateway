package gateway_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	translatortest "github.com/kgateway-dev/kgateway/v2/test/translator"
)

func TestStatuses(t *testing.T) {
	testFn := func(t *testing.T, inputFile string, wantPolicyErrors map[reporter.PolicyKey]*gwv1alpha2.PolicyStatus) {
		dir := fsutils.MustGetThisDir()
		translatortest.TestTranslation(
			t,
			t.Context(),
			[]string{
				filepath.Join(dir, "testutils/inputs/status", inputFile),
			},
			filepath.Join(dir, "testutils/outputs/status", inputFile),
			types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
			func(_ types.NamespacedName, reportsMap reports.ReportMap) {
				for policyKey, wantStatus := range wantPolicyErrors {
					var currentStatus gwv1alpha2.PolicyStatus
					a := assert.New(t)
					gotStatus := reportsMap.BuildPolicyStatus(context.Background(), policyKey, wellknown.DefaultGatewayControllerName, currentStatus)
					diff := cmp.Diff(wantStatus, gotStatus, cmpopts.IgnoreFields(metav1.Condition{}, "LastTransitionTime"))
					a.Empty(diff, "status mismatch for policy %v", policyKey)
				}
			},
		)
	}

	t.Run("Basic", func(t *testing.T) {
		testFn(t, "basic.yaml", map[reporter.PolicyKey]*gwv1alpha2.PolicyStatus{
			{Group: "gateway.kgateway.dev", Kind: "TrafficPolicy", Namespace: "default", Name: "extensionref-policy"}: {
				Ancestors: []gwv1alpha2.PolicyAncestorStatus{
					{
						AncestorRef: gwv1.ParentReference{
							Group:     ptr.To(gwv1.Group("gateway.networking.k8s.io")),
							Kind:      ptr.To(gwv1.Kind("Gateway")),
							Namespace: ptr.To(gwv1.Namespace("default")),
							Name:      gwv1.ObjectName("example-gateway"),
						},
						ControllerName: wellknown.DefaultGatewayControllerName,
						Conditions: []metav1.Condition{
							{
								ObservedGeneration: 1,
								Type:               string(v1alpha1.PolicyConditionAccepted),
								Status:             metav1.ConditionTrue,
								Reason:             string(v1alpha1.PolicyReasonValid),
								Message:            reporter.PolicyAcceptedMsg,
							},
							{
								ObservedGeneration: 1,
								Type:               string(v1alpha1.PolicyConditionAttached),
								Status:             metav1.ConditionTrue,
								Reason:             string(v1alpha1.PolicyReasonMerged),
								Message:            reporter.PolicyMergedMsg,
							},
						},
					},
				},
			},
			{Group: "gateway.kgateway.dev", Kind: "TrafficPolicy", Namespace: "default", Name: "policy-with-section-name"}: {
				Ancestors: []gwv1alpha2.PolicyAncestorStatus{
					{
						AncestorRef: gwv1.ParentReference{
							Group:     ptr.To(gwv1.Group("gateway.networking.k8s.io")),
							Kind:      ptr.To(gwv1.Kind("Gateway")),
							Namespace: ptr.To(gwv1.Namespace("default")),
							Name:      gwv1.ObjectName("example-gateway"),
						},
						ControllerName: wellknown.DefaultGatewayControllerName,
						Conditions: []metav1.Condition{
							{
								ObservedGeneration: 2,
								Type:               string(v1alpha1.PolicyConditionAccepted),
								Status:             metav1.ConditionTrue,
								Reason:             string(v1alpha1.PolicyReasonValid),
								Message:            reporter.PolicyAcceptedMsg,
							},
							{
								ObservedGeneration: 2,
								Type:               string(v1alpha1.PolicyConditionAttached),
								Status:             metav1.ConditionTrue,
								Reason:             string(v1alpha1.PolicyReasonMerged),
								Message:            reporter.PolicyMergedMsg,
							},
						},
					},
				},
			},
			{Group: "gateway.kgateway.dev", Kind: "TrafficPolicy", Namespace: "default", Name: "policy-without-section-name"}: {
				Ancestors: []gwv1alpha2.PolicyAncestorStatus{
					{
						AncestorRef: gwv1.ParentReference{
							Group:     ptr.To(gwv1.Group("gateway.networking.k8s.io")),
							Kind:      ptr.To(gwv1.Kind("Gateway")),
							Namespace: ptr.To(gwv1.Namespace("default")),
							Name:      gwv1.ObjectName("example-gateway"),
						},
						ControllerName: wellknown.DefaultGatewayControllerName,
						Conditions: []metav1.Condition{
							{
								ObservedGeneration: 3,
								Type:               string(v1alpha1.PolicyConditionAccepted),
								Status:             metav1.ConditionTrue,
								Reason:             string(v1alpha1.PolicyReasonValid),
								Message:            reporter.PolicyAcceptedMsg,
							},
							{
								ObservedGeneration: 3,
								Type:               string(v1alpha1.PolicyConditionAttached),
								Status:             metav1.ConditionTrue,
								Reason:             string(v1alpha1.PolicyReasonMerged),
								Message:            reporter.PolicyMergedMsg,
							},
						},
					},
				},
			},
			{Group: "gateway.kgateway.dev", Kind: "TrafficPolicy", Namespace: "default", Name: "fully-ignored"}: {
				Ancestors: []gwv1alpha2.PolicyAncestorStatus{
					{
						AncestorRef: gwv1.ParentReference{
							Group:     ptr.To(gwv1.Group("gateway.networking.k8s.io")),
							Kind:      ptr.To(gwv1.Kind("Gateway")),
							Namespace: ptr.To(gwv1.Namespace("default")),
							Name:      gwv1.ObjectName("example-gateway"),
						},
						ControllerName: wellknown.DefaultGatewayControllerName,
						Conditions: []metav1.Condition{
							{
								ObservedGeneration: 4,
								Type:               string(v1alpha1.PolicyConditionAccepted),
								Status:             metav1.ConditionTrue,
								Reason:             string(v1alpha1.PolicyReasonValid),
								Message:            reporter.PolicyAcceptedMsg,
							},
							{
								ObservedGeneration: 4,
								Type:               string(v1alpha1.PolicyConditionAttached),
								Status:             metav1.ConditionFalse,
								Reason:             string(v1alpha1.PolicyReasonOverridden),
								Message:            reporter.PolicyOverriddenMsg,
							},
						},
					},
				},
			},
			{Group: "gateway.kgateway.dev", Kind: "TrafficPolicy", Namespace: "default", Name: "policy-no-merge"}: {
				Ancestors: []gwv1alpha2.PolicyAncestorStatus{
					{
						AncestorRef: gwv1.ParentReference{
							Group:     ptr.To(gwv1.Group("gateway.networking.k8s.io")),
							Kind:      ptr.To(gwv1.Kind("Gateway")),
							Namespace: ptr.To(gwv1.Namespace("default")),
							Name:      gwv1.ObjectName("example-gateway"),
						},
						ControllerName: wellknown.DefaultGatewayControllerName,
						Conditions: []metav1.Condition{
							{
								ObservedGeneration: 1,
								Type:               string(v1alpha1.PolicyConditionAccepted),
								Status:             metav1.ConditionTrue,
								Reason:             string(v1alpha1.PolicyReasonValid),
								Message:            reporter.PolicyAcceptedMsg,
							},
							{
								ObservedGeneration: 1,
								Type:               string(v1alpha1.PolicyConditionAttached),
								Status:             metav1.ConditionTrue,
								Reason:             string(v1alpha1.PolicyReasonAttached),
								Message:            reporter.PolicyAttachedMsg,
							},
						},
					},
				},
			},
			{Group: "gateway.kgateway.dev", Kind: "HTTPListenerPolicy", Namespace: "default", Name: "policy-1"}: {
				Ancestors: []gwv1alpha2.PolicyAncestorStatus{
					{
						AncestorRef: gwv1.ParentReference{
							Group:     ptr.To(gwv1.Group("gateway.networking.k8s.io")),
							Kind:      ptr.To(gwv1.Kind("Gateway")),
							Namespace: ptr.To(gwv1.Namespace("default")),
							Name:      gwv1.ObjectName("example-gateway"),
						},
						ControllerName: wellknown.DefaultGatewayControllerName,
						Conditions: []metav1.Condition{
							{
								ObservedGeneration: 1,
								Type:               string(v1alpha1.PolicyConditionAccepted),
								Status:             metav1.ConditionTrue,
								Reason:             string(v1alpha1.PolicyReasonValid),
								Message:            reporter.PolicyAcceptedMsg,
							},
							{
								ObservedGeneration: 1,
								Type:               string(v1alpha1.PolicyConditionAttached),
								Status:             metav1.ConditionTrue,
								Reason:             string(v1alpha1.PolicyReasonMerged),
								Message:            reporter.PolicyMergedMsg,
							},
						},
					},
				},
			},
			{Group: "gateway.kgateway.dev", Kind: "HTTPListenerPolicy", Namespace: "default", Name: "policy-2"}: {
				Ancestors: []gwv1alpha2.PolicyAncestorStatus{
					{
						AncestorRef: gwv1.ParentReference{
							Group:     ptr.To(gwv1.Group("gateway.networking.k8s.io")),
							Kind:      ptr.To(gwv1.Kind("Gateway")),
							Namespace: ptr.To(gwv1.Namespace("default")),
							Name:      gwv1.ObjectName("example-gateway"),
						},
						ControllerName: wellknown.DefaultGatewayControllerName,
						Conditions: []metav1.Condition{
							{
								ObservedGeneration: 2,
								Type:               string(v1alpha1.PolicyConditionAccepted),
								Status:             metav1.ConditionTrue,
								Reason:             string(v1alpha1.PolicyReasonValid),
								Message:            reporter.PolicyAcceptedMsg,
							},
							{
								ObservedGeneration: 2,
								Type:               string(v1alpha1.PolicyConditionAttached),
								Status:             metav1.ConditionTrue,
								Reason:             string(v1alpha1.PolicyReasonMerged),
								Message:            reporter.PolicyMergedMsg,
							},
						},
					},
				},
			},
		})
	})
}

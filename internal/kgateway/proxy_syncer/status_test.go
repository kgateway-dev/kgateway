package proxy_syncer

import (
	"errors"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/assert"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	pluginsdkreporter "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

func TestPolicyStatus(t *testing.T) {
	connPolGK := schema.GroupKind{
		Group: "test",
		Kind:  "ConnectionPolicy",
	}
	tlsPolicyAtt := ir.PolicyAtt{
		GroupKind: wellknown.BackendTLSPolicyGVK.GroupKind(),
		PolicyRef: &ir.AttachedPolicyRef{
			Group:     wellknown.BackendTLSPolicyGVK.Group,
			Kind:      wellknown.BackendTLSPolicyKind,
			Name:      "tls-policy",
			Namespace: "default",
		},
		Errors: []error{
			errors.New("tls-policy error"),
		},
	}
	connPolicy1Att := ir.PolicyAtt{
		GroupKind: connPolGK,
		PolicyRef: &ir.AttachedPolicyRef{
			Group:     connPolGK.Group,
			Kind:      connPolGK.Kind,
			Name:      "conn-policy-1",
			Namespace: "default",
		},
		Errors: []error{},
	}
	connPolicy2Att := ir.PolicyAtt{
		GroupKind: connPolGK,
		PolicyRef: &ir.AttachedPolicyRef{
			Group:     connPolGK.Group,
			Kind:      connPolGK.Kind,
			Name:      "conn-policy-2",
			Namespace: "default",
		},
		Errors: []error{
			errors.New("conn-policy-2 error"),
		},
	}

	backend1 := ir.BackendObjectIR{
		ObjectSource: ir.ObjectSource{
			Group:     "",
			Kind:      "Service",
			Namespace: "default",
			Name:      "reviews",
		},
		AttachedPolicies: ir.AttachedPolicies{
			Policies: map[schema.GroupKind][]ir.PolicyAtt{
				wellknown.BackendTLSPolicyGVK.GroupKind(): {
					tlsPolicyAtt,
				},
				connPolGK: {
					connPolicy1Att,
				},
			},
		},
	}
	backend2 := ir.BackendObjectIR{
		ObjectSource: ir.ObjectSource{
			Group:     "",
			Kind:      "Service",
			Namespace: "default",
			Name:      "ratings",
		},
		AttachedPolicies: ir.AttachedPolicies{
			Policies: map[schema.GroupKind][]ir.PolicyAtt{
				wellknown.BackendTLSPolicyGVK.GroupKind(): {
					tlsPolicyAtt,
				},
				connPolGK: {
					connPolicy2Att,
				},
			},
		},
	}
	backends := []ir.BackendObjectIR{backend1, backend2}

	a := assert.New(t)
	rm := generatePolicyReport(backends)

	// assert 3 unique policies: conn-policy-1, conn-policy-2, tls-policy
	a.Len(rm.Policies, 3)

	// assert conn-policy-1 report
	connpolicy1report := rm.Policies[reports.PolicyKey{
		Group:     connPolicy1Att.PolicyRef.Group,
		Kind:      connPolicy1Att.PolicyRef.Kind,
		Namespace: connPolicy1Att.PolicyRef.Namespace,
		Name:      connPolicy1Att.PolicyRef.Name,
	}]
	a.Len(connpolicy1report.Ancestors, 1)
	ancestor1ConnPolicy1Report := connpolicy1report.Ancestors[reports.ParentRefKey{
		Group:          backend1.Group,
		Kind:           backend1.Kind,
		NamespacedName: types.NamespacedName{Namespace: backend1.Namespace, Name: backend1.Name},
	}]
	a.NotNil(ancestor1ConnPolicy1Report)
	diff := cmp.Diff(
		ancestor1ConnPolicy1Report.Conditions,
		[]metav1.Condition{
			{
				Type:    string(gwv1alpha2.PolicyConditionAccepted),
				Status:  metav1.ConditionTrue,
				Reason:  string(gwv1alpha2.PolicyReasonAccepted),
				Message: pluginsdkreporter.PolicyAcceptedAndAttachedMsg,
			},
		},
		cmpopts.IgnoreFields(metav1.Condition{}, "LastTransitionTime"),
	)
	a.Empty(diff)

	// assert conn-policy-2 report
	connpolicy2report := rm.Policies[pluginsdkreporter.PolicyKey{
		Group:     connPolicy2Att.PolicyRef.Group,
		Kind:      connPolicy2Att.PolicyRef.Kind,
		Namespace: connPolicy2Att.PolicyRef.Namespace,
		Name:      connPolicy2Att.PolicyRef.Name,
	}]
	a.Len(connpolicy2report.Ancestors, 1)
	ancestor1ConnPolicy2Report := connpolicy2report.Ancestors[reports.ParentRefKey{
		Group:          backend2.Group,
		Kind:           backend2.Kind,
		NamespacedName: types.NamespacedName{Namespace: backend2.Namespace, Name: backend2.Name},
	}]
	a.NotNil(ancestor1ConnPolicy2Report)
	diff = cmp.Diff(
		ancestor1ConnPolicy2Report.Conditions,
		[]metav1.Condition{
			{
				Type:    string(gwv1alpha2.PolicyConditionAccepted),
				Status:  metav1.ConditionFalse,
				Reason:  string(gwv1alpha2.PolicyReasonInvalid),
				Message: "conn-policy-2 error",
			},
		},
		cmpopts.IgnoreFields(metav1.Condition{}, "LastTransitionTime"),
	)
	a.Empty(diff)

	// assert conn-policy-2 report
	tlsPolicyreport := rm.Policies[reports.PolicyKey{
		Group:     tlsPolicyAtt.PolicyRef.Group,
		Kind:      tlsPolicyAtt.PolicyRef.Kind,
		Namespace: tlsPolicyAtt.PolicyRef.Namespace,
		Name:      tlsPolicyAtt.PolicyRef.Name,
	}]
	a.Len(tlsPolicyreport.Ancestors, 2)
	ancestor1TLSPolicyreport := tlsPolicyreport.Ancestors[reports.ParentRefKey{
		Group:          backend1.Group,
		Kind:           backend1.Kind,
		NamespacedName: types.NamespacedName{Namespace: backend1.Namespace, Name: backend1.Name},
	}]
	a.NotNil(ancestor1TLSPolicyreport)
	diff = cmp.Diff(
		ancestor1TLSPolicyreport.Conditions,
		[]metav1.Condition{
			{
				Type:    string(gwv1alpha2.PolicyConditionAccepted),
				Status:  metav1.ConditionFalse,
				Reason:  string(gwv1alpha2.PolicyReasonInvalid),
				Message: "tls-policy error",
			},
		},
		cmpopts.IgnoreFields(metav1.Condition{}, "LastTransitionTime"),
	)
	a.Empty(diff)
	ancestor2TLSPolicyreport := tlsPolicyreport.Ancestors[reports.ParentRefKey{
		Group:          backend2.Group,
		Kind:           backend2.Kind,
		NamespacedName: types.NamespacedName{Namespace: backend2.Namespace, Name: backend2.Name},
	}]
	a.NotNil(ancestor2TLSPolicyreport)
	diff = cmp.Diff(
		ancestor2TLSPolicyreport.Conditions,
		[]metav1.Condition{
			{
				Type:    string(gwv1alpha2.PolicyConditionAccepted),
				Status:  metav1.ConditionFalse,
				Reason:  string(gwv1alpha2.PolicyReasonInvalid),
				Message: "tls-policy error",
			},
		},
		cmpopts.IgnoreFields(metav1.Condition{}, "LastTransitionTime"),
	)
	a.Empty(diff)
}

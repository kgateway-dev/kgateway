package backendconfigpolicy

import (
	"errors"
	"testing"
	"time"

	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	. "github.com/onsi/gomega"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

// fakeBTPIR stands in for the BackendTLSPolicy plugin's IR, which this package cannot import.
type fakeBTPIR struct{ ct time.Time }

func (f *fakeBTPIR) CreationTime() time.Time { return f.ct }
func (f *fakeBTPIR) Equals(in any) bool {
	o, ok := in.(*fakeBTPIR)
	return ok && f.ct.Equal(o.ct)
}

var (
	bcpGK = wellknown.BackendConfigPolicyGVK.GroupKind()
	btpGK = wellknown.BackendTLSPolicyGVK.GroupKind()
	t2020 = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	t2021 = time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)
)

func bcpAtt(name string, ct time.Time, tls bool, opts ...func(*ir.PolicyAtt)) ir.PolicyAtt {
	pol := &BackendConfigPolicyIR{ct: ct}
	if tls {
		pol.tlsConfig = &envoytlsv3.UpstreamTlsContext{Sni: name}
	}
	att := ir.PolicyAtt{
		GroupKind:  bcpGK,
		Generation: 3,
		PolicyRef:  &ir.AttachedPolicyRef{Group: bcpGK.Group, Kind: bcpGK.Kind, Namespace: "ns", Name: name},
		PolicyIr:   pol,
	}
	for _, o := range opts {
		o(&att)
	}
	return att
}

func btpAtt(name string, ct time.Time, opts ...func(*ir.PolicyAtt)) ir.PolicyAtt {
	att := ir.PolicyAtt{
		GroupKind: btpGK,
		PolicyRef: &ir.AttachedPolicyRef{Group: btpGK.Group, Kind: btpGK.Kind, Namespace: "ns", Name: name},
		PolicyIr:  &fakeBTPIR{ct: ct},
	}
	for _, o := range opts {
		o(&att)
	}
	return att
}

func withErr(att *ir.PolicyAtt)              { att.Errors = []error{errors.New("boom")} }
func withWeight(w int32) func(*ir.PolicyAtt) { return func(a *ir.PolicyAtt) { a.PrecedenceWeight = w } }

func attached(bcps, btps []ir.PolicyAtt) ir.AttachedPolicies {
	return ir.AttachedPolicies{Policies: map[schema.GroupKind][]ir.PolicyAtt{bcpGK: bcps, btpGK: btps}}
}

func TestResolveTLS(t *testing.T) {
	type override struct {
		policy  string
		reason  string
		message string
	}
	tests := []struct {
		name       string
		bcps, btps []ir.PolicyAtt
		wantSource TLSSource
		wantWinner string
		// wantInvalid is set when the winning policy has errors.
		wantInvalid bool
		overridden  []override
		// notOverridden lists BackendConfigPolicies that must get no Overridden condition.
		notOverridden []string
	}{
		{
			name:       "nothing attached",
			wantSource: TLSSourceNone,
		},
		{
			name:          "single BackendConfigPolicy supplies tls",
			bcps:          []ir.PolicyAtt{bcpAtt("only", t2020, true)},
			wantSource:    TLSSourceBackendConfigPolicy,
			wantWinner:    "only",
			notOverridden: []string{"only"},
		},
		{
			name:          "BackendConfigPolicy without tls is never overridden",
			bcps:          []ir.PolicyAtt{bcpAtt("no-tls", t2020, false)},
			btps:          []ir.PolicyAtt{btpAtt("btp", t2021)},
			wantSource:    TLSSourceBackendTLSPolicy,
			wantWinner:    "btp",
			notOverridden: []string{"no-tls"},
		},
		{
			name:       "BackendTLSPolicy overrides BackendConfigPolicy tls",
			bcps:       []ir.PolicyAtt{bcpAtt("bcp", t2020, true)},
			btps:       []ir.PolicyAtt{btpAtt("btp", t2021)},
			wantSource: TLSSourceBackendTLSPolicy,
			wantWinner: "btp",
			overridden: []override{{"bcp", ReasonConflictedWithBackendTLSPolicy, "spec.tls overridden by BackendTLSPolicy ns/btp"}},
		},
		{
			// An invalid BackendTLSPolicy still owns TLS (the cluster is blackholed), so the
			// dropped spec.tls must be reported.
			name:        "only an invalid BackendTLSPolicy still overrides",
			bcps:        []ir.PolicyAtt{bcpAtt("bcp", t2020, true)},
			btps:        []ir.PolicyAtt{btpAtt("invalid-btp", t2021, withErr)},
			wantSource:  TLSSourceBackendTLSPolicy,
			wantWinner:  "invalid-btp",
			wantInvalid: true,
			overridden:  []override{{"bcp", ReasonConflictedWithBackendTLSPolicy, "spec.tls overridden by BackendTLSPolicy ns/invalid-btp, which is invalid; requests to this backend fail until it is fixed or removed"}},
		},
		{
			// The oldest BackendTLSPolicy wins even when invalid, matching the BackendTLSPolicy
			// plugin's MergePolicies, so the message names it rather than the newer valid one.
			name:        "older invalid BackendTLSPolicy is named over a newer valid one",
			bcps:        []ir.PolicyAtt{bcpAtt("bcp", t2020, true)},
			btps:        []ir.PolicyAtt{btpAtt("newer-valid", t2021), btpAtt("older-invalid", t2020, withErr)},
			wantSource:  TLSSourceBackendTLSPolicy,
			wantWinner:  "older-invalid",
			wantInvalid: true,
			overridden:  []override{{"bcp", ReasonConflictedWithBackendTLSPolicy, "spec.tls overridden by BackendTLSPolicy ns/older-invalid, which is invalid; requests to this backend fail until it is fixed or removed"}},
		},
		{
			name:          "older BackendConfigPolicy tls wins over newer",
			bcps:          []ir.PolicyAtt{bcpAtt("newer", t2021, true), bcpAtt("older", t2020, true)},
			wantSource:    TLSSourceBackendConfigPolicy,
			wantWinner:    "older",
			overridden:    []override{{"newer", ReasonConflictedWithBackendConfigPolicy, "spec.tls overridden by BackendConfigPolicy ns/older"}},
			notOverridden: []string{"older"},
		},
		{
			name:          "higher weight BackendConfigPolicy tls wins over older",
			bcps:          []ir.PolicyAtt{bcpAtt("older", t2020, true), bcpAtt("heavier", t2021, true, withWeight(10))},
			wantSource:    TLSSourceBackendConfigPolicy,
			wantWinner:    "heavier",
			overridden:    []override{{"older", ReasonConflictedWithBackendConfigPolicy, "spec.tls overridden by BackendConfigPolicy ns/heavier"}},
			notOverridden: []string{"heavier"},
		},
		{
			name:          "newer BackendConfigPolicy without tls is not overridden",
			bcps:          []ir.PolicyAtt{bcpAtt("older", t2020, true), bcpAtt("newer", t2021, false)},
			wantSource:    TLSSourceBackendConfigPolicy,
			wantWinner:    "older",
			notOverridden: []string{"older", "newer"},
		},
		{
			// An invalid BackendConfigPolicy makes the merge apply nothing, so no tls is in effect.
			name:          "invalid BackendConfigPolicy means no BackendConfigPolicy tls applies",
			bcps:          []ir.PolicyAtt{bcpAtt("older", t2020, true), bcpAtt("newer", t2021, true, withErr)},
			wantSource:    TLSSourceNone,
			notOverridden: []string{"older", "newer"},
		},
		{
			name:          "invalid BackendConfigPolicy alongside a single tls one",
			bcps:          []ir.PolicyAtt{bcpAtt("tls", t2020, true), bcpAtt("broken", t2021, false, withErr)},
			wantSource:    TLSSourceNone,
			notOverridden: []string{"tls"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			res := ResolveTLS(attached(tt.bcps, tt.btps))

			g.Expect(res.Source).To(Equal(tt.wantSource), "TLS source")
			if tt.wantWinner == "" {
				g.Expect(res.Winner).To(BeNil(), "no policy should supply TLS")
			} else {
				g.Expect(res.Winner).NotTo(BeNil(), "a policy should supply TLS")
				g.Expect(res.Winner.Name).To(Equal(tt.wantWinner), "winning policy")
				g.Expect(res.WinnerValid).To(Equal(!tt.wantInvalid), "winner validity")
			}

			byName := map[string]ir.PolicyAtt{}
			for _, p := range tt.bcps {
				byName[p.PolicyRef.Name] = p
			}
			for _, o := range tt.overridden {
				cond, ok := res.OverrideCondition(byName[o.policy])
				g.Expect(ok).To(BeTrue(), "%s should be reported as overridden", o.policy)
				g.Expect(cond.Type).To(Equal(ConditionOverridden))
				g.Expect(cond.Status).To(Equal(metav1.ConditionTrue))
				g.Expect(cond.Reason).To(Equal(o.reason))
				g.Expect(cond.Message).To(Equal(o.message))
				g.Expect(cond.ObservedGeneration).To(Equal(int64(3)), "condition should carry the policy generation")
			}
			for _, name := range tt.notOverridden {
				_, ok := res.OverrideCondition(byName[name])
				g.Expect(ok).To(BeFalse(), "%s should not be reported as overridden", name)
			}
		})
	}
}

// TestBackendTLSPolicyOwnsTLSMatchesResolveTLS pins the data path (which drops spec.tls) to
// the status path (which reports it as Overridden), so the two cannot drift apart again.
func TestBackendTLSPolicyOwnsTLSMatchesResolveTLS(t *testing.T) {
	for _, btps := range [][]ir.PolicyAtt{
		nil,
		{btpAtt("valid", t2020)},
		{btpAtt("invalid", t2020, withErr)},
	} {
		backend := ir.BackendObjectIR{AttachedPolicies: attached([]ir.PolicyAtt{bcpAtt("bcp", t2020, true)}, btps)}
		res := ResolveTLS(backend.AttachedPolicies)
		_, overridden := res.OverrideCondition(bcpAtt("bcp", t2020, true))
		if got := backendTLSPolicyOwnsTLS(backend); got != overridden {
			t.Errorf("with %d BackendTLSPolicies: data path drops spec.tls=%v, status reports Overridden=%v", len(btps), got, overridden)
		}
	}
}

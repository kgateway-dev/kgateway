package backendconfigpolicy_test

import (
	"context"
	"errors"
	"testing"
	"time"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	. "github.com/onsi/gomega"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/extensions2/plugins/backendconfigpolicy"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/extensions2/plugins/backendtlspolicy"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/proxy_syncer"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
)

// fakeBTPIR stands in for the BackendTLSPolicy plugin's IR. The BackendConfigPolicy side
// only needs its creation time and whether it carries errors.
type fakeBTPIR struct{ ct time.Time }

func (f *fakeBTPIR) CreationTime() time.Time { return f.ct }
func (f *fakeBTPIR) Equals(in any) bool {
	o, ok := in.(*fakeBTPIR)
	return ok && f.ct.Equal(o.ct)
}

var (
	bcpGK = wellknown.BackendConfigPolicyGVK.GroupKind()
	btpGK = wellknown.BackendTLSPolicyGVK.GroupKind()
)

func policyAtt(gk schema.GroupKind, name string, pol ir.PolicyIR, errs ...error) ir.PolicyAtt {
	return ir.PolicyAtt{
		GroupKind:  gk,
		Generation: 1,
		PolicyRef:  &ir.AttachedPolicyRef{Group: gk.Group, Kind: gk.Kind, Namespace: "default", Name: name},
		PolicyIr:   pol,
		Errors:     errs,
	}
}

func bcpKey(name string) reporter.PolicyKey {
	return reporter.PolicyKey{Group: bcpGK.Group, Kind: bcpGK.Kind, Namespace: "default", Name: name}
}

// resolutionPlugins are the two plugins' backend resolution hooks, as they register them.
var resolutionPlugins = sdk.ContributesPolicies{
	bcpGK: backendconfigpolicy.PolicyPluginForTest,
	btpGK: {MergePolicies: backendtlspolicy.MergePolicies, Supersedes: backendtlspolicy.Supersedes},
}

// tlsBackend is the Service backend every step attaches policies to, resolved as the backend
// index resolves it on attachment.
func tlsBackend(bcps, btps []ir.PolicyAtt) ir.BackendObjectIR {
	be := ir.NewBackendObjectIR(ir.ObjectSource{Kind: "Service", Namespace: "default", Name: "tls-backend"}, 8443, "", "")
	be.AttachedPolicies = ir.AttachedPolicies{Policies: map[schema.GroupKind][]ir.PolicyAtt{}}
	if len(bcps) > 0 {
		be.AttachedPolicies.Policies[bcpGK] = bcps
	}
	if len(btps) > 0 {
		be.AttachedPolicies.Policies[btpGK] = btps
	}
	be.PolicyResolution = sdk.ResolveBackendPolicies(be.AttachedPolicies, resolutionPlugins)
	return be
}

// TestTLSOwnershipAppearsAndDisappears walks a backend through a TLS conflict appearing,
// changing and going away, and checks at each step that the xDS the BackendConfigPolicy
// plugin writes and the status written for each BackendConfigPolicy agree. Each step's status
// is the live status the next step builds on, as the status writer does, so a condition that
// is carried forward instead of cleared shows up here.
func TestTLSOwnershipAppearsAndDisappears(t *testing.T) {
	t2020 := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	t2021 := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)
	t2022 := time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC)

	// The older policy skips verification; the newer one pins a SNI.
	olderInsecure := policyAtt(bcpGK, "older-insecure", backendconfigpolicy.NewTLSPolicyIRForTest(t2020,
		&envoytlsv3.UpstreamTlsContext{Sni: "older-insecure"}))
	newerVerify := policyAtt(bcpGK, "newer-verify", backendconfigpolicy.NewTLSPolicyIRForTest(t2021,
		&envoytlsv3.UpstreamTlsContext{Sni: "newer-verify"}))
	validBTP := policyAtt(btpGK, "verify", &fakeBTPIR{ct: t2022})
	invalidBTP := policyAtt(btpGK, "verify", &fakeBTPIR{ct: t2022}, errors.New("missing CA"))

	type wantStatus struct {
		// overridden is the expected Overridden reason, "" for none.
		reason  string
		message string
	}
	steps := []struct {
		name string
		bcps []ir.PolicyAtt
		btps []ir.PolicyAtt
		// wantSNI is the SNI of the TLS socket the BackendConfigPolicy plugin writes, "" for none.
		wantSNI string
		want    map[string]wantStatus
	}{
		{
			name:    "newer policy alone is in effect",
			bcps:    []ir.PolicyAtt{newerVerify},
			wantSNI: "newer-verify",
			want:    map[string]wantStatus{"newer-verify": {}},
		},
		{
			name:    "older policy takes tls and the newer one is overridden",
			bcps:    []ir.PolicyAtt{newerVerify, olderInsecure},
			wantSNI: "older-insecure",
			want: map[string]wantStatus{
				"older-insecure": {},
				"newer-verify": {
					reason:  "ConflictedWithBackendConfigPolicy",
					message: "spec.tls overridden by BackendConfigPolicy default/older-insecure",
				},
			},
		},
		{
			name: "a BackendTLSPolicy overrides both",
			bcps: []ir.PolicyAtt{newerVerify, olderInsecure},
			btps: []ir.PolicyAtt{validBTP},
			want: map[string]wantStatus{
				"older-insecure": {
					reason:  "ConflictedWithBackendTLSPolicy",
					message: "spec.tls overridden by BackendTLSPolicy default/verify",
				},
				"newer-verify": {
					reason:  "ConflictedWithBackendTLSPolicy",
					message: "spec.tls overridden by BackendTLSPolicy default/verify",
				},
			},
		},
		{
			name: "the BackendTLSPolicy turns invalid and still owns tls",
			bcps: []ir.PolicyAtt{newerVerify, olderInsecure},
			btps: []ir.PolicyAtt{invalidBTP},
			want: map[string]wantStatus{
				"older-insecure": {
					reason:  "ConflictedWithBackendTLSPolicy",
					message: "spec.tls overridden by BackendTLSPolicy default/verify, which is invalid; requests to this backend fail until it is fixed or removed",
				},
				"newer-verify": {
					reason:  "ConflictedWithBackendTLSPolicy",
					message: "spec.tls overridden by BackendTLSPolicy default/verify, which is invalid; requests to this backend fail until it is fixed or removed",
				},
			},
		},
		{
			name:    "the BackendTLSPolicy is deleted and the older policy takes tls back",
			bcps:    []ir.PolicyAtt{newerVerify, olderInsecure},
			wantSNI: "older-insecure",
			want: map[string]wantStatus{
				"older-insecure": {},
				"newer-verify": {
					reason:  "ConflictedWithBackendConfigPolicy",
					message: "spec.tls overridden by BackendConfigPolicy default/older-insecure",
				},
			},
		},
		{
			name:    "the older policy is deleted and the newer one is in effect again",
			bcps:    []ir.PolicyAtt{newerVerify},
			wantSNI: "newer-verify",
			want:    map[string]wantStatus{"newer-verify": {}},
		},
	}

	live := map[string]gwv1.PolicyStatus{}
	for _, step := range steps {
		t.Run(step.name, func(t *testing.T) {
			g := NewWithT(t)
			backend := tlsBackend(step.bcps, step.btps)

			// xDS: run the BackendConfigPolicy hook on what the resolution applies, as translation does.
			applied := backend.PolicyResolution.Applied(bcpGK)
			g.Expect(applied).To(HaveLen(1), "BackendConfigPolicies should resolve to one merged policy")
			cluster := &envoyclusterv3.Cluster{}
			backendconfigpolicy.ProcessBackendForTest(context.Background(), applied[0].PolicyIr, backend, cluster)
			if step.wantSNI == "" {
				g.Expect(cluster.GetTransportSocket()).To(BeNil(), "BackendConfigPolicy must not write tls")
			} else {
				tlsCtx := &envoytlsv3.UpstreamTlsContext{}
				g.Expect(cluster.GetTransportSocket().GetTypedConfig().UnmarshalTo(tlsCtx)).To(Succeed())
				g.Expect(tlsCtx.GetSni()).To(Equal(step.wantSNI), "tls applied from the wrong policy")
			}

			// Status, built on the previous step's live status.
			reports := proxy_syncer.GenerateBackendPolicyReport([]*ir.BackendObjectIR{&backend}, nil)
			for name, want := range step.want {
				status := reports.BuildPolicyStatus(bcpKey(name), wellknown.DefaultGatewayControllerName, live[name])
				g.Expect(status).NotTo(BeNil(), "%s should have a status", name)
				g.Expect(status.Ancestors).To(HaveLen(1), "%s should report one ancestor", name)
				conds := status.Ancestors[0].Conditions

				g.Expect(meta.IsStatusConditionTrue(conds, "Accepted")).To(BeTrue(), "%s should stay Accepted", name)
				overridden := meta.FindStatusCondition(conds, reporter.PolicyConditionOverridden)
				if want.reason == "" {
					g.Expect(overridden).To(BeNil(), "%s should not report Overridden", name)
				} else {
					g.Expect(overridden).NotTo(BeNil(), "%s should report Overridden", name)
					g.Expect(overridden.Status).To(Equal(metav1.ConditionTrue))
					g.Expect(overridden.Reason).To(Equal(want.reason))
					g.Expect(overridden.Message).To(Equal(want.message))
				}
				live[name] = *status
			}
		})
	}
}

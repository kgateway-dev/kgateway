package istio

import (
	"context"
	"testing"

	"istio.io/istio/pkg/kube/krt"

	. "github.com/onsi/gomega"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

func TestGlobalPolicies(t *testing.T) {
	tests := []struct {
		name           string
		enableAutoMtls bool
		want           ir.PolicyIR
	}{
		{
			name:           "auto-mTLS disabled attaches no global policy",
			enableAutoMtls: false,
			want:           nil,
		},
		{
			name:           "auto-mTLS enabled attaches the settings",
			enableAutoMtls: true,
			want:           IstioSettings{EnableAutoMtls: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			settings := apisettings.Settings{EnableIstioAutoMtls: tt.enableAutoMtls}
			plugin := NewPlugin(context.Background(), &collections.CommonCollections{Settings: settings})

			globalPolicy := plugin.ContributesPolicies[VirtualIstioGK].GlobalPolicies(nil)
			if tt.want == nil {
				g.Expect(globalPolicy).To(BeNil(), "auto-mTLS disabled should return no global policy")
			} else {
				g.Expect(globalPolicy).To(Equal(tt.want))
			}

			// A listener lookup should carry the global policy only when auto-mTLS is enabled.
			index := krtcollections.NewPolicyIndex(krtutil.KrtOptions{}, plugin.ContributesPolicies, settings)
			listener := ir.ObjectSource{
				Group:     wellknown.GatewayGVK.Group,
				Kind:      wellknown.GatewayKind,
				Namespace: "default",
				Name:      "gw",
			}
			attached := krtcollections.ToAttachedPolicies(index.GetTargetingPolicies(krt.TestingDummyContext{}, listener, "http", nil))
			if tt.want == nil {
				g.Expect(attached.Policies).To(BeEmpty(), "no policies should attach with auto-mTLS disabled")
			} else {
				g.Expect(attached.Policies).To(HaveKey(VirtualIstioGK))
				g.Expect(attached.Policies[VirtualIstioGK]).To(HaveLen(1))
				g.Expect(attached.Policies[VirtualIstioGK][0].PolicyIr).To(Equal(tt.want))
			}
		})
	}
}

package trafficpolicy

import (
	"testing"

	extensiondynamicmodulev3 "github.com/envoyproxy/go-control-plane/envoy/extensions/dynamic_modules/v3"
	dynamicmodulesv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/dynamic_modules/v3"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
)

func TestHttpACLIREquals(t *testing.T) {
	makeFilterCfg := func(json string) *dynamicmodulesv3.DynamicModuleFilterPerRoute {
		filterCfg := utils.MustMessageToAny(&wrapperspb.StringValue{Value: json})
		return &dynamicmodulesv3.DynamicModuleFilterPerRoute{
			DynamicModuleConfig: &extensiondynamicmodulev3.DynamicModuleConfig{
				Name: httpACLModuleName,
			},
			PerRouteConfigName: httpACLFilterName,
			FilterConfig:       filterCfg,
		}
	}

	allowAllCfg := makeFilterCfg(`{"defaultAction":"allow"}`)
	denyAllCfg := makeFilterCfg(`{"defaultAction":"deny"}`)
	withRulesCfg := makeFilterCfg(`{"defaultAction":"allow","rules":[{"cidrs":["10.0.0.0/8"],"action":"deny"}]}`)

	tests := []struct {
		name     string
		acl1     *httpACLIR
		acl2     *httpACLIR
		expected bool
	}{
		{
			name:     "both nil are equal",
			acl1:     nil,
			acl2:     nil,
			expected: true,
		},
		{
			name:     "nil vs non-nil are not equal",
			acl1:     nil,
			acl2:     &httpACLIR{config: allowAllCfg},
			expected: false,
		},
		{
			name:     "non-nil vs nil are not equal",
			acl1:     &httpACLIR{config: allowAllCfg},
			acl2:     nil,
			expected: false,
		},
		{
			name:     "identical configs are equal",
			acl1:     &httpACLIR{config: makeFilterCfg(`{"defaultAction":"allow"}`)},
			acl2:     &httpACLIR{config: makeFilterCfg(`{"defaultAction":"allow"}`)},
			expected: true,
		},
		{
			name:     "nil config fields are equal",
			acl1:     &httpACLIR{config: nil},
			acl2:     &httpACLIR{config: nil},
			expected: true,
		},
		{
			name:     "nil vs non-nil config fields are not equal",
			acl1:     &httpACLIR{config: nil},
			acl2:     &httpACLIR{config: allowAllCfg},
			expected: false,
		},
		{
			name:     "allow vs deny default action are not equal",
			acl1:     &httpACLIR{config: allowAllCfg},
			acl2:     &httpACLIR{config: denyAllCfg},
			expected: false,
		},
		{
			name:     "config with rules vs config without rules are not equal",
			acl1:     &httpACLIR{config: allowAllCfg},
			acl2:     &httpACLIR{config: withRulesCfg},
			expected: false,
		},
		{
			name:     "wrong type returns false",
			acl1:     &httpACLIR{config: allowAllCfg},
			acl2:     nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.acl1.Equals(tt.acl2)
			assert.Equal(t, tt.expected, result)

			// Test symmetry: a.Equals(b) should equal b.Equals(a)
			reverseResult := tt.acl2.Equals(tt.acl1)
			assert.Equal(t, result, reverseResult, "Equals should be symmetric")
		})
	}

	// Test reflexivity: x.Equals(x) should always be true for non-nil values
	t.Run("reflexivity", func(t *testing.T) {
		acl := &httpACLIR{config: allowAllCfg}
		assert.True(t, acl.Equals(acl), "httpACLIR should equal itself")
	})

	// Test transitivity: if a.Equals(b) && b.Equals(c), then a.Equals(c)
	t.Run("transitivity", func(t *testing.T) {
		a := &httpACLIR{config: makeFilterCfg(`{"defaultAction":"allow"}`)}
		b := &httpACLIR{config: makeFilterCfg(`{"defaultAction":"allow"}`)}
		c := &httpACLIR{config: makeFilterCfg(`{"defaultAction":"allow"}`)}

		assert.True(t, a.Equals(b), "a should equal b")
		assert.True(t, b.Equals(c), "b should equal c")
		assert.True(t, a.Equals(c), "a should equal c (transitivity)")
	})

	// Test that Equals returns false when compared against a different PolicySubIR type
	t.Run("wrong PolicySubIR type", func(t *testing.T) {
		acl := &httpACLIR{config: allowAllCfg}
		other := &rustformationIR{config: nil}
		assert.False(t, acl.Equals(other), "httpACLIR should not equal a different PolicySubIR type")
	})
}

func TestValidateACLCIDRs(t *testing.T) {
	makeACL := func(cidrs ...string) *shared.ACLPolicy {
		entries := make([]shared.IPOrCIDR, len(cidrs))
		for i, c := range cidrs {
			entries[i] = shared.IPOrCIDR(c)
		}
		return &shared.ACLPolicy{
			DefaultAction: shared.ACLActionDeny,
			Rules: []shared.ACLRule{
				{CIDRs: entries, Action: shared.ACLActionAllow},
			},
		}
	}

	t.Run("valid CIDRs pass", func(t *testing.T) {
		valid := []string{
			"10.0.0.0/8",
			"172.16.0.0/12",
			"192.168.0.0/16",
			"0.0.0.0/0",
			"fd00::/8",
			"2001:db8::/32",
			"::/0",
		}
		for _, cidr := range valid {
			t.Run(cidr, func(t *testing.T) {
				assert.NoError(t, validateACLCIDRs(makeACL(cidr)))
			})
		}
	})

	t.Run("bare IPs pass", func(t *testing.T) {
		bare := []string{
			"192.168.1.100",
			"10.0.0.1",
			"2001:db8::1",
			"::1",
		}
		for _, ip := range bare {
			t.Run(ip, func(t *testing.T) {
				assert.NoError(t, validateACLCIDRs(makeACL(ip)))
			})
		}
	})

	// CIDRs that have host bits set and must be rejected.
	invalidCIDRs := []struct {
		cidr    string
		wantNet string // expected network address in "did you mean" suggestion
	}{
		{"172.18.0.0/12", "172.16.0.0/12"},
		{"fd01::/8", "fd00::/8"},
		{"172.29.0.0/12", "172.16.0.0/12"},
		{"fd04::/8", "fd00::/8"},
		{"fd05::/8", "fd00::/8"},
		{"172.17.0.0/12", "172.16.0.0/12"},
		{"172.22.0.0/12", "172.16.0.0/12"},
		{"fd08::/8", "fd00::/8"},
		{"fd09::/8", "fd00::/8"},
		{"172.26.0.0/12", "172.16.0.0/12"},
		{"fd0c::/8", "fd00::/8"},
	}

	t.Run("CIDRs with host bits set are rejected", func(t *testing.T) {
		for _, tc := range invalidCIDRs {
			t.Run(tc.cidr, func(t *testing.T) {
				err := validateACLCIDRs(makeACL(tc.cidr))
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.cidr)
				assert.Contains(t, err.Error(), tc.wantNet)
			})
		}
	})

	t.Run("all invalid CIDRs reported together", func(t *testing.T) {
		all := make([]string, len(invalidCIDRs))
		for i, tc := range invalidCIDRs {
			all[i] = tc.cidr
		}
		err := validateACLCIDRs(makeACL(all...))
		assert.Error(t, err)
		for _, tc := range invalidCIDRs {
			assert.Contains(t, err.Error(), tc.cidr)
		}
	})

	t.Run("nil ACL policy", func(t *testing.T) {
		assert.NoError(t, validateACLCIDRs(&shared.ACLPolicy{}))
	})
}

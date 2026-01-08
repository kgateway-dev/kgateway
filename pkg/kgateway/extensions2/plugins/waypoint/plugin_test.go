package waypoint

import (
	"testing"

	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	"github.com/stretchr/testify/assert"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
)

func TestSortAddressesByDnsLookupFamily(t *testing.T) {
	tests := []struct {
		name      string
		addresses []string
		settings  *apisettings.Settings
		want      []string
	}{
		{
			name:      "nil settings defaults to V4_PREFERRED",
			addresses: []string{"10.0.0.1", "2001:db8::1"},
			settings:  nil,
			want:      []string{"10.0.0.1", "2001:db8::1"},
		},
		{
			name:      "ALL mode returns all addresses unchanged",
			addresses: []string{"2001:db8::1", "10.0.0.1", "10.0.0.2"},
			settings: &apisettings.Settings{
				DnsLookupFamily: apisettings.DnsLookupFamilyAll,
			},
			want: []string{"2001:db8::1", "10.0.0.1", "10.0.0.2"},
		},
		{
			name:      "V4_ONLY returns only IPv4 addresses",
			addresses: []string{"10.0.0.1", "2001:db8::1", "10.0.0.2", "2001:db8::2"},
			settings: &apisettings.Settings{
				DnsLookupFamily: apisettings.DnsLookupFamilyV4Only,
			},
			want: []string{"10.0.0.1", "10.0.0.2"},
		},
		{
			name:      "V4_ONLY with no IPv4 returns empty",
			addresses: []string{"2001:db8::1", "2001:db8::2"},
			settings: &apisettings.Settings{
				DnsLookupFamily: apisettings.DnsLookupFamilyV4Only,
			},
			want: []string{},
		},
		{
			name:      "V6_ONLY returns only IPv6 addresses",
			addresses: []string{"10.0.0.1", "2001:db8::1", "10.0.0.2", "2001:db8::2"},
			settings: &apisettings.Settings{
				DnsLookupFamily: apisettings.DnsLookupFamilyV6Only,
			},
			want: []string{"2001:db8::1", "2001:db8::2"},
		},
		{
			name:      "V6_ONLY with no IPv6 returns empty",
			addresses: []string{"10.0.0.1", "10.0.0.2"},
			settings: &apisettings.Settings{
				DnsLookupFamily: apisettings.DnsLookupFamilyV6Only,
			},
			want: []string{},
		},
		{
			name:      "V4_PREFERRED returns IPv4 first, then IPv6",
			addresses: []string{"2001:db8::1", "10.0.0.1", "2001:db8::2", "10.0.0.2"},
			settings: &apisettings.Settings{
				DnsLookupFamily: apisettings.DnsLookupFamilyV4Preferred,
			},
			want: []string{"10.0.0.1", "10.0.0.2", "2001:db8::1", "2001:db8::2"},
		},
		{
			name:      "V4_PREFERRED with no IPv4 returns IPv6 only",
			addresses: []string{"2001:db8::1", "2001:db8::2"},
			settings: &apisettings.Settings{
				DnsLookupFamily: apisettings.DnsLookupFamilyV4Preferred,
			},
			want: []string{"2001:db8::1", "2001:db8::2"},
		},
		{
			name:      "AUTO returns IPv6 first, then IPv4",
			addresses: []string{"10.0.0.1", "2001:db8::1", "10.0.0.2", "2001:db8::2"},
			settings: &apisettings.Settings{
				DnsLookupFamily: apisettings.DnsLookupFamilyAuto,
			},
			want: []string{"2001:db8::1", "2001:db8::2", "10.0.0.1", "10.0.0.2"},
		},
		{
			name:      "AUTO with no IPv6 returns IPv4 only",
			addresses: []string{"10.0.0.1", "10.0.0.2"},
			settings: &apisettings.Settings{
				DnsLookupFamily: apisettings.DnsLookupFamilyAuto,
			},
			want: []string{"10.0.0.1", "10.0.0.2"},
		},
		{
			name:      "invalid addresses are skipped",
			addresses: []string{"10.0.0.1", "invalid-address", "2001:db8::1", "not-an-ip"},
			settings: &apisettings.Settings{
				DnsLookupFamily: apisettings.DnsLookupFamilyV4Preferred,
			},
			want: []string{"10.0.0.1", "2001:db8::1"},
		},
		{
			name:      "empty addresses returns empty",
			addresses: []string{},
			settings: &apisettings.Settings{
				DnsLookupFamily: apisettings.DnsLookupFamilyV4Preferred,
			},
			want: []string{},
		},
		{
			name:      "unknown DNS lookup family defaults to V4_PREFERRED",
			addresses: []string{"2001:db8::1", "10.0.0.1"},
			settings: &apisettings.Settings{
				DnsLookupFamily: apisettings.DnsLookupFamily("UNKNOWN"),
			},
			want: []string{"10.0.0.1", "2001:db8::1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sortAddressesByDnsLookupFamily(tt.addresses, tt.settings)
			// Normalize nil and empty slices for comparison
			if got == nil {
				got = []string{}
			}
			if tt.want == nil {
				tt.want = []string{}
			}
			assert.Equal(t, tt.want, got, "sortAddressesByDnsLookupFamily() = %v, want %v", got, tt.want)
		})
	}
}

func TestClaEndpoint(t *testing.T) {
	tests := []struct {
		name      string
		addresses []string
		port      uint32
		wantNil   bool
		validate  func(t *testing.T, result *envoyendpointv3.LocalityLbEndpoints)
	}{
		{
			name:      "empty addresses returns nil",
			addresses: []string{},
			port:      8080,
			wantNil:   true,
		},
		{
			name:      "nil addresses returns nil",
			addresses: nil,
			port:      8080,
			wantNil:   true,
		},
		{
			name:      "single address - primary only, no AdditionalAddresses",
			addresses: []string{"10.0.0.1"},
			port:      8080,
			wantNil:   false,
			validate: func(t *testing.T, result *envoyendpointv3.LocalityLbEndpoints) {
				assert.NotNil(t, result)
				assert.Len(t, result.LbEndpoints, 1)

				endpoint := result.LbEndpoints[0].GetEndpoint()
				assert.NotNil(t, endpoint)

				// Check primary address
				socketAddr := endpoint.Address.GetSocketAddress()
				assert.NotNil(t, socketAddr)
				assert.Equal(t, "10.0.0.1", socketAddr.Address)
				assert.Equal(t, uint32(8080), socketAddr.GetPortValue())

				// Should have no additional addresses
				assert.Nil(t, endpoint.AdditionalAddresses)
			},
		},
		{
			name:      "multiple addresses - primary + AdditionalAddresses",
			addresses: []string{"10.0.0.1", "10.0.0.2", "10.0.0.3"},
			port:      9090,
			wantNil:   false,
			validate: func(t *testing.T, result *envoyendpointv3.LocalityLbEndpoints) {
				assert.NotNil(t, result)
				assert.Len(t, result.LbEndpoints, 1)

				endpoint := result.LbEndpoints[0].GetEndpoint()
				assert.NotNil(t, endpoint)

				// Check primary address
				socketAddr := endpoint.Address.GetSocketAddress()
				assert.NotNil(t, socketAddr)
				assert.Equal(t, "10.0.0.1", socketAddr.Address)
				assert.Equal(t, uint32(9090), socketAddr.GetPortValue())

				// Check additional addresses
				assert.NotNil(t, endpoint.AdditionalAddresses)
				assert.Len(t, endpoint.AdditionalAddresses, 2)

				// First additional address
				addr1 := endpoint.AdditionalAddresses[0].Address.GetSocketAddress()
				assert.NotNil(t, addr1)
				assert.Equal(t, "10.0.0.2", addr1.Address)
				assert.Equal(t, uint32(9090), addr1.GetPortValue())

				// Second additional address
				addr2 := endpoint.AdditionalAddresses[1].Address.GetSocketAddress()
				assert.NotNil(t, addr2)
				assert.Equal(t, "10.0.0.3", addr2.Address)
				assert.Equal(t, uint32(9090), addr2.GetPortValue())
			},
		},
		{
			name:      "mixed IPv4 and IPv6 addresses",
			addresses: []string{"10.0.0.1", "2001:db8::1", "10.0.0.2"},
			port:      443,
			wantNil:   false,
			validate: func(t *testing.T, result *envoyendpointv3.LocalityLbEndpoints) {
				assert.NotNil(t, result)
				endpoint := result.LbEndpoints[0].GetEndpoint()

				// Primary should be first address
				assert.Equal(t, "10.0.0.1", endpoint.Address.GetSocketAddress().Address)
				assert.Equal(t, uint32(443), endpoint.Address.GetSocketAddress().GetPortValue())

				// Additional addresses should be in order
				assert.Len(t, endpoint.AdditionalAddresses, 2)
				assert.Equal(t, "2001:db8::1", endpoint.AdditionalAddresses[0].Address.GetSocketAddress().Address)
				assert.Equal(t, uint32(443), endpoint.AdditionalAddresses[0].Address.GetSocketAddress().GetPortValue())
				assert.Equal(t, "10.0.0.2", endpoint.AdditionalAddresses[1].Address.GetSocketAddress().Address)
				assert.Equal(t, uint32(443), endpoint.AdditionalAddresses[1].Address.GetSocketAddress().GetPortValue())
			},
		},
		{
			name:      "port zero is valid",
			addresses: []string{"10.0.0.1"},
			port:      0,
			wantNil:   false,
			validate: func(t *testing.T, result *envoyendpointv3.LocalityLbEndpoints) {
				assert.NotNil(t, result)
				endpoint := result.LbEndpoints[0].GetEndpoint()
				assert.Equal(t, uint32(0), endpoint.Address.GetSocketAddress().GetPortValue())
			},
		},
		{
			name:      "large port number",
			addresses: []string{"10.0.0.1", "10.0.0.2"},
			port:      65535,
			wantNil:   false,
			validate: func(t *testing.T, result *envoyendpointv3.LocalityLbEndpoints) {
				assert.NotNil(t, result)
				endpoint := result.LbEndpoints[0].GetEndpoint()
				assert.Equal(t, uint32(65535), endpoint.Address.GetSocketAddress().GetPortValue())
				assert.Equal(t, uint32(65535), endpoint.AdditionalAddresses[0].Address.GetSocketAddress().GetPortValue())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := claEndpoint(tt.addresses, tt.port)

			if tt.wantNil {
				assert.Nil(t, result)
			} else {
				assert.NotNil(t, result)
				if tt.validate != nil {
					tt.validate(t, result)
				}
			}
		})
	}
}

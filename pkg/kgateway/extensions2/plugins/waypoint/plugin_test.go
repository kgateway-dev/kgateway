package waypoint

import (
	"testing"

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

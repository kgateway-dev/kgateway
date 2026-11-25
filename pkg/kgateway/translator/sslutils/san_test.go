package sslutils

import (
	"testing"

	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoymatcher "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"github.com/stretchr/testify/assert"
)

func TestVerifySanListToTypedMatchSanList(t *testing.T) {
	tests := []struct {
		name     string
		sanList  []string
		expected []*envoytlsv3.SubjectAltNameMatcher
	}{
		{
			name:     "empty SAN list",
			sanList:  []string{},
			expected: []*envoytlsv3.SubjectAltNameMatcher{},
		},
		{
			name:    "single SAN",
			sanList: []string{"example.com"},
			expected: []*envoytlsv3.SubjectAltNameMatcher{
				{
					SanType: envoytlsv3.SubjectAltNameMatcher_DNS,
					Matcher: &envoymatcher.StringMatcher{
						MatchPattern: &envoymatcher.StringMatcher_Exact{Exact: "example.com"},
					},
				},
			},
		},
		{
			name:    "multiple SANs",
			sanList: []string{"example.com", "api.example.com", "www.example.com"},
			expected: []*envoytlsv3.SubjectAltNameMatcher{
				{
					SanType: envoytlsv3.SubjectAltNameMatcher_DNS,
					Matcher: &envoymatcher.StringMatcher{
						MatchPattern: &envoymatcher.StringMatcher_Exact{Exact: "example.com"},
					},
				},
				{
					SanType: envoytlsv3.SubjectAltNameMatcher_DNS,
					Matcher: &envoymatcher.StringMatcher{
						MatchPattern: &envoymatcher.StringMatcher_Exact{Exact: "api.example.com"},
					},
				},
				{
					SanType: envoytlsv3.SubjectAltNameMatcher_DNS,
					Matcher: &envoymatcher.StringMatcher{
						MatchPattern: &envoymatcher.StringMatcher_Exact{Exact: "www.example.com"},
					},
				},
			},
		},
		{
			name:    "SPIFFE ID is matched as a URI SAN",
			sanList: []string{"spiffe://example.org/ns/default/sa/backend"},
			expected: []*envoytlsv3.SubjectAltNameMatcher{
				{
					SanType: envoytlsv3.SubjectAltNameMatcher_URI,
					Matcher: &envoymatcher.StringMatcher{
						MatchPattern: &envoymatcher.StringMatcher_Exact{Exact: "spiffe://example.org/ns/default/sa/backend"},
					},
				},
			},
		},
		{
			name:    "IPv4 address is matched as an IP SAN and as a DNS SAN",
			sanList: []string{"10.0.0.1"},
			expected: []*envoytlsv3.SubjectAltNameMatcher{
				{
					SanType: envoytlsv3.SubjectAltNameMatcher_IP_ADDRESS,
					Matcher: &envoymatcher.StringMatcher{
						MatchPattern: &envoymatcher.StringMatcher_Exact{Exact: "10.0.0.1"},
					},
				},
				{
					SanType: envoytlsv3.SubjectAltNameMatcher_DNS,
					Matcher: &envoymatcher.StringMatcher{
						MatchPattern: &envoymatcher.StringMatcher_Exact{Exact: "10.0.0.1"},
					},
				},
			},
		},
		{
			name:    "IPv6 address is matched as an IP SAN and as a DNS SAN",
			sanList: []string{"fd00::1"},
			expected: []*envoytlsv3.SubjectAltNameMatcher{
				{
					SanType: envoytlsv3.SubjectAltNameMatcher_IP_ADDRESS,
					Matcher: &envoymatcher.StringMatcher{
						MatchPattern: &envoymatcher.StringMatcher_Exact{Exact: "fd00::1"},
					},
				},
				{
					SanType: envoytlsv3.SubjectAltNameMatcher_DNS,
					Matcher: &envoymatcher.StringMatcher{
						MatchPattern: &envoymatcher.StringMatcher_Exact{Exact: "fd00::1"},
					},
				},
			},
		},
		{
			name:    "IPv6 address in long form is matched in canonical form as an IP SAN",
			sanList: []string{"fd00:0:0:0:0:0:0:1"},
			expected: []*envoytlsv3.SubjectAltNameMatcher{
				{
					SanType: envoytlsv3.SubjectAltNameMatcher_IP_ADDRESS,
					Matcher: &envoymatcher.StringMatcher{
						MatchPattern: &envoymatcher.StringMatcher_Exact{Exact: "fd00::1"},
					},
				},
				{
					SanType: envoytlsv3.SubjectAltNameMatcher_DNS,
					Matcher: &envoymatcher.StringMatcher{
						MatchPattern: &envoymatcher.StringMatcher_Exact{Exact: "fd00:0:0:0:0:0:0:1"},
					},
				},
			},
		},
		{
			name:    "email address is matched as an email SAN",
			sanList: []string{"svc@example.org"},
			expected: []*envoytlsv3.SubjectAltNameMatcher{
				{
					SanType: envoytlsv3.SubjectAltNameMatcher_EMAIL,
					Matcher: &envoymatcher.StringMatcher{
						MatchPattern: &envoymatcher.StringMatcher_Exact{Exact: "svc@example.org"},
					},
				},
			},
		},
		{
			name:    "wildcard hostname stays a DNS SAN",
			sanList: []string{"*.example.com"},
			expected: []*envoytlsv3.SubjectAltNameMatcher{
				{
					SanType: envoytlsv3.SubjectAltNameMatcher_DNS,
					Matcher: &envoymatcher.StringMatcher{
						MatchPattern: &envoymatcher.StringMatcher_Exact{Exact: "*.example.com"},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := VerifySanListToTypedMatchSanList(tt.sanList)

			assert.Len(t, result, len(tt.expected))

			for i, expected := range tt.expected {
				assert.Equal(t, expected.SanType, result[i].SanType)
				assert.Equal(t, expected.Matcher.GetExact(), result[i].Matcher.GetExact())
			}
		})
	}
}

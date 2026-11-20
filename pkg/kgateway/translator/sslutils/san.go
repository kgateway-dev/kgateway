package sslutils

import (
	"net/netip"
	"strings"

	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoymatcher "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
)

// VerifySanListToTypedMatchSanList builds exact SAN matchers for a list of subject alt names,
// picking the SAN type to match each entry against from its value. It is used for both
// upstream (BackendConfigPolicy verifySubjectAltNames) and downstream (the
// kgateway.dev/verify-subject-alt-names listener TLS option) certificate verification.
func VerifySanListToTypedMatchSanList(sanList []string) []*envoytlsv3.SubjectAltNameMatcher {
	var matchSanList []*envoytlsv3.SubjectAltNameMatcher
	for _, san := range sanList {
		if ip, err := netip.ParseAddr(san); err == nil {
			// A certificate can carry an IP address as an IP SAN or as a DNS SAN. Match both, so
			// certificates that matched the DNS-only matcher before still match. The IP matcher
			// uses the canonical form of the address, which is the form Envoy compares IP SANs in.
			matchSanList = append(matchSanList,
				exactSanMatcher(envoytlsv3.SubjectAltNameMatcher_IP_ADDRESS, ip.String()),
				exactSanMatcher(envoytlsv3.SubjectAltNameMatcher_DNS, san),
			)
			continue
		}
		matchSanList = append(matchSanList, exactSanMatcher(sanTypeOf(san), san))
	}
	return matchSanList
}

func exactSanMatcher(sanType envoytlsv3.SubjectAltNameMatcher_SanType, san string) *envoytlsv3.SubjectAltNameMatcher {
	return &envoytlsv3.SubjectAltNameMatcher{
		SanType: sanType,
		Matcher: &envoymatcher.StringMatcher{
			MatchPattern: &envoymatcher.StringMatcher_Exact{Exact: san},
		},
	}
}

// sanTypeOf picks the SAN type to match a verifySubjectAltNames entry that is not an IP address
// against. Typed matchers only compare against a single SAN type, so a DNS-only matcher never
// matches a certificate that carries its identity as a URI (such as a SPIFFE ID) or an email
// address.
func sanTypeOf(san string) envoytlsv3.SubjectAltNameMatcher_SanType {
	switch {
	case strings.Contains(san, ":"):
		return envoytlsv3.SubjectAltNameMatcher_URI
	case strings.Contains(san, "@"):
		return envoytlsv3.SubjectAltNameMatcher_EMAIL
	default:
		return envoytlsv3.SubjectAltNameMatcher_DNS
	}
}

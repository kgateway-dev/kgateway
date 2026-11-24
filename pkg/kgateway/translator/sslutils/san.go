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
			// uses the form Envoy prints a certificate's IP SAN in before comparing it.
			matchSanList = append(matchSanList,
				exactSanMatcher(envoytlsv3.SubjectAltNameMatcher_IP_ADDRESS, envoyIPString(ip)),
				exactSanMatcher(envoytlsv3.SubjectAltNameMatcher_DNS, san),
			)
			continue
		}
		matchSanList = append(matchSanList, exactSanMatcher(sanTypeOf(san), san))
	}
	return matchSanList
}

// envoyIPString formats ip the way Envoy prints a certificate's IP SAN, with inet_ntop, so an
// exact IP_ADDRESS matcher compares equal. That is the canonical form netip.Addr.String returns,
// except for an IPv4-compatible IPv6 address (the first 96 bits zero, then an IPv4 address other
// than 0.0.0.0/16, such as ::192.0.2.1): inet_ntop prints its last 32 bits as dotted IPv4, while
// netip.Addr.String prints them in hex (::c000:201).
func envoyIPString(ip netip.Addr) string {
	if !ip.Is6() || ip.Zone() != "" {
		return ip.String()
	}
	b := ip.As16()
	if [12]byte(b[:12]) != [12]byte{} || (b[12] == 0 && b[13] == 0) {
		return ip.String()
	}
	return "::" + netip.AddrFrom4([4]byte(b[12:])).String()
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

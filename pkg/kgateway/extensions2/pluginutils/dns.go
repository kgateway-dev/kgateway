package pluginutils

import (
	"fmt"

	envoycommondnsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/clusters/common/dns/v3"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
)

// EnvoyDNSLookupFamilyFromSettings maps the controller setting to the Envoy DNS family.
func EnvoyDNSLookupFamilyFromSettings(family apisettings.DnsLookupFamily) envoycommondnsv3.DnsLookupFamily {
	switch family {
	case apisettings.DnsLookupFamilyV4Only:
		return envoycommondnsv3.DnsLookupFamily_V4_ONLY
	case apisettings.DnsLookupFamilyV6Only:
		return envoycommondnsv3.DnsLookupFamily_V6_ONLY
	case apisettings.DnsLookupFamilyAuto:
		return envoycommondnsv3.DnsLookupFamily_AUTO
	case apisettings.DnsLookupFamilyAll:
		return envoycommondnsv3.DnsLookupFamily_ALL
	default:
		return envoycommondnsv3.DnsLookupFamily_V4_PREFERRED
	}
}

// ToEnvoyDNSLookupFamily maps the API address family to the corresponding Envoy DNS family.
func ToEnvoyDNSLookupFamily(family kgateway.DnsLookupFamily) (envoycommondnsv3.DnsLookupFamily, error) {
	switch family {
	case kgateway.DnsLookupFamilyIPv4Preferred:
		return envoycommondnsv3.DnsLookupFamily_V4_PREFERRED, nil
	case kgateway.DnsLookupFamilyIPv4Only:
		return envoycommondnsv3.DnsLookupFamily_V4_ONLY, nil
	case kgateway.DnsLookupFamilyIPv6Only:
		return envoycommondnsv3.DnsLookupFamily_V6_ONLY, nil
	case kgateway.DnsLookupFamilyIPv6Preferred:
		return envoycommondnsv3.DnsLookupFamily_AUTO, nil
	case kgateway.DnsLookupFamilyAll:
		return envoycommondnsv3.DnsLookupFamily_ALL, nil
	default:
		return envoycommondnsv3.DnsLookupFamily_V4_PREFERRED, fmt.Errorf("unknown DNS lookup family %q", family)
	}
}

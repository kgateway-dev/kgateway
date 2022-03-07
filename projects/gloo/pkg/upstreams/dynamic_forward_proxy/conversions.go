package dynamic_forward_proxy

import (
	"fmt"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/dynamic_forward_proxy"
	"github.com/solo-io/go-utils/hashutils"
	"strings"

	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"

	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

const UpstreamNamePrefix = "dynamic-forward-proxy-svc:"

func IsDynamicForwardProxyUpstream(upstreamName string) bool {
	return strings.HasPrefix(upstreamName, UpstreamNamePrefix)
}

func DestinationToUpstreamRef(dfpDest *dynamic_forward_proxy.PerRouteConfig) *core.ResourceRef {
	return &core.ResourceRef{
		Namespace: defaults.GlooSystem,
		Name:      fakeUpstreamName(fmt.Sprintf("%v", hashutils.MustHash(dfpDest))),
	}
}

func fakeUpstreamName(dfpHash string) string {
	return UpstreamNamePrefix + dfpHash
}
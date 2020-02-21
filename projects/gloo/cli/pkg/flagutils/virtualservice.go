package flagutils

import (
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/ratelimit"
	"github.com/spf13/pflag"
)

func AddVirtualServiceFlags(set *pflag.FlagSet, vs *options.InputVirtualService) {
	addDisplayNameFlag(set, &vs.DisplayName)
	addDomainsFlag(set, &vs.Domains)
	addVirtualServiceFlagsRateLimit(set, &vs.RateLimit)
}

func addDisplayNameFlag(set *pflag.FlagSet, ptr *string) {
	set.StringVar(ptr, "display-name", "", "descriptive name of virtual service (defaults to resource name)")
}

func addDomainsFlag(set *pflag.FlagSet, ptr *[]string) {
	set.StringSliceVar(ptr, "domains", []string{}, "comma separated list of domains")
}

func addVirtualServiceFlagsRateLimit(set *pflag.FlagSet, rl *options.RateLimit) {
	// TODO: add support for authorization when it is supported for ratelimit
	//set.StringVar(&Options.RateLimits.AuthorizedHeader, "rate-limit-authorize-header", "", "header name used to authorize requests")
	set.BoolVar(&rl.Enable, "enable-rate-limiting", false, "enable rate limiting features for this virtual service")
	set.StringVar(&rl.TimeUnit, "rate-limit-time-unit", ratelimit.RateLimit_MINUTE.String(), "unit of time over which to apply the rate limit")
	set.Uint32Var(&rl.RequestsPerTimeUnit, "rate-limit-requests", 100, "requests per unit of time")
}

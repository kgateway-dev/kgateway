package glooctl

import (
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	"github.com/onsi/gomega/types"
)

type checkOutput struct {
	// include is the expected matcher when `glooctl check` includes a given type
	include types.GomegaMatcher
	// exclude is the expected matcher when `glooctl check` excludes a given type
	exclude types.GomegaMatcher
	// readOnly is the expected matcher when `glooctl check` is executed in --read-only mode
	readOnly types.GomegaMatcher
}

var (
	checkOutputByKey = map[string]checkOutput{
		"deployments": {
			include: ContainSubstring("Checking deployments... OK"),
			exclude: And(
				Not(ContainSubstring("Checking deployments...")),
				ContainSubstring("Checking proxies... Skipping proxies because deployments were excluded"),
			),
			readOnly: gstruct.Ignore(),
		},
		"pods": {
			include:  ContainSubstring("Checking pods... OK"),
			exclude:  Not(ContainSubstring("Checking pods...")),
			readOnly: gstruct.Ignore(),
		},
		"upstreams": {
			include:  ContainSubstring("Checking upstreams... OK"),
			exclude:  Not(ContainSubstring("Checking upstreams...")),
			readOnly: gstruct.Ignore(),
		},
		"upstreamgroup": {
			include:  ContainSubstring("Checking upstream groups... OK"),
			exclude:  Not(ContainSubstring("Checking upstream groups...")),
			readOnly: gstruct.Ignore(),
		},
		"rate-limit-configs": {
			include:  ContainSubstring("Checking rate limit configs... OK"),
			exclude:  Not(ContainSubstring("Checking rate limit configs...")),
			readOnly: gstruct.Ignore(),
		},
		"virtual-host-options": {
			include:  ContainSubstring("Checking VirtualHostOptions... OK"),
			exclude:  Not(ContainSubstring("Checking VirtualHostOptions...")),
			readOnly: gstruct.Ignore(),
		},
		"route-options": {
			include:  ContainSubstring("Checking RouteOptions... OK"),
			exclude:  Not(ContainSubstring("Checking RouteOptions...")),
			readOnly: gstruct.Ignore(),
		},
		"secrets": {
			include:  ContainSubstring("Checking secrets... OK"),
			exclude:  Not(ContainSubstring("Checking secrets...")),
			readOnly: gstruct.Ignore(),
		},
		"virtual-services": {
			include:  ContainSubstring("Checking virtual services... OK"),
			exclude:  Not(ContainSubstring("Checking virtual services...")),
			readOnly: gstruct.Ignore(),
		},
		"route-tables": {
			// RouteTable CRs are not currently included in `glooctl check`
			// https://github.com/solo-io/gloo/issues/4244
			// https://github.com/solo-io/gloo/issues/2780
			include:  gstruct.Ignore(),
			exclude:  gstruct.Ignore(),
			readOnly: gstruct.Ignore(),
		},
		"gateways": {
			include:  ContainSubstring("Checking gateways... OK"),
			exclude:  Not(ContainSubstring("Checking gateways...")),
			readOnly: gstruct.Ignore(),
		},
		"proxies": {
			include:  ContainSubstring("Checking proxies... OK"),
			exclude:  Not(ContainSubstring("Checking proxies...")),
			readOnly: ContainSubstring("Warning: checking proxies with port forwarding is disabled"),
		},
		"xds-metrics": {
			include:  gstruct.Ignore(), // We have not had historical tests for this, it would be good to add
			exclude:  gstruct.Ignore(), // We have not had historical tests for this, it would be good to add
			readOnly: ContainSubstring("Warning: checking proxies with port forwarding is disabled"),
		},
	}
)

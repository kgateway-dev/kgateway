package glooctl

import (
	"errors"
	"fmt"
	"strings"

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/format"
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
	checkCommonGlooGatewayOutputByKey = map[string]checkOutput{
		"deployments": {
			include: ContainSubstring("Checking Deployments... OK"),
			exclude: And(
				Not(ContainSubstring("Checking Deployments...")),
				ContainSubstring("Checking Proxies... Skipping proxies because deployments were excluded"),
			),
			readOnly: gstruct.Ignore(),
		},
		"pods": {
			include:  ContainSubstring("Checking Pods... OK"),
			exclude:  Not(ContainSubstring("Checking Pods...")),
			readOnly: gstruct.Ignore(),
		},
		"upstreams": {
			include:  ContainSubstring("Checking Upstreams... OK"),
			exclude:  Not(ContainSubstring("Checking Upstreams...")),
			readOnly: gstruct.Ignore(),
		},
		"upstreamgroup": {
			include:  ContainSubstring("Checking UpstreamGroups... OK"),
			exclude:  Not(ContainSubstring("Checking UpstreamGroups...")),
			readOnly: gstruct.Ignore(),
		},
		"auth-configs": {
			include:  ContainSubstring("Checking AuthConfigs... OK"),
			exclude:  Not(ContainSubstring("Checking AuthConfigs...")),
			readOnly: gstruct.Ignore(),
		},
		"rate-limit-configs": {
			include:  ContainSubstring("Checking RateLimitConfigs... OK"),
			exclude:  Not(ContainSubstring("Checking RateLimitConfigs...")),
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
			include:  ContainSubstring("Checking Secrets... OK"),
			exclude:  Not(ContainSubstring("Checking Secrets...")),
			readOnly: gstruct.Ignore(),
		},
		"virtual-services": {
			include:  ContainSubstring("Checking VirtualServices... OK"),
			exclude:  Not(ContainSubstring("Checking VirtualServices...")),
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
			include:  ContainSubstring("Checking Gateways... OK"),
			exclude:  Not(ContainSubstring("Checking Gateways...")),
			readOnly: gstruct.Ignore(),
		},
		"proxies": {
			include:  ContainSubstring("Checking Proxies... OK"),
			exclude:  Not(ContainSubstring("Checking Proxies...")),
			readOnly: ContainSubstring("Warning: checking proxies with port forwarding is disabled"),
		},
		"xds-metrics": {
			include:  gstruct.Ignore(), // We have not had historical tests for this, it would be good to add
			exclude:  gstruct.Ignore(), // We have not had historical tests for this, it would be good to add
			readOnly: ContainSubstring("Warning: checking proxies with port forwarding is disabled"),
		},
	}

	checkK8sGatewayOutputByKey = map[string]checkOutput{
		"kube-gateway-classes": {
			include:  ContainSubstring("Checking Kubernetes GatewayClasses... OK"),
			exclude:  Not(ContainSubstring("Checking Kubernetes GatewayClasses...")),
			readOnly: gstruct.Ignore(),
		},
		"kube-gateways": {
			include:  ContainSubstring("Checking Kubernetes Gateways... OK"),
			exclude:  Not(ContainSubstring("Checking Kubernetes Gateways...")),
			readOnly: gstruct.Ignore(),
		},
		"kube-http-routes": {
			include:  ContainSubstring("Checking Kubernetes HTTPRoutes... OK"),
			exclude:  Not(ContainSubstring("Checking Kubernetes HTTPRoutes...")),
			readOnly: gstruct.Ignore(),
		},
	}
)

// Common expected substrings for both matchers (Edge and k8s Gateway)
var edgeExpectedChecks = []string{
	"Checking Deployments... OK",
	"Checking Pods... OK",
	"Checking Upstreams... OK",
	"Checking UpstreamGroups... OK",
	"Checking AuthConfigs... OK",
	"Checking RateLimitConfigs... OK",
	"Checking Secrets... OK",
	"Checking VirtualServices... OK",
	"Checking Gateways... OK",
	"Checking Proxies... OK",
}

// GlooctlEdgeHealthyCheck returns a GomegaMatcher that checks for all the expected Gloo Edge checks in the output.
func GlooctlEdgeHealthyCheck() types.GomegaMatcher {
	return &glooctlEdgeHealthyCheckMatcher{}
}

type glooctlEdgeHealthyCheckMatcher struct{}

func (matcher *glooctlEdgeHealthyCheckMatcher) Match(actual interface{}) (success bool, err error) {
	output, ok := actual.(string)
	if !ok {
		return false, errors.New(fmt.Sprintf("Invalid type. Expected string, got %v", actual))
	}

	for _, substring := range edgeExpectedChecks {
		if !strings.Contains(output, substring) {
			return false, nil
		}
	}
	return true, nil
}

func (matcher *glooctlEdgeHealthyCheckMatcher) FailureMessage(actual interface{}) (message string) {
	return format.Message(actual, "to contain expected glooctl checks")
}

func (matcher *glooctlEdgeHealthyCheckMatcher) NegatedFailureMessage(actual interface{}) (message string) {
	return format.Message(actual, "to contain expected glooctl checks")
}

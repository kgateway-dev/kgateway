package tests

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/glooctl"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/istio"
)

func GlooctlIstioInjectTests() TestRunner { return glooctlIstioInjectTestsToRun }

// NOTE: Order of tests is important here because the tests are dependent on each other (e.g. the inject test must run before the istio test)
var (
	glooctlIstioInjectTestsToRun = OrderedTests{
		{
			Name: "GlooctlIstioInject",
			Test: func(ctx context.Context, t *testing.T, testInstallation *e2e.TestInstallation) func(t *testing.T) {
				return func(t *testing.T) {
					suite.Run(t, glooctl.NewIstioInjectTestingSuite(ctx, testInstallation))
				}
			},
		},

		{
			Name: "IstioIntegration",
			Test: func(ctx context.Context, t *testing.T, testInstallation *e2e.TestInstallation) func(t *testing.T) {
				return func(t *testing.T) {
					suite.Run(t, istio.NewGlooTestingSuite(ctx, testInstallation))
				}
			},
		},
		{
			Name: "GlooctlIstioUninject",
			Test: func(ctx context.Context, t *testing.T, testInstallation *e2e.TestInstallation) func(t *testing.T) {
				return func(t *testing.T) {
					suite.Run(t, glooctl.NewIstioUninjectTestingSuite(ctx, testInstallation))
				}
			},
		},
	}
)

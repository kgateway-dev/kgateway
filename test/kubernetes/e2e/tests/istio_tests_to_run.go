package tests

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/deployer"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/headless_svc"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/istio"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/port_routing"
)

func IstioTests() TestRunner { return istioTestsToRun }

var (
	istioTestsToRun = UnorderedTests{
		"PortRouting": func(ctx context.Context, t *testing.T, testInstallation *e2e.TestInstallation) func(t *testing.T) {
			return func(t *testing.T) {
				suite.Run(t, port_routing.NewTestingSuite(ctx, testInstallation))
			}
		},

		"HeadlessSvc": func(ctx context.Context, t *testing.T, testInstallation *e2e.TestInstallation) func(t *testing.T) {
			return func(t *testing.T) {
				suite.Run(t, headless_svc.NewK8sGatewayHeadlessSvcSuite(ctx, testInstallation))
			}
		},

		"IstioIntegration": func(ctx context.Context, t *testing.T, testInstallation *e2e.TestInstallation) func(t *testing.T) {
			return func(t *testing.T) {
				suite.Run(t, istio.NewTestingSuite(ctx, testInstallation))
			}
		},

		"IstioGatewayParameters": func(ctx context.Context, t *testing.T, testInstallation *e2e.TestInstallation) func(t *testing.T) {
			return func(t *testing.T) {
				suite.Run(t, deployer.NewIstioIntegrationTestingSuite(ctx, testInstallation))

			}
		},
	}
)

package tests

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/headless_svc"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/istio"
)

func AutomtlsIstioEdgeApiTests() TestRunner { return automtlsIstioEdgeApiTestsToRun }

var (
	automtlsIstioEdgeApiTestsToRun = UnorderedTests{
		"HeadlessSvc": func(ctx context.Context, t *testing.T, testInstallation *e2e.TestInstallation) func(t *testing.T) {
			return func(t *testing.T) {
				suite.Run(t, headless_svc.NewEdgeGatewayHeadlessSvcSuite(ctx, testInstallation))
			}
		},
		"IstioIntegrationAutoMtls": func(ctx context.Context, t *testing.T, testInstallation *e2e.TestInstallation) func(t *testing.T) {
			return func(t *testing.T) {
				suite.Run(t, istio.NewGlooIstioAutoMtlsSuite(ctx, testInstallation))
			}
		},
	}
)

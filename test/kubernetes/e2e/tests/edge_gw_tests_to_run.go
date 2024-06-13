package tests

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/headless_svc"
)

func EdgeGwTests() TestRunner { return edgeGwTestsToRun }

var (
	edgeGwTestsToRun = UnorderedTests{
		"HeadlessSvc": func(ctx context.Context, t *testing.T, testInstallation *e2e.TestInstallation) func(t *testing.T) {
			return func(t *testing.T) {
				suite.Run(t, headless_svc.NewEdgeGatewayHeadlessSvcSuite(ctx, testInstallation))
			}
		},
	}
)

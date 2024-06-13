package tests

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/listener_options"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/port_routing"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/route_options"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/virtualhost_options"
)

func KubeGatewayNoValidationTests() TestRunner { return kubeGatewayNoValidationTestsToRun }

var (
	kubeGatewayNoValidationTestsToRun = UnorderedTests{
		"ListenerOptions": func(ctx context.Context, t *testing.T, testInstallation *e2e.TestInstallation) func(t *testing.T) {
			return func(t *testing.T) {
				suite.Run(t, listener_options.NewTestingSuite(ctx, testInstallation))
			}
		},

		"RouteOptions": func(ctx context.Context, t *testing.T, testInstallation *e2e.TestInstallation) func(t *testing.T) {
			return func(t *testing.T) {
				suite.Run(t, route_options.NewTestingSuite(ctx, testInstallation))
			}
		},

		"VirtualHostOptions": func(ctx context.Context, t *testing.T, testInstallation *e2e.TestInstallation) func(t *testing.T) {
			return func(t *testing.T) {
				suite.Run(t, virtualhost_options.NewTestingSuite(ctx, testInstallation))
			}
		},

		"PortRouting": func(ctx context.Context, t *testing.T, testInstallation *e2e.TestInstallation) func(t *testing.T) {
			return func(t *testing.T) {
				suite.Run(t, port_routing.NewTestingSuite(ctx, testInstallation))

			}
		},
	}
)

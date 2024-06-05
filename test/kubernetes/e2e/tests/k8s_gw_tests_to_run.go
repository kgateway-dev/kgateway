package tests

import (
	"context"
	"testing"

	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/deployer"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/glooctl"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/headless_svc"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/listener_options"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/port_routing"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/route_delegation"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/route_options"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/upstreams"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/virtualhost_options"
	"github.com/stretchr/testify/suite"
)

var (
	TestsToRun = map[string]func(ctx context.Context, t *testing.T, testInstallation *e2e.TestInstallation) func(t *testing.T){
		"Deployer": func(ctx context.Context, t *testing.T, testInstallation *e2e.TestInstallation) func(t *testing.T) {
			return func(t *testing.T) {
				suite.Run(t, deployer.NewTestingSuite(ctx, testInstallation))
			}
		},
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
		"Upstreams": func(ctx context.Context, t *testing.T, testInstallation *e2e.TestInstallation) func(t *testing.T) {
			return func(t *testing.T) {
				suite.Run(t, upstreams.NewTestingSuite(ctx, testInstallation))
			}
		},
		"HeadlessSvc": func(ctx context.Context, t *testing.T, testInstallation *e2e.TestInstallation) func(t *testing.T) {
			return func(t *testing.T) {
				suite.Run(t, headless_svc.NewK8sGatewayHeadlessSvcSuite(ctx, testInstallation))
			}
		},
		"PortRouting": func(ctx context.Context, t *testing.T, testInstallation *e2e.TestInstallation) func(t *testing.T) {
			return func(t *testing.T) {
				suite.Run(t, port_routing.NewTestingSuite(ctx, testInstallation))
			}
		},
		"RouteDelegation": func(ctx context.Context, t *testing.T, testInstallation *e2e.TestInstallation) func(t *testing.T) {
			return func(t *testing.T) {
				suite.Run(t, route_delegation.NewTestingSuite(ctx, testInstallation))
			}
		},
		"Glooctl": func(ctx context.Context, t *testing.T, testInstallation *e2e.TestInstallation) func(t *testing.T) {
			return func(t *testing.T) {
				t.Run("Check", func(t *testing.T) {
					suite.Run(t, glooctl.NewCheckSuite(ctx, testInstallation))
				})

				t.Run("Debug", func(t *testing.T) {
					suite.Run(t, glooctl.NewDebugSuite(ctx, testInstallation))
				})

				t.Run("GetProxy", func(t *testing.T) {
					suite.Run(t, glooctl.NewGetProxySuite(ctx, testInstallation))
				})
			}
		},
	}
)

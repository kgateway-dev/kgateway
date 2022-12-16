package e2e

import (
	"context"
	"fmt"

	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gatewaydefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/services"
	"github.com/solo-io/gloo/test/v1helpers"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
)

var (
	writeNamespace = defaults.GlooSystem
	envoyRole      = fmt.Sprintf("%v~%v", writeNamespace, gatewaydefaults.GatewayProxyName)
)

type TestContextFactory struct {
	EnvoyFactory *services.EnvoyFactory
}

func (f *TestContextFactory) NewTestContext() *TestContext {
	return &TestContext{
		envoyInstance: f.EnvoyFactory.MustEnvoyInstance(),
	}
}

type TestContext struct {
	ctx           context.Context
	cancel        context.CancelFunc
	envoyInstance *services.EnvoyInstance

	runOptions  *services.RunOptions
	testClients services.TestClients

	testUpstream *v1helpers.TestUpstream

	resourcesToCreate *gloosnapshot.ApiSnapshot
}

func (c *TestContext) SetRunOptions(options *services.RunOptions) {
	c.runOptions = options
}

func (c *TestContext) BeforeEach() {
	c.ctx, c.cancel = context.WithCancel(context.Background())

	c.testUpstream = v1helpers.NewTestHttpUpstream(c.ctx, c.EnvoyInstance().LocalAddr())

	c.runOptions = &services.RunOptions{
		NsToWrite: writeNamespace,
		NsToWatch: []string{"default", writeNamespace},
		WhatToRun: services.What{
			DisableFds: true,
			DisableUds: true,
		},
	}

	vsToTestUpstream := helpers.NewVirtualServiceBuilder().
		WithName("vs-test").
		WithNamespace(writeNamespace).
		WithDomain("test.com").
		WithRoutePrefixMatcher("test", "/").
		WithRouteActionToUpstream("test", c.testUpstream.Upstream).
		Build()

	// The set of resources that these tests will generate
	// Individual tests may modify these resources, but we provide the default resources
	// required to form a Proxy and handle requests
	c.resourcesToCreate = &gloosnapshot.ApiSnapshot{
		Gateways: v1.GatewayList{
			gatewaydefaults.DefaultGateway(writeNamespace),
		},
		VirtualServices: v1.VirtualServiceList{
			vsToTestUpstream,
		},
		Upstreams: gloov1.UpstreamList{
			c.testUpstream.Upstream,
		},
	}
}

func (c *TestContext) AfterEach() {
	// Stop Envoy
	c.envoyInstance.Clean()

	c.cancel()
}

func (c *TestContext) JustBeforeEach() {
	// Run Gloo
	c.testClients = services.RunGlooGatewayUdsFds(c.ctx, c.runOptions)

	// Run Envoy
	err := c.envoyInstance.RunWithRole(envoyRole, c.testClients.GlooPort)
	Expect(err).NotTo(HaveOccurred())

	// Create Resources
	err = c.testClients.WriteSnapshot(c.ctx, c.resourcesToCreate)
	Expect(err).NotTo(HaveOccurred())

	// Wait for a proxy to be accepted
	helpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
		return c.testClients.ProxyClient.Read(writeNamespace, gatewaydefaults.GatewayProxyName, clients.ReadOpts{Ctx: c.ctx})
	})
}

func (c *TestContext) JustAfterEach() {
	// We do not need to clean up the Snapshot that was written in the JustBeforeEach
	// That is because each test uses its own InMemoryCache
}

func (c *TestContext) Ctx() context.Context {
	return c.ctx
}

func (c *TestContext) ResourcesToCreate() *gloosnapshot.ApiSnapshot {
	return c.resourcesToCreate
}

func (c *TestContext) SetResourcesToCreate(snapshot *gloosnapshot.ApiSnapshot) {
	c.resourcesToCreate = snapshot
}

func (c *TestContext) EnvoyInstance() *services.EnvoyInstance {
	return c.envoyInstance
}

func (c *TestContext) TestUpstream() *v1helpers.TestUpstream {
	return c.testUpstream
}

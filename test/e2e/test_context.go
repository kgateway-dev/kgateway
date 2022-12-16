package e2e

import (
	"context"
	"fmt"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gatewaydefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/services"
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
	envoyInstance, err := f.EnvoyFactory.NewEnvoyInstance()
	Expect(err).NotTo(HaveOccurred())

	return &TestContext{
		envoyInstance: envoyInstance,
		runOptions: &services.RunOptions{
			NsToWrite: writeNamespace,
			NsToWatch: []string{"default", writeNamespace},
			WhatToRun: services.What{
				DisableFds: true,
				DisableUds: true,
			},
		},
	}
}

type TestContext struct {
	ctx           context.Context
	cancel        context.CancelFunc
	envoyInstance *services.EnvoyInstance

	runOptions  *services.RunOptions
	testClients services.TestClients

	resourcesToCreate *gloosnapshot.ApiSnapshot
}

func (c *TestContext) SetRunOptions(options *services.RunOptions) {
	c.runOptions = options
}

func (c *TestContext) BeforeEach() {
	c.ctx, c.cancel = context.WithCancel(context.Background())

	// The set of resources that these tests will generate
	c.resourcesToCreate = &gloosnapshot.ApiSnapshot{
		Gateways:        v1.GatewayList{},
		VirtualServices: v1.VirtualServiceList{},
	}
}

func (c *TestContext) AfterEach() {
	// Stop Envoy
	c.envoyInstance.Clean()

	c.cancel()
}

func (c *TestContext) JustBeforeEach() {
	// Run gloo
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

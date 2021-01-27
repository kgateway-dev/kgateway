package e2e_test

import (
	"context"

	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gwdefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"

	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/test/services"

	. "github.com/onsi/gomega"
)

type TestContext struct {
	Ctx            context.Context
	Cancel         context.CancelFunc
	TestClients    services.TestClients
	WriteNamespace string
	What           services.What

	beforeCalled bool
}

func (t *TestContext) Before() {
	if t.beforeCalled {
		return
	}
	t.beforeCalled = true
	if t.Ctx == nil {
		t.Ctx, t.Cancel = context.WithCancel(context.Background())
	}
	if defaults.HttpPort == 0 {
		defaults.HttpPort = services.NextBindPort()
	}
	if defaults.HttpsPort == 0 {
		defaults.HttpsPort = services.NextBindPort()
	}
	if t.WriteNamespace == "" {
		t.WriteNamespace = "gloo-system"
	}
	ro := &services.RunOptions{
		NsToWrite: t.WriteNamespace,
		NsToWatch: []string{"default", t.WriteNamespace},
		WhatToRun: t.What,
	}

	t.TestClients = services.RunGlooGatewayUdsFds(t.Ctx, ro)

}
func (t *TestContext) After() {
	if t.Cancel != nil {
		t.Cancel()
	}
}

func (t *TestContext) EnsureDefaultGateways() {
	err := helpers.WriteDefaultGateways(t.WriteNamespace, t.TestClients.GatewayClient)
	Expect(err).NotTo(HaveOccurred(), "Should be able to write default gateways")

	// wait for the two gateways to be created.
	Eventually(func() (gatewayv1.GatewayList, error) {
		return t.TestClients.GatewayClient.List(t.WriteNamespace, clients.ListOpts{})
	}, "10s", "0.1s").Should(HaveLen(2))
}

func (t TestContext) Role() string {
	return t.WriteNamespace + "~" + gwdefaults.GatewayProxyName
}

func (t TestContext) Port() uint32 {
	return uint32(t.TestClients.GlooPort)
}

func (t TestContext) RestXdsPort() uint32 {
	return uint32(t.TestClients.RestXdsPort)
}

func (t TestContext) Context() context.Context {
	return t.Ctx
}

package services

import (
	"context"
	"fmt"
	"net"
	"sync/atomic"

	"github.com/golang/protobuf/ptypes/duration"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/imdario/mergo"
	. "github.com/onsi/gomega"
	fdsrunner "github.com/solo-io/gloo/projects/discovery/pkg/fds/runner"
	udsrunner "github.com/solo-io/gloo/projects/discovery/pkg/uds/runner"
	"github.com/solo-io/gloo/projects/gloo/pkg/runner"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"

	"github.com/solo-io/gloo/pkg/utils/settingsutil"

	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"

	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"

	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/config"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
)

var glooPortBase = int32(30400)

func AllocateGlooPort() int32 {
	return atomic.AddInt32(&glooPortBase, 1) + int32(config.GinkgoConfig.ParallelNode*1000)
}

func RunGateway(ctx context.Context, justGloo bool) TestClients {
	ns := defaults.GlooSystem
	ro := &RunOptions{
		NsToWrite: ns,
		NsToWatch: []string{"default", ns},
		WhatToRun: What{
			DisableGateway: justGloo,
		},
	}
	return RunGlooGatewayUdsFds(ctx, ro)
}

type What struct {
	DisableGateway bool
	DisableUds     bool
	DisableFds     bool
}

type RunOptions struct {
	NsToWrite      string
	NsToWatch      []string
	WhatToRun      What
	GlooPort       int32
	ValidationPort int32
	RestXdsPort    int32
	Settings       *gloov1.Settings
}

// RunGlooGatewayUdsFds accepts at configurable set of RunOptions
// and starts Gloo, UDS and FDS in separate goroutines to simulate an in memory instance of Gloo Edge
func RunGlooGatewayUdsFds(ctx context.Context, runOptions *RunOptions) TestClients {
	// Allocate any required ports which were not explicitly set
	if runOptions.GlooPort == 0 {
		runOptions.GlooPort = AllocateGlooPort()
	}
	if runOptions.ValidationPort == 0 {
		runOptions.ValidationPort = AllocateGlooPort()
	}
	if runOptions.RestXdsPort == 0 {
		runOptions.RestXdsPort = AllocateGlooPort()
	}
	if runOptions.Settings == nil {
		runOptions.Settings = &gloov1.Settings{}
	}

	// Initialize the Settings based on the RunOptions
	settings := &gloov1.Settings{
		WatchNamespaces:    runOptions.NsToWatch,
		DiscoveryNamespace: runOptions.NsToWrite,
		DevMode:            true,
		RefreshRate: &duration.Duration{
			Seconds: 1,
		},
		Gloo: &gloov1.GlooOptions{
			RestXdsBindAddr:    fmt.Sprintf("%s:%d", net.IPv4zero.String(), runOptions.RestXdsPort),
			ValidationBindAddr: fmt.Sprintf("%s:%d", net.IPv4zero.String(), runOptions.ValidationPort),
			XdsBindAddr:        fmt.Sprintf("%s:%d", net.IPv4zero.String(), runOptions.GlooPort),
			ProxyDebugBindAddr: fmt.Sprintf("%s:%d", net.IPv4zero.String(), AllocateGlooPort()),
			RemoveUnusedFilters: &wrappers.BoolValue{
				// Setting this to true would be preferred, but certain tests failed (consul_vault_test)
				// The failure indicates that the feature isn't entirely correct.
				// The case that fails is when transformations are set at the route level by other plugins
				// but then the Transformation HTTP filter is never added to the filter chain
				Value: false,
			},
		},
		Gateway: &gloov1.GatewayOptions{
			PersistProxySpec: &wrappers.BoolValue{
				Value: true,
			},
			EnableGatewayController: &wrappers.BoolValue{
				Value: !runOptions.WhatToRun.DisableGateway,
			},
		},
	}

	// Initialize the caches used by the Runners
	inMemoryCache := memory.NewInMemoryResourceCache()
	kubeCache := kube.NewKubeCache(ctx)

	// Override any Settings explicitly defined by a test
	err := mergo.Merge(settings, runOptions.Settings, mergo.WithOverride)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	ctx = settingsutil.WithSettings(ctx, settings)

	// Run Gloo
	glooRunner := runner.NewGlooRunner()
	runErr := glooRunner.Run(ctx, kubeCache, inMemoryCache, settings)
	ExpectWithOffset(1, runErr).NotTo(HaveOccurred())
	resourceClientset := glooRunner.GetResourceClientset()
	typedClientset := glooRunner.GetTypedClientset()

	// Run FDS (if necessary)
	if !runOptions.WhatToRun.DisableFds {
		go func() {
			defer GinkgoRecover()

			fdsRunner := fdsrunner.NewFDSRunner()
			err := fdsRunner.Run(ctx, kubeCache, inMemoryCache, settings)
			ExpectWithOffset(1, err).NotTo(HaveOccurred())
		}()
	}

	// Run UDS (if necessary)
	if !runOptions.WhatToRun.DisableUds {
		go func() {
			defer GinkgoRecover()

			udsRunner := udsrunner.NewUDSRunner()
			err := udsRunner.Run(ctx, kubeCache, inMemoryCache, settings)
			ExpectWithOffset(1, err).NotTo(HaveOccurred())

		}()
	}

	return TestClients{
		GatewayClient:        resourceClientset.Gateways,
		HttpGatewayClient:    resourceClientset.MatchableHttpGateways,
		VirtualServiceClient: resourceClientset.VirtualServices,
		UpstreamClient:       resourceClientset.Upstreams,
		SecretClient:         resourceClientset.Secrets,
		ProxyClient:          resourceClientset.Proxies,
		ServiceClient:        typedClientset.KubeServiceClient,
		GlooPort:             int(runOptions.GlooPort),
		RestXdsPort:          int(runOptions.RestXdsPort),
	}
}

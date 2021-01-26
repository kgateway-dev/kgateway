package e2e_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	fault "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/faultinjection"
	"github.com/solo-io/gloo/test/services"
	"github.com/solo-io/gloo/test/v1helpers"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/utils/prototime"
)

var _ = Describe("Fault Injection", func() {

	var (
		testClients services.TestClients
		ctx         context.Context
	)

	BeforeEach(func() {
		ctx, _ = context.WithCancel(context.Background())
		t := services.RunGateway(ctx, true)
		testClients = t
	})

	Context("with envoy", func() {

		var (
			envoyInstance *services.EnvoyInstance
			up            *gloov1.Upstream
			opts          clients.WriteOpts
		)

		setupProxy := func(proxy *gloov1.Proxy, up *gloov1.Upstream) error {
			proxyCli := testClients.ProxyClient
			_, err := proxyCli.Write(proxy, opts)
			return err
		}

		envoyPort := services.NextBindPort()

		setupInitialProxy := func() {
			proxy := getGlooProxyWithVersion(nil, nil, envoyPort, up, "")
			err := setupProxy(proxy, up)
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() error {
				_, err := http.Get(fmt.Sprintf("http://%s:%d/status/200", "localhost", envoyPort))
				if err != nil {
					return err
				}
				return nil
			}, "20s", ".1s").Should(BeNil())
		}

		setupUpstream := func() {
			tu := v1helpers.NewTestHttpUpstream(ctx, envoyInstance.LocalAddr())
			// drain channel as we dont care about it
			go func() {
				for range tu.C {
				}
			}()
			var opts clients.WriteOpts
			up = tu.Upstream
			_, err := testClients.UpstreamClient.Write(up, opts)
			Expect(err).NotTo(HaveOccurred())
		}

		BeforeEach(func() {
			var err error
			envoyInstance, err = envoyFactory.NewEnvoyInstance()
			Expect(err).NotTo(HaveOccurred())

			err = envoyInstance.RunWithRoleAndRestXds(services.DefaultProxyName, testClients.GlooPort, testClients.RestXdsPort)
			Expect(err).NotTo(HaveOccurred())

			setupUpstream()
			setupInitialProxy()
		})

		AfterEach(func() {
			if envoyInstance != nil {
				envoyInstance.Clean()
			}
		})

		It("should cause envoy abort fault", func() {
			abort := &fault.RouteAbort{
				HttpStatus: uint32(503),
				Percentage: float32(100),
			}

			Eventually(func() error {
				proxy, err := getGlooProxy(testClients, abort, nil, envoyPort, up)
				if err != nil {
					return err
				}
				opts.OverwriteExisting = true
				return setupProxy(proxy, up)
			}, "20s", ".1s").Should(BeNil())

			Eventually(func() error {
				res, err := http.Get(fmt.Sprintf("http://%s:%d/status/200", "localhost", envoyPort))
				if err != nil {
					return err
				}
				if res.StatusCode != http.StatusServiceUnavailable {
					return errors.New(fmt.Sprintf("%v is not ServiceUnavailable", res.StatusCode))
				}
				return nil
			}, "20s", ".1s").Should(BeNil())
		})

		It("should cause envoy delay fault", func() {
			fixedDelay := prototime.DurationToProto(time.Second * 3)
			delay := &fault.RouteDelay{
				FixedDelay: fixedDelay,
				Percentage: float32(100),
			}

			Eventually(func() error {
				proxy, err := getGlooProxy(testClients, nil, delay, envoyPort, up)
				if err != nil {
					return err
				}
				opts.OverwriteExisting = true
				return setupProxy(proxy, up)
			}, "20s", ".1s").Should(BeNil())

			Eventually(func() error {
				start := time.Now()
				_, err := http.Get(fmt.Sprintf("http://%s:%d/status/200", "localhost", envoyPort))
				if err != nil {
					return err
				}
				elapsed := time.Now().Sub(start)
				// This test regularly flakes, and the error is usually of the form:
				// "Elapsed time 2.998280684s not longer than delay 3s"
				// There's a small precision issue when communicating with Envoy, so including a small
				// margin of error to eliminate the test flake.
				marginOfError := 100 * time.Millisecond
				if elapsed+marginOfError < (3 * time.Second) {
					return errors.New(fmt.Sprintf("Elapsed time %s not longer than delay %s", elapsed.String(), fixedDelay.String()))
				}
				return nil
			}, "20s", ".1s").Should(BeNil())

		})
	})
})

func getGlooProxy(testClients services.TestClients, abort *fault.RouteAbort, delay *fault.RouteDelay, envoyPort uint32, up *gloov1.Upstream) (*gloov1.Proxy, error) {
	readProxy, err := testClients.ProxyClient.Read("default", "proxy", clients.ReadOpts{})
	if err != nil {
		return nil, err
	}
	return getGlooProxyWithVersion(abort, delay, envoyPort, up, readProxy.Metadata.ResourceVersion), nil
}

func getGlooProxyWithVersion(abort *fault.RouteAbort, delay *fault.RouteDelay, envoyPort uint32, up *gloov1.Upstream, resourceVersion string) *gloov1.Proxy {
	return &gloov1.Proxy{
		Metadata: &core.Metadata{
			Name:            "proxy",
			Namespace:       "default",
			ResourceVersion: resourceVersion,
		},
		Listeners: []*gloov1.Listener{{
			Name:        "listener",
			BindAddress: "127.0.0.1",
			BindPort:    envoyPort,
			ListenerType: &gloov1.Listener_HttpListener{
				HttpListener: &gloov1.HttpListener{
					VirtualHosts: []*gloov1.VirtualHost{{
						Name:    "virt1",
						Domains: []string{"*"},
						Routes: []*gloov1.Route{{
							Action: &gloov1.Route_RouteAction{
								RouteAction: &gloov1.RouteAction{
									Destination: &gloov1.RouteAction_Single{
										Single: &gloov1.Destination{
											DestinationType: &gloov1.Destination_Upstream{
												Upstream: up.Metadata.Ref(),
											},
										},
									},
								},
							},
							Options: &gloov1.RouteOptions{
								Faults: &fault.RouteFaults{
									Abort: abort,
									Delay: delay,
								},
							},
						}},
						Options: &gloov1.VirtualHostOptions{},
					}},
				},
			},
		}},
	}
}

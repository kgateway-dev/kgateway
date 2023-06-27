package e2e_test

import (
	"context"
	"errors"
	"time"

	"github.com/golang/protobuf/ptypes/duration"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/test/e2e"
	"github.com/solo-io/gloo/test/gomega/matchers"

	"github.com/solo-io/gloo/test/testutils"

	consulplugin "github.com/solo-io/gloo/projects/gloo/pkg/plugins/consul"

	"github.com/solo-io/gloo/test/helpers"
	"google.golang.org/protobuf/types/known/wrapperspb"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hashicorp/consul/api"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	consulapi "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/consul"
	"github.com/solo-io/gloo/projects/gloo/pkg/upstreams/consul"
	"github.com/solo-io/gloo/test/services"
	"github.com/solo-io/gloo/test/v1helpers"
)

var _ = Describe("Consul e2e", Serial, func() {

	// These tests use the Serial decorator because they rely on a hard-coded port for the Consul agent (8300)

	Context("Consul Service Registry", func() {

		var (
			svc1, svc2, svc3 *v1helpers.TestUpstream
			testContext      *e2e.TestContextWithConsul
		)

		BeforeEach(func() {
			testContext = testContextFactory.NewTestContextWithConsul()
			testContext.BeforeEach()

			testContext.SetRunSettings(&gloov1.Settings{
				Consul: &gloov1.Settings_ConsulConfiguration{
					DnsAddress: consulplugin.DefaultDnsAddress,
					DnsPollingInterval: &duration.Duration{
						Seconds: 1,
					},
					ServiceDiscovery: &gloov1.Settings_ConsulConfiguration_ServiceDiscoveryOptions{
						DataCenters: nil, // Use all available data-centers
					},
				},
				ConsulDiscovery: &gloov1.Settings_ConsulUpstreamDiscoveryConfiguration{
					ServiceTagsAllowlist: []string{"1", "2"},
				},
			})

			// Run Consul
			testContext.RunConsul()

			// Run web applications
			// We don't need to create the corresponding upstream because we are routing directly to consul
			svc1 = v1helpers.NewTestHttpUpstreamWithReply(testContext.Ctx(), testContext.EnvoyInstance().LocalAddr(), "svc-1")
			svc2 = v1helpers.NewTestHttpUpstreamWithReply(testContext.Ctx(), testContext.EnvoyInstance().LocalAddr(), "svc-2")
			svc3 = v1helpers.NewTestHttpUpstreamWithReply(testContext.Ctx(), testContext.EnvoyInstance().LocalAddr(), "svc-3")

			// Register services with consul
			consulInstance := testContext.ConsulInstance()
			err := consulInstance.RegisterService("my-svc", "my-svc-1", testContext.EnvoyInstance().GlooAddr, []string{"svc", "1"}, svc1.Port)
			Expect(err).NotTo(HaveOccurred())
			err = consulInstance.RegisterService("my-svc", "my-svc-2", testContext.EnvoyInstance().GlooAddr, []string{"svc", "2"}, svc2.Port)
			Expect(err).NotTo(HaveOccurred())
			//we should not discover this service as it will be filtered out
			err = consulInstance.RegisterService("my-svc-1", "my-svc-3", testContext.EnvoyInstance().GlooAddr, []string{"svc", "3"}, svc3.Port)
			Expect(err).NotTo(HaveOccurred())

			vsToConsulService := helpers.NewVirtualServiceBuilder().
				WithName(e2e.DefaultVirtualServiceName).
				WithNamespace(writeNamespace).
				WithDomain(e2e.DefaultHost).
				WithRoutePrefixMatcher(e2e.DefaultRouteName, "/").
				WithRouteActionToSingleDestination(e2e.DefaultRouteName, &gloov1.Destination{
					DestinationType: &gloov1.Destination_Consul{
						Consul: &gloov1.ConsulServiceDestination{
							ServiceName: "my-svc",
							Tags:        []string{"svc", "1"},
						},
					},
				}).
				Build()

			testContext.ResourcesToCreate().VirtualServices = v1.VirtualServiceList{
				vsToConsulService,
			}
		})

		AfterEach(func() {
			testContext.AfterEach()
		})

		JustBeforeEach(func() {
			testContext.JustBeforeEach()
		})

		JustAfterEach(func() {
			testContext.JustAfterEach()
		})

		It("works as expected", func() {
			requestBuilder := testContext.GetHttpRequestBuilder()

			Eventually(func(g Gomega) {
				g.Expect(testutils.DefaultHttpClient.Do(requestBuilder.Build())).To(matchers.HaveExactResponseBody("svc-1"))
			}, "20s", ".1s").Should(Succeed(), "Eventually requests should only go to service with tag `1`")
			Consistently(func(g Gomega) {
				g.Expect(testutils.DefaultHttpClient.Do(requestBuilder.Build())).To(matchers.HaveExactResponseBody("svc-1"))
			}, "2s", ".2s").Should(Succeed(), "Consistently requests should only go to service with tag `1`")

			err := testContext.ConsulInstance().RegisterService("my-svc", "my-svc-2", testContext.EnvoyInstance().GlooAddr, []string{"svc", "1"}, svc2.Port)
			Expect(err).NotTo(HaveOccurred())

			// svc2 first to ensure we also still route to svc1 after registering svc2
			Eventually(func(g Gomega) {
				g.Expect(testutils.DefaultHttpClient.Do(requestBuilder.Build())).To(matchers.HaveExactResponseBody("svc-2"))
			}, "20s", ".1s").Should(Succeed(), "Eventually requests should only go to service with tag `2`")
			Eventually(func(g Gomega) {
				g.Expect(testutils.DefaultHttpClient.Do(requestBuilder.Build())).To(matchers.HaveExactResponseBody("svc-1"))
			}, "20s", ".1s").Should(Succeed(), "Eventually requests should only go to service with tag `1`")
		})

		Context("Using Hostname address (as opposed to IPs addresses)", func() {

			BeforeEach(func() {
				// These tests only seem to pass on a Linux machine, I have not investigated why
				testutils.ValidateRequirementsAndNotifyGinkgo(testutils.LinuxOnly("Unknown"))

				err := testContext.ConsulInstance().RegisterService("my-svc", "my-svc-1", "my-svc.service.dc1.consul", []string{"svc", "1"}, svc1.Port)
				Expect(err).NotTo(HaveOccurred())
			})

			It("resolves consul services", func() {
				requestBuilder := testContext.GetHttpRequestBuilder()

				Eventually(func(g Gomega) {
					g.Expect(testutils.DefaultHttpClient.Do(requestBuilder.Build())).To(matchers.HaveExactResponseBody("svc-1"))
				}, "20s", ".1s").Should(Succeed(), "Eventually requests should only go to service with tag `1`")
				Consistently(func(g Gomega) {
					g.Expect(testutils.DefaultHttpClient.Do(requestBuilder.Build())).To(matchers.HaveExactResponseBody("svc-1"))
				}, "2s", ".2s").Should(Succeed(), "Consistently requests should only go to service with tag `1`")
			})
		})

		Context("EDS only updates", func() {

			var (
				defaultConsulSettings = &gloov1.Settings{
					Consul: &gloov1.Settings_ConsulConfiguration{
						DnsAddress: consulplugin.DefaultDnsAddress,
						DnsPollingInterval: &duration.Duration{
							Seconds: 1,
						},
						ServiceDiscovery: &gloov1.Settings_ConsulConfiguration_ServiceDiscoveryOptions{
							DataCenters: nil, // Use all available data-centers
						},
					},
					ConsulDiscovery: &gloov1.Settings_ConsulUpstreamDiscoveryConfiguration{
						ServiceTagsAllowlist: []string{"1", "2"},
					},
				}
			)

			runTest := func() {

				By("requests only go to endpoints behind test upstream 1")
				requestBuilder := testContext.GetHttpRequestBuilder()

				Eventually(func(g Gomega) {
					g.Expect(testutils.DefaultHttpClient.Do(requestBuilder.Build())).To(matchers.HaveExactResponseBody("svc-1"))
				}, "20s", ".1s").Should(Succeed(), "Eventually requests should only go to service with tag `1`")
				Consistently(func(g Gomega) {
					g.Expect(testutils.DefaultHttpClient.Do(requestBuilder.Build())).To(matchers.HaveExactResponseBody("svc-1"))
				}, "2s", ".2s").Should(Succeed(), "Consistently requests should only go to service with tag `1`")

				// update service one to point to test upstream 2 port
				err := testContext.ConsulInstance().RegisterService("my-svc", "my-svc-1", testContext.EnvoyInstance().GlooAddr, []string{"svc", "1"}, svc2.Port)
				Expect(err).NotTo(HaveOccurred())

				By("requests only go to endpoints behind test upstream 2")

				// ensure EDS picked up this endpoint-only change
				// test upstream 1 endpoint is now stale; should only get requests to endpoints for test upstream 2 for svc1
				Eventually(func(g Gomega) {
					g.Expect(testutils.DefaultHttpClient.Do(requestBuilder.Build())).To(matchers.HaveExactResponseBody("svc-2"))
				}, "20s", ".1s").Should(Succeed(), "Eventually requests should only go to service with tag `2`")
				Consistently(func(g Gomega) {
					g.Expect(testutils.DefaultHttpClient.Do(requestBuilder.Build())).To(matchers.HaveExactResponseBody("svc-2"))
				}, "2s", ".2s").Should(Succeed(), "Consistently requests should only go to service with tag `1`")
			}

			Context("non-blocking EDS queries", func() {

				BeforeEach(func() {
					defaultConsulSettings.ConsulDiscovery.EdsBlockingQueries = &wrapperspb.BoolValue{Value: false}
					testContext.SetRunSettings(defaultConsulSettings)
				})

				It("works as expected", func() {
					runTest()
				})
			})

			Context("blocking EDS queries", func() {

				BeforeEach(func() {
					defaultConsulSettings.ConsulDiscovery.EdsBlockingQueries = &wrapperspb.BoolValue{Value: true}
					testContext.SetRunSettings(defaultConsulSettings)
				})

				It("works as expected", func() {
					runTest()
				})
			})

		})

	})

	Context("Consul Golang Client / Consul CLI Differences", func() {
		// This test was written to prove that the consul golang client behaves differently than the consul CLI, and thus
		// that our `refreshSpecs()` usage in consul eds.go is correct and does not miss updates (which also allows
		// us to make performance optimizations at scale, since our current implementation has a lot more cache hits).

		var (
			ctx    context.Context
			cancel context.CancelFunc

			consulInstance       *services.ConsulInstance
			serviceTagsAllowlist []string
			consulWatcher        consul.ConsulWatcher
		)

		BeforeEach(func() {
			ctx, cancel = context.WithCancel(context.Background())

			consulInstance = consulFactory.MustConsulInstance()

			err := consulInstance.Run(ctx)
			Expect(err).NotTo(HaveOccurred())

			// init consul client
			client, err := api.NewClient(api.DefaultConfig())
			Expect(err).NotTo(HaveOccurred())

			serviceTagsAllowlist = []string{"1", "2"}

			consulWatcher, err = consul.NewConsulWatcher(client, nil, serviceTagsAllowlist)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			cancel()
		})

		// This test was written to prove that the consul golang client behaves differently than the consul CLI, and thus
		// that our `refreshSpecs()` usage in consul eds.go is correct and does not miss updates (which also allows
		// us to make performance optimizations at scale, since our current implementation has a lot more cache hits).
		It("fires service watch even if catalog service is the only update", func() {
			svcsChan, errChan := consulWatcher.WatchServices(ctx, []string{"dc1"}, consulapi.ConsulConsistencyModes_DefaultMode, nil)

			// use select instead of eventually for easier debugging.
			select {
			case err := <-errChan:
				Expect(err).NotTo(HaveOccurred())
				Fail("err chan closed prematurely")
			case svcsReceived := <-svcsChan:
				// the default consul svc in dc1 does not show up in our watch because of service tag filtering
				Expect(svcsReceived).To(HaveLen(0))
			case <-time.After(5 * time.Second):
				Fail("timeout waiting for services")
			}

			Consistently(func() error {
				select {
				case err := <-errChan:
					ExpectWithOffset(1, err).NotTo(HaveOccurred())
					return errors.New("err chan closed prematurely")
				case svcsReceived := <-svcsChan:
					// happy path, continue
					ExpectWithOffset(1, svcsReceived).To(HaveLen(0))
				case <-time.After(100 * time.Millisecond):
					// happy path, continue
				}
				return nil
			}, "2s", "0.2s").Should(Succeed())

			// now that consul has a chance to update raft index with all internal updates; we should see no more updates
			Consistently(func() error {
				select {
				case err := <-errChan:
					ExpectWithOffset(1, err).NotTo(HaveOccurred())
					return errors.New("err chan closed prematurely")
				case svcsReceived := <-svcsChan:
					ExpectWithOffset(1, svcsReceived).To(HaveLen(0)) // we actually expect len(0) if anything; this is just here to get a nice output / diff before we fail regardless
					Fail("did not expect to receive empty services")
				case <-time.After(100 * time.Millisecond):
					// happy path, continue
				}
				return nil
			}, "2s", "0.2s").Should(Succeed())

			// add a single service.
			// this will fire a watch via consul CLI for both:
			// - consul watch -type=service -service my-svc ./echo.sh
			// - consul watch -type=services ./echo.sh
			//
			// as the service is new
			//
			// echo.sh is just a shell script with the contents: `echo "watch fired!"` so we can check that the watch fired.
			err := consulInstance.RegisterLiveService("my-svc", "my-svc-1", "127.0.0.1", []string{"svc", "1"}, 80)
			Expect(err).NotTo(HaveOccurred())

			// use select instead of eventually for easier debugging.
			select {
			case err := <-errChan:
				Expect(err).NotTo(HaveOccurred())
				Fail("err chan closed prematurely")
			case svcsReceived := <-svcsChan:
				// the default consul svc in dc1 does not show up in our watch because of service tag filtering
				Expect(svcsReceived).To(HaveLen(1))
				Expect(svcsReceived[0].Name).To(Equal("my-svc"))
				Expect(svcsReceived[0].Tags).To(ConsistOf([]string{"svc", "1"}))
				Expect(svcsReceived[0].DataCenters).To(ConsistOf([]string{"dc1"}))
			case <-time.After(5 * time.Second):
				Fail("timeout waiting for services")
			}

			Consistently(func() error {
				select {
				case err := <-errChan:
					ExpectWithOffset(1, err).NotTo(HaveOccurred())
					return errors.New("err chan closed prematurely")
				case svcsReceived := <-svcsChan:
					// happy path, continue
					ExpectWithOffset(1, svcsReceived).To(HaveLen(1))
					ExpectWithOffset(1, svcsReceived[0].Name).To(Equal("my-svc"))
					ExpectWithOffset(1, svcsReceived[0].Tags).To(ConsistOf([]string{"svc", "1"}))
					ExpectWithOffset(1, svcsReceived[0].DataCenters).To(ConsistOf([]string{"dc1"}))
				case <-time.After(100 * time.Millisecond):
					// happy path, continue
				}
				return nil
			}, "2s", "0.2s").Should(Succeed())

			// now that consul has a chance to update raft index with all internal updates; we should see no more updates
			Consistently(func() error {
				select {
				case err := <-errChan:
					ExpectWithOffset(1, err).NotTo(HaveOccurred())
					return errors.New("err chan closed prematurely")
				case svcsReceived := <-svcsChan:
					ExpectWithOffset(1, svcsReceived).To(HaveLen(0)) // we actually expect len(1) if anything; this is just here to get a nice output / diff before we fail regardless
					Fail("did not expect to receive services")
				case <-time.After(100 * time.Millisecond):
					// happy path, continue
				}
				return nil
			}, "2s", "0.2s").Should(Succeed())

			// update an existing service.
			// this will fire a watch via consul CLI only for:
			// - consul watch -type=service -service my-svc ./echo.sh
			//
			// as the service is new
			//
			// However this WILL fire an update on our golang client watch for services (last index increments)
			// and thus we can depend on this to signal that we should query again for all catalog services in eds.go
			//
			// It appears the golang client lastIndex mirrors the raft index (per `consul info`), and while dated,
			// this behavior still seems to hold: https://github.com/hashicorp/consul/issues/1244#issuecomment-141146851
			//
			// In the event this behavior changes (this test fails on newer consul versions),
			// we may need to move completely to the `eds_blocking_queries` as true despite the performance implications
			// for correctness.
			err = consulInstance.RegisterLiveService("my-svc", "my-svc-1", "127.0.0.1", []string{"svc", "1"}, 81)
			Expect(err).NotTo(HaveOccurred())

			// this is where golang client differs from cli! CLI registers watch on svc but not svcs; but we get for both
			// use select instead of eventually for easier debugging.
			select {
			case err := <-errChan:
				Expect(err).NotTo(HaveOccurred())
				Fail("err chan closed prematurely")
			case svcsReceived := <-svcsChan:
				// the default consul svc in dc1 does not show up in our watch
				Expect(svcsReceived).To(HaveLen(1))
				Expect(svcsReceived[0].Name).To(Equal("my-svc"))
				Expect(svcsReceived[0].Tags).To(ConsistOf([]string{"svc", "1"}))
				Expect(svcsReceived[0].DataCenters).To(ConsistOf([]string{"dc1"}))
			case <-time.After(5 * time.Second):
				Fail("timeout waiting for services")
			}

			Consistently(func() error {
				select {
				case err := <-errChan:
					ExpectWithOffset(1, err).NotTo(HaveOccurred())
					return errors.New("err chan closed prematurely")
				case svcsReceived := <-svcsChan:
					// happy path, continue
					ExpectWithOffset(1, svcsReceived).To(HaveLen(1))
					ExpectWithOffset(1, svcsReceived[0].Name).To(Equal("my-svc"))
					ExpectWithOffset(1, svcsReceived[0].Tags).To(ConsistOf([]string{"svc", "1"}))
					ExpectWithOffset(1, svcsReceived[0].DataCenters).To(ConsistOf([]string{"dc1"}))
				case <-time.After(100 * time.Millisecond):
					// happy path, continue
				}
				return nil
			}, "2s", "0.2s").Should(Succeed())

			// now that consul has a chance to update raft index with all internal updates; we should see no more updates
			Consistently(func() error {
				select {
				case err := <-errChan:
					ExpectWithOffset(1, err).NotTo(HaveOccurred())
					return errors.New("err chan closed prematurely")
				case svcsReceived := <-svcsChan:
					ExpectWithOffset(1, svcsReceived).To(HaveLen(0)) // we actually expect len(1) if anything; this is just here to get a nice output / diff before we fail regardless
					Fail("did not expect to receive services")
				case <-time.After(100 * time.Millisecond):
					// happy path, continue
				}
				return nil
			}, "2s", "0.2s").Should(Succeed())
		})

	})

})

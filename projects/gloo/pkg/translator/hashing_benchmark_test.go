package translator_test

import (
	"context"
	"encoding/json"
	"time"

	"github.com/solo-io/gloo/test/ginkgo/decorators"

	"github.com/solo-io/gloo/test/ginkgo/labels"

	"github.com/onsi/gomega/gmeasure"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"

	gloo_matchers "github.com/solo-io/gloo/test/gomega/matchers"

	v1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"

	v3 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/config/core/v3"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/registry"
	glooutils "github.com/solo-io/gloo/projects/gloo/pkg/utils"
	validationutils "github.com/solo-io/gloo/projects/gloo/pkg/utils/validation"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/solo-io/gloo/projects/gloo/pkg/translator"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/protoc-gen-ext/pkg/hasher/hashstructure"
	"github.com/solo-io/solo-kit/pkg/utils/protoutils"
)

// These tests are meant to demonstrate that FNV hashing is more efficient than hashstructure hashing
var _ = Describe("Hashing Benchmarks", decorators.Performance, Label(labels.Performance), func() {
	var allUpstreams v1.UpstreamList
	BeforeEach(func() {
		var upstreams []interface{}
		data := helpers.MustReadFile("upstream_list.json")
		err := json.Unmarshal(data, &upstreams)
		Expect(err).NotTo(HaveOccurred())
		for _, usInt := range upstreams {
			usMap := usInt.(map[string]interface{})
			var us v1.Upstream
			err = protoutils.UnmarshalMap(usMap, &us)
			Expect(err).NotTo(HaveOccurred())
			allUpstreams = append(allUpstreams, &us)
		}

		// Sort merged slice for consistent hashing
		allUpstreams.Sort()
	})

	It("should demonstrate that generated hashing is more efficient than reflection-based", func() {
		reflectionName, generatedName := "reflection-based", "generated"

		experiment := gmeasure.NewExperiment("hash comparison")

		experiment.Sample(func(idx int) {
			// Measure the time it takes to hash all upstreams via each method
			experiment.MeasureDuration(reflectionName, func() {
				for _, us := range allUpstreams {
					_, _ = hashstructure.Hash(us, nil)
				}
			})

			experiment.MeasureDuration(generatedName, func() {
				for _, us := range allUpstreams {
					_, _ = us.Hash(nil)
				}
			})

		}, gmeasure.SamplingConfig{N: 10, Duration: 10 * time.Second})

		ranking := gmeasure.RankStats(gmeasure.LowerMedianIsBetter, experiment.GetStats(reflectionName), experiment.GetStats(generatedName))
		AddReportEntry("hash comparison ranking", ranking)

		Expect(ranking.Winner().MeasurementName).To(Equal(generatedName), "expect the generated hashing method to be more efficient")
	})

	Context("accuracy", func() {
		It("Exhaustive", func() {
			present := make(map[uint64]*v1.Upstream, len(allUpstreams))
			for _, v := range allUpstreams {
				hash, err := v.Hash(nil)
				Expect(err).NotTo(HaveOccurred())
				val, ok := present[hash]
				if ok {
					Expect(v.Equal(val))
				}
				present[hash] = v
			}
		})
	})

	Context("Benchmark Translation With Different Hashers", func() {

		var (
			settings                *v1.Settings
			fnvTranslator           Translator
			hashstructureTranslator Translator
			upName                  *core.Metadata
			proxy                   *v1.Proxy
			params                  plugins.Params
			registeredPlugins       []plugins.Plugin
			routes                  []*v1.Route
		)

		beforeEach := func() {

			settings = &v1.Settings{}
			memoryClientFactory := &factory.MemoryResourceClientFactory{
				Cache: memory.NewInMemoryResourceCache(),
			}
			opts := bootstrap.Opts{
				Settings:  settings,
				Secrets:   memoryClientFactory,
				Upstreams: memoryClientFactory,
			}
			registeredPlugins = registry.Plugins(opts)

			upName = &core.Metadata{
				Name:      "kube-svc:default-helloworld-1-xvgfm-q2ktv-9090",
				Namespace: "default",
			}

			endpoints := v1.EndpointList{}
			for _, us := range allUpstreams {
				ep := &v1.Endpoint{
					Upstreams: []*core.ResourceRef{us.Metadata.Ref()},
					Address:   "1.2.3.4",
					Port:      32,
					Metadata: &core.Metadata{
						Name:      us.Metadata.Ref().GetName(),
						Namespace: us.Metadata.Ref().GetNamespace(),
					},
				}
				endpoints = append(endpoints, ep)
			}

			params = plugins.Params{
				Ctx: context.TODO(),
				Snapshot: &v1snap.ApiSnapshot{
					Endpoints: endpoints,
					Upstreams: allUpstreams,
				},
			}
			routes = []*v1.Route{{
				Name:     "testRouteName",
				Matchers: []*matchers.Matcher{defaults.DefaultMatcher()},
				Action: &v1.Route_RouteAction{
					RouteAction: &v1.RouteAction{
						Destination: &v1.RouteAction_Single{
							Single: &v1.Destination{
								DestinationType: &v1.Destination_Upstream{
									Upstream: upName.Ref(),
								},
							},
						},
					},
				},
			}}
		}

		BeforeEach(beforeEach)

		JustBeforeEach(func() {
			pluginRegistry := registry.NewPluginRegistry(registeredPlugins)

			fnvTranslator = NewTranslatorWithHasher(glooutils.NewSslConfigTranslator(), settings, pluginRegistry, EnvoyCacheResourcesListToFnvHash)
			hashstructureTranslator = NewTranslatorWithHasher(glooutils.NewSslConfigTranslator(), settings, pluginRegistry, EnvoyCacheResourcesListToHash)

			httpListener := &v1.Listener{
				Name:        "http-listener",
				BindAddress: "127.0.0.1",
				BindPort:    80,
				ListenerType: &v1.Listener_HttpListener{
					HttpListener: &v1.HttpListener{
						VirtualHosts: []*v1.VirtualHost{{
							Name:    "virt1",
							Domains: []string{"*"},
							Routes:  routes,
						}},
					},
				},
			}
			tcpListener := &v1.Listener{
				Name:        "tcp-listener",
				BindAddress: "127.0.0.1",
				BindPort:    8080,
				ListenerType: &v1.Listener_TcpListener{
					TcpListener: &v1.TcpListener{
						TcpHosts: []*v1.TcpHost{
							{
								Destination: &v1.TcpHost_TcpAction{
									Destination: &v1.TcpHost_TcpAction_Single{
										Single: &v1.Destination{
											DestinationType: &v1.Destination_Upstream{
												Upstream: upName.Ref(),
											},
										},
									},
								},
							},
						},
					},
				},
			}
			hybridListener := &v1.Listener{
				Name:        "hybrid-listener",
				BindAddress: "127.0.0.1",
				BindPort:    8081,
				ListenerType: &v1.Listener_HybridListener{
					HybridListener: &v1.HybridListener{
						MatchedListeners: []*v1.MatchedListener{
							{
								Matcher: &v1.Matcher{
									SourcePrefixRanges: []*v3.CidrRange{
										{
											AddressPrefix: "0.0.0.0",
											PrefixLen: &wrappers.UInt32Value{
												Value: 1,
											},
										},
									},
								},
								ListenerType: &v1.MatchedListener_HttpListener{
									HttpListener: &v1.HttpListener{
										VirtualHosts: []*v1.VirtualHost{{
											Name:    "virt1",
											Domains: []string{"*"},
											Routes:  routes,
										}},
									},
								},
							},
							{
								Matcher: &v1.Matcher{
									SourcePrefixRanges: []*v3.CidrRange{
										{
											AddressPrefix: "255.0.0.0",
											PrefixLen: &wrappers.UInt32Value{
												Value: 1,
											},
										},
									},
								},
								ListenerType: &v1.MatchedListener_TcpListener{
									TcpListener: &v1.TcpListener{
										TcpHosts: []*v1.TcpHost{
											{
												Destination: &v1.TcpHost_TcpAction{
													Destination: &v1.TcpHost_TcpAction_Single{
														Single: &v1.Destination{
															DestinationType: &v1.Destination_Upstream{
																Upstream: upName.Ref(),
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			}

			proxy = &v1.Proxy{
				Metadata: &core.Metadata{
					Name:      "test",
					Namespace: "gloo-system",
				},
				Listeners: []*v1.Listener{
					httpListener,
					tcpListener,
					hybridListener,
				},
			}
		})

		It("should demonstrate that FNV translator is more efficient than hashstructure translator", func() {
			var (
				snap                       cache.Snapshot
				errs                       reporter.ResourceReports
				report                     *validation.ProxyReport
				fnvName, hashstructureName = "fnv", "hashstructure"
			)

			proxyClone := proto.Clone(proxy).(*v1.Proxy)
			proxyClone.GetListeners()[0].Options = &v1.ListenerOptions{PerConnectionBufferLimitBytes: &wrappers.UInt32Value{Value: 4096}}

			experiment := gmeasure.NewExperiment("translation comparison")

			experiment.Sample(func(idx int) {
				// Measure how long translation takes using each translator
				experiment.MeasureDuration(fnvName, func() {
					snap, errs, report = fnvTranslator.Translate(params, proxyClone)
				})

				// Assert correctness on the first pass
				if idx == 0 {
					Expect(errs.Validate()).NotTo(HaveOccurred())
					Expect(snap).NotTo(BeNil())
					Expect(report).To(gloo_matchers.BeEquivalentToDiff(validationutils.MakeReport(proxy)))
				}

				experiment.MeasureDuration(hashstructureName, func() {
					snap, errs, report = hashstructureTranslator.Translate(params, proxyClone)
				})

				// Assert correctness on the first pass
				if idx == 0 {
					Expect(errs.Validate()).NotTo(HaveOccurred())
					Expect(snap).NotTo(BeNil())
					Expect(report).To(gloo_matchers.BeEquivalentToDiff(validationutils.MakeReport(proxy)))
				}
			}, gmeasure.SamplingConfig{N: 15, Duration: 15 * time.Second})

			ranking := gmeasure.RankStats(gmeasure.LowerMedianIsBetter, experiment.GetStats(fnvName), experiment.GetStats(hashstructureName))
			AddReportEntry("translation comparison ranking", ranking)

			Expect(ranking.Winner().MeasurementName).To(Equal(fnvName), "expect the FNV translator to be more efficient")
		})
	})

})

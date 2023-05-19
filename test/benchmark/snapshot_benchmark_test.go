package benchmark_test

import (
	"context"
	"encoding/json"
	"fmt"

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
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	glooutils "github.com/solo-io/gloo/projects/gloo/pkg/utils"
	validationutils "github.com/solo-io/gloo/projects/gloo/pkg/utils/validation"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/protoc-gen-ext/pkg/hasher/hashstructure"
	"github.com/solo-io/solo-kit/pkg/utils/protoutils"
)

var _ = Describe("SnapshotBenchmark", func() {
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

	Measure("it should do something hard efficiently", func(b Benchmarker) {
		const times = 1
		reflectionBased := b.Time(fmt.Sprintf("runtime of %d reflect-based hash calls", times), func() {
			for i := 0; i < times; i++ {
				for _, us := range allUpstreams {
					hashstructure.Hash(us, nil)
				}
			}
		})
		generated := b.Time(fmt.Sprintf("runtime of %d generated hash calls", times), func() {
			for i := 0; i < times; i++ {
				for _, us := range allUpstreams {
					us.Hash(nil)
				}
			}
		})
		// divide by 1e3 to get time in micro seconds instead of nano seconds
		b.RecordValue("Runtime per reflection call in µ seconds", float64(int64(reflectionBased)/times)/1e3)
		b.RecordValue("Runtime per generated call in µ seconds", float64(int64(generated)/times)/1e3)

	}, 10)

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

	Context("Benchmark Gloo Translation", func() {

		var (
			settings                *v1.Settings
			fnvTranslator           translator.Translator
			hashstructureTranslator translator.Translator
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

			fnvTranslator = translator.NewTranslatorWithHasher(glooutils.NewSslConfigTranslator(), settings, pluginRegistry, translator.EnvoyCacheResourcesListToFnvHash)
			hashstructureTranslator = translator.NewTranslatorWithHasher(glooutils.NewSslConfigTranslator(), settings, pluginRegistry, translator.EnvoyCacheResourcesListToFnvHash)

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

		Measure("it should perform translation efficiently", func(b Benchmarker) {
			proxyClone := proto.Clone(proxy).(*v1.Proxy)
			proxyClone.GetListeners()[0].Options = &v1.ListenerOptions{PerConnectionBufferLimitBytes: &wrappers.UInt32Value{Value: 4096}}

			b.Time(fmt.Sprintf("runtime of fnv hash translate"), func() {
				snap, errs, report := fnvTranslator.Translate(params, proxyClone)
				Expect(errs.Validate()).NotTo(HaveOccurred())
				Expect(snap).NotTo(BeNil())
				Expect(report).To(gloo_matchers.BeEquivalentToDiff(validationutils.MakeReport(proxy)))
			})
			b.Time(fmt.Sprintf("runtime of hashstructure translate"), func() {
				snap, errs, report := hashstructureTranslator.Translate(params, proxyClone)
				Expect(errs.Validate()).NotTo(HaveOccurred())
				Expect(snap).NotTo(BeNil())
				Expect(report).To(gloo_matchers.BeEquivalentToDiff(validationutils.MakeReport(proxy)))
			})
		}, 15)
	})

})

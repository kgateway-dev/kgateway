package kubernetes

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"

	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/solo-io/gloo/pkg/utils/settingsutil"

	"github.com/golang/protobuf/ptypes/wrappers"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options"
	kubeplugin "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/kubernetes"
	rest "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/rest"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/ssl"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/kubernetes/serviceconverter"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/utils/kubeutils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	kubev1 "k8s.io/api/core/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	uc *KubeUpstreamConverter
)

var _ = Describe("UdsConvert", func() {
	BeforeEach(func() {
		uc = DefaultUpstreamConverter()
	})

	It("should truncate long names", func() {
		name := UpstreamName(strings.Repeat("y", 120), "gloo-system", 12)
		Expect(name).To(HaveLen(63))
	})

	It("should handle collisions", func() {
		name := UpstreamName(strings.Repeat("y", 120), "gloo-system", 12)
		name2 := UpstreamName(strings.Repeat("y", 120)+"2", "gloo-system", 12)
		Expect(name).ToNot(Equal(name2))
	})

	Context("h2 upstream", func() {
		It("should not normally create upstream with grpc service spec", func() {
			svc := &kubev1.Service{
				Spec: kubev1.ServiceSpec{},
			}
			svc.Name = "test"
			svc.Namespace = "test-ns"

			port := kubev1.ServicePort{
				Port: 123,
			}
			up := uc.CreateUpstream(context.TODO(), svc, port)
			spec := up.GetKube().GetServiceSpec()
			Expect(spec.GetGrpc()).To(BeNil())
		})

		It("should save discovery metadata to upstream", func() {
			testLabels := make(map[string]string)
			testLabels["foo"] = "bar"
			svc := &kubev1.Service{
				Spec: kubev1.ServiceSpec{},
			}
			svc.Labels = testLabels
			svc.Name = "test"
			svc.Namespace = "test-ns"

			port := kubev1.ServicePort{
				Port: 123,
			}

			up := uc.CreateUpstream(context.TODO(), svc, port)
			Expect(up.GetDiscoveryMetadata().Labels).To(Equal(testLabels))
		})

		DescribeTable("should create upstream with use_http2=true when port name starts with known prefix", func(portname string) {
			svc := &kubev1.Service{
				Spec: kubev1.ServiceSpec{},
			}
			svc.Name = "test"
			svc.Namespace = "test-ns"

			port := kubev1.ServicePort{
				Port: 123,
				Name: portname,
			}
			up := uc.CreateUpstream(context.TODO(), svc, port)
			Expect(up.GetUseHttp2().GetValue()).To(BeTrue())
		},
			Entry("exactly grpc", "grpc"),
			Entry("prefix grpc", "grpc-test"),
			Entry("exactly h2", "h2"),
			Entry("prefix h2", "h2-test"),
			Entry("exactly http2", "http2"),
		)

		Describe("Upstream Config when Annotations Exist", func() {

			It("Should create upstream with use_http2=true when annotation exists", testSetUseHttp2Converter)

			It("General annotation converter should override SetUseHttp2Converter", func() {
				svc := &kubev1.Service{
					Spec: kubev1.ServiceSpec{},
				}
				svc.Annotations = make(map[string]string)
				svc.Annotations[serviceconverter.GlooH2Annotation] = "true"
				svc.Annotations[serviceconverter.GlooAnnotationPrefix] = `{
					"use_http2": false
				}`
				svc.Name = "test"
				svc.Namespace = "test-ns"

				port := kubev1.ServicePort{
					Port: 123,
				}
				up := uc.CreateUpstream(context.TODO(), svc, port)
				Expect(up.GetUseHttp2().GetValue()).To(BeFalse())
			})

			Describe("Should create upstream with SSL Config when annotations exist", testSetSslConfig)

			expectAnnotationsToProduceUpstreamConfig := func(annotations map[string]string, expectedCfg *v1.Upstream) {
				svc := &kubev1.Service{
					Spec: kubev1.ServiceSpec{},
					ObjectMeta: metav1.ObjectMeta{
						Annotations: annotations,
					},
				}
				svc.Name = "test"
				svc.Namespace = "test-ns"

				port := kubev1.ServicePort{
					Port: 123,
				}

				up := uc.CreateUpstream(context.TODO(), svc, port)

				// Unset the values of fields which are written to by other (i.e., not service converter) modules
				var excludedFields = map[string]bool{
					"NamespacedStatuses": true,
					"Metadata":           true,
					"DiscoveryMetadata":  true,
					"UpstreamType":       true,
				}
				for fieldName := range excludedFields {
					currentStruct := reflect.ValueOf(up).Elem()
					field := currentStruct.FieldByName(fieldName)
					field.Set(reflect.Zero(field.Type()))
				}
				upstreamConfigJson, err := json.Marshal(up)
				Expect(err).To(Not(HaveOccurred()))
				expectedCfgJson, err := json.Marshal(expectedCfg)
				Expect(err).To(Not(HaveOccurred()))
				Expect(upstreamConfigJson).To(Equal(expectedCfgJson))
			}

			CreateUpstreamWithSpec := func(uc *KubeUpstreamConverter, ctx context.Context, svc *kubev1.Service, port kubev1.ServicePort, upstream *v1.Upstream) (*v1.Upstream, error) {
				for _, sc := range uc.serviceConverters {
					if err := sc.ConvertService(context.TODO(), svc, port, upstream); err != nil {
						return nil, err
					}
				}

				return upstream, nil
			}

			Describe("Global config Annotation from Gloo settings", func() {

				It("Should have config from settings annotation but override these with service-specified values", func() {

					// In this test we will apply four unique annotation-based upstream configuration values,
					// between the two possible configuration sources:
					// - At service-level, we'll define: use_http2
					// - At the global scope, we'll define: max_concurrent_streams
					// - And at both we also populate GlooSslTlsKeyAnnotation, and initial_stream_window_size
					// All four unique settings are expected to be configured, while those appearing twice are
					// expected to get their value from service-level configuration which takes precedence.

					svc := &kubev1.Service{
						Spec:       kubev1.ServiceSpec{},
						ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "test-ns"},
					}
					svc.Annotations = make(map[string]string)
					svc.Annotations[serviceconverter.GlooH2Annotation] = "true"
					svc.Annotations[serviceconverter.GlooAnnotationPrefix] = `{"use_http2": false, "initial_stream_window_size": 2048}`
					svc.Annotations[serviceconverter.GlooSslTlsKeyAnnotation] = "ServiceTLSKey"

					// Global-level upstream configuration values applied in Gloo Settings.UpstreamOptions
					ctx := settingsutil.WithSettings(
						context.TODO(),
						&v1.Settings{
							UpstreamOptions: &v1.UpstreamOptions{
								GlobalAnnotations: map[string]string{
									serviceconverter.GlooAnnotationPrefix:    "{\"initial_stream_window_size\": 1024, \"max_concurrent_streams\": 64}",
									serviceconverter.GlooSslTlsKeyAnnotation: "GlobalTLSKey",
								},
							},
						})

					port := kubev1.ServicePort{
						Port: 123,
					}
					up := uc.CreateUpstream(ctx, svc, port)
					Expect(up.GetUseHttp2().GetValue()).To(BeFalse())
					Expect(up.GetInitialStreamWindowSize().GetValue()).To(Equal(uint32(2048)))
					Expect(up.GetMaxConcurrentStreams().GetValue()).To(Equal(uint32(64)))
					Expect(up.GetSslConfig().GetSslFiles().GetTlsKey()).To(BeEquivalentTo("ServiceTLSKey"))
				})
			})

			Describe("deep merge", func() {
				var (
					svc      *kubev1.Service
					port     kubev1.ServicePort
					meta     metav1.ObjectMeta
					coremeta *core.Metadata
					labels   map[string]string
				)

				BeforeEach(func() {
					svc = &kubev1.Service{
						Spec: kubev1.ServiceSpec{},
					}
					svc.Name = "test"
					svc.Namespace = "test-ns"

					port = kubev1.ServicePort{
						Port: 123,
					}

					meta = svc.ObjectMeta
					coremeta = kubeutils.FromKubeMeta(meta, false)
					coremeta.ResourceVersion = ""
					coremeta.Name = UpstreamName(meta.Namespace, meta.Name, port.Port)
					labels = coremeta.GetLabels()
					coremeta.Labels = make(map[string]string)
				})

				It("Should not deep merge upstream fields when merge is not enabled", func() {
					annotations := map[string]string{
						serviceconverter.DeepMergeAnnotationPrefix: "false",
						serviceconverter.GlooAnnotationPrefix: `{
							"sslConfig": {
								"sslFiles": {
									"tlsCert": "certB",
									"rootCa":  "ca"
								}
							}
						}`,
					}

					svc.ObjectMeta = metav1.ObjectMeta{
						Annotations: annotations,
					}

					us := &v1.Upstream{
						Metadata: coremeta,
						DiscoveryMetadata: &v1.DiscoveryMetadata{
							Labels: labels,
						},
						SslConfig: &ssl.UpstreamSslConfig{
							SslSecrets: &ssl.UpstreamSslConfig_SslFiles{
								SslFiles: &ssl.SSLFiles{
									TlsCert: "certA",
									TlsKey:  "testKey",
								},
							},
						},
					}

					up, err := CreateUpstreamWithSpec(uc, context.TODO(), svc, port, us)
					Expect(err).To(BeNil())
					actualSslConfig := up.GetSslConfig()
					Expect(actualSslConfig).NotTo(BeNil())

					expectedSslConfig := &ssl.UpstreamSslConfig{
						SslSecrets: &ssl.UpstreamSslConfig_SslFiles{
							SslFiles: &ssl.SSLFiles{
								TlsCert: "certB",
								RootCa:  "ca",
							},
						},
					}
					Expect(actualSslConfig.GetSslFiles().GetRootCa()).To(BeEquivalentTo(expectedSslConfig.GetSslFiles().GetRootCa()))
					Expect(actualSslConfig.GetSslFiles().GetTlsCert()).To(BeEquivalentTo(expectedSslConfig.GetSslFiles().GetTlsCert()))
					Expect(actualSslConfig.GetSslFiles().GetTlsKey()).To(BeEquivalentTo(expectedSslConfig.GetSslFiles().GetTlsKey()))
				})
				It("should merge upstream fields properly with merge enabled", func() {
					annotations := map[string]string{
						serviceconverter.DeepMergeAnnotationPrefix: "true",
						serviceconverter.GlooAnnotationPrefix: `{
							"sslConfig": {
								"sslFiles": {
									"tlsCert": "certB",
									"rootCa":  "ca"
								}
							}
						}`,
					}

					svc.ObjectMeta = metav1.ObjectMeta{
						Annotations: annotations,
					}

					us := &v1.Upstream{
						Metadata: coremeta,
						UpstreamType: &v1.Upstream_Kube{
							Kube: &kubeplugin.UpstreamSpec{
								ServiceName:      meta.Name,
								ServiceNamespace: meta.Namespace,
								ServicePort:      uint32(port.Port),
								Selector:         svc.Spec.Selector,
							},
						},
						DiscoveryMetadata: &v1.DiscoveryMetadata{
							Labels: labels,
						},
						SslConfig: &ssl.UpstreamSslConfig{
							SslSecrets: &ssl.UpstreamSslConfig_SslFiles{
								SslFiles: &ssl.SSLFiles{
									TlsCert: "certA",
									TlsKey:  "testKey",
								},
							},
						},
					}

					up, err := CreateUpstreamWithSpec(uc, context.TODO(), svc, port, us)
					Expect(err).To(BeNil())
					actualSslConfig := up.GetSslConfig()
					Expect(actualSslConfig).NotTo(BeNil())

					expectedSslConfig := &ssl.UpstreamSslConfig{
						SslSecrets: &ssl.UpstreamSslConfig_SslFiles{
							SslFiles: &ssl.SSLFiles{
								TlsCert: "certB",
								TlsKey:  "testKey",
								RootCa:  "ca",
							},
						},
					}
					Expect(actualSslConfig.GetSslFiles().GetRootCa()).To(BeEquivalentTo(expectedSslConfig.GetSslFiles().GetRootCa()))
					Expect(actualSslConfig.GetSslFiles().GetTlsCert()).To(BeEquivalentTo(expectedSslConfig.GetSslFiles().GetTlsCert()))
					Expect(actualSslConfig.GetSslFiles().GetTlsKey()).To(BeEquivalentTo(expectedSslConfig.GetSslFiles().GetTlsKey()))
				})

				It("should only merge explicitly set upstream fields properly with deep merge enabled", func() {
					annotations := map[string]string{
						serviceconverter.DeepMergeAnnotationPrefix: "true",
						serviceconverter.GlooAnnotationPrefix: `{
							"kube": {
								"serviceSpec": {
									"rest": {
										"swaggerInfo": {
										  "url":"http://newexample.com"
										}
									  }
								}
							}
						}`,
					}

					svc.ObjectMeta = metav1.ObjectMeta{
						Annotations: annotations,
					}

					us := &v1.Upstream{
						Metadata: coremeta,
						UpstreamType: &v1.Upstream_Kube{
							Kube: &kubeplugin.UpstreamSpec{
								ServiceName:      meta.Name,
								ServiceNamespace: meta.Namespace,
								ServicePort:      uint32(port.Port),
								Selector:         svc.Spec.Selector,
								ServiceSpec: &options.ServiceSpec{
									PluginType: &options.ServiceSpec_Rest{
										Rest: &rest.ServiceSpec{
											SwaggerInfo: &rest.ServiceSpec_SwaggerInfo{
												SwaggerSpec: &rest.ServiceSpec_SwaggerInfo_Url{
													Url: "http://example.com",
												},
											},
										},
									},
								},
							},
						},
						DiscoveryMetadata: &v1.DiscoveryMetadata{
							Labels: labels,
						},
					}

					up, err := CreateUpstreamWithSpec(uc, context.TODO(), svc, port, us)
					Expect(err).To(BeNil())
					actualServiceSpec := up.GetKube()
					Expect(actualServiceSpec).NotTo(BeNil())

					expectedServiceSpec := &kubeplugin.UpstreamSpec{
						ServiceName:      meta.Name,
						ServiceNamespace: meta.Namespace,
						ServicePort:      uint32(port.Port),
						Selector:         svc.Spec.Selector,
						ServiceSpec: &options.ServiceSpec{
							PluginType: &options.ServiceSpec_Rest{
								Rest: &rest.ServiceSpec{
									SwaggerInfo: &rest.ServiceSpec_SwaggerInfo{
										SwaggerSpec: &rest.ServiceSpec_SwaggerInfo_Url{
											Url: "http://newexample.com",
										},
									},
								},
							},
						},
					}
					Expect(actualServiceSpec.GetServiceName()).To(BeEquivalentTo(expectedServiceSpec.GetServiceName()))
					Expect(actualServiceSpec.GetServiceNamespace()).To(BeEquivalentTo(expectedServiceSpec.GetServiceNamespace()))
					Expect(actualServiceSpec.GetServicePort()).To(BeEquivalentTo(expectedServiceSpec.GetServicePort()))
					Expect(actualServiceSpec.GetSelector()).To(BeEquivalentTo(expectedServiceSpec.GetSelector()))
					Expect(actualServiceSpec.GetServiceSpec().GetRest().GetSwaggerInfo().GetUrl()).To(BeEquivalentTo(expectedServiceSpec.GetServiceSpec().GetRest().GetSwaggerInfo().GetUrl()))
				})
			})

			DescribeTable("should create upstream with appropriate config when snake_case annotations are present", expectAnnotationsToProduceUpstreamConfig,
				Entry("Using SetHttp2Converter", map[string]string{
					serviceconverter.GlooAnnotationPrefix: `{
						"use_http2": true
					}`,
				}, &v1.Upstream{
					UseHttp2: &wrappers.BoolValue{
						Value: true,
					},
				}),
				Entry("using ssl secret", map[string]string{
					serviceconverter.GlooAnnotationPrefix: `{
						"ssl_config": {
							"secret_ref": {
								"name": "mysecret",
								"namespace": "test"
							}
						}
					}`,
				}, &v1.Upstream{
					SslConfig: &ssl.UpstreamSslConfig{
						SslSecrets: &ssl.UpstreamSslConfig_SecretRef{
							SecretRef: &core.ResourceRef{Name: "mysecret", Namespace: "test"},
						},
					},
				}),
				Entry("using ssl files", map[string]string{
					serviceconverter.GlooAnnotationPrefix: `{
						"ssl_config": {
							"ssl_files": {
								"tls_cert": "cert",
								"tls_key": "key",
								"root_ca": "ca"
							}
						}
					}`,
				}, &v1.Upstream{
					SslConfig: &ssl.UpstreamSslConfig{
						SslSecrets: &ssl.UpstreamSslConfig_SslFiles{
							SslFiles: &ssl.SSLFiles{
								TlsCert: "cert",
								TlsKey:  "key",
								RootCa:  "ca",
							},
						},
					},
				}),
				Entry("Using InitialStreamWindowSize", map[string]string{
					serviceconverter.GlooAnnotationPrefix: `{
						"initial_stream_window_size": 2048
					}`,
				}, &v1.Upstream{
					InitialStreamWindowSize: &wrappers.UInt32Value{
						Value: 2048,
					},
				}),
				Entry("Using HttpProxyHostname", map[string]string{
					serviceconverter.GlooAnnotationPrefix: `{
						"http_proxy_hostname": "test"
					}`,
				}, &v1.Upstream{
					HttpProxyHostname: &wrappers.StringValue{
						Value: "test",
					},
				}),
				Entry("Using IgnoreHealthOnHostRemoval", map[string]string{
					serviceconverter.GlooAnnotationPrefix: `{
						"ignore_health_on_host_removal": true
					}`,
				}, &v1.Upstream{
					IgnoreHealthOnHostRemoval: &wrappers.BoolValue{
						Value: true,
					},
				}),
				Entry("Using RespectDnsTtl", map[string]string{
					serviceconverter.GlooAnnotationPrefix: `{
						"respect_dns_ttl": true
					}`,
				}, &v1.Upstream{
					RespectDnsTtl: &wrappers.BoolValue{
						Value: true,
					},
				}),
				Entry("Using DnsRefreshRate", map[string]string{
					serviceconverter.GlooAnnotationPrefix: `{
						"dns_refresh_rate": "10s"
					}`,
				}, &v1.Upstream{
					DnsRefreshRate: &durationpb.Duration{
						Seconds: 10,
					},
				}),
				Entry("Using CircuitBreakers", map[string]string{
					serviceconverter.GlooAnnotationPrefix: `{
						"circuit_breakers": {
							"max_connections": 2048,
							"max_pending_requests": 2048,
							"max_requests": 2048,
							"max_retries": 2048
						}
					}`,
				}, &v1.Upstream{
					CircuitBreakers: &v1.CircuitBreakerConfig{
						MaxConnections: &wrappers.UInt32Value{
							Value: 2048,
						},
						MaxPendingRequests: &wrappers.UInt32Value{
							Value: 2048,
						},
						MaxRequests: &wrappers.UInt32Value{
							Value: 2048,
						},
						MaxRetries: &wrappers.UInt32Value{
							Value: 2048,
						},
					},
				}),
			)

			DescribeTable("should create upstream with appropriate config when camelCase annotations are present", expectAnnotationsToProduceUpstreamConfig,
				Entry("Using SetHttp2Converter", map[string]string{
					serviceconverter.GlooAnnotationPrefix: `{
						"useHttp2": true
					}`,
				}, &v1.Upstream{
					UseHttp2: &wrappers.BoolValue{
						Value: true,
					},
				}),
				Entry("using ssl secret", map[string]string{
					serviceconverter.GlooAnnotationPrefix: `{
						"sslConfig": {
							"secretRef": {
								"name": "mysecret",
								"namespace": "test"
							}
						}
					}`,
				}, &v1.Upstream{
					SslConfig: &ssl.UpstreamSslConfig{
						SslSecrets: &ssl.UpstreamSslConfig_SecretRef{
							SecretRef: &core.ResourceRef{Name: "mysecret", Namespace: "test"},
						},
					},
				}),
				Entry("using ssl files", map[string]string{
					serviceconverter.GlooAnnotationPrefix: `{
						"sslConfig": {
							"sslFiles": {
								"tlsCert": "cert",
								"tlsKey": "key",
								"rootCa": "ca"
							}
						}
					}`,
				}, &v1.Upstream{
					SslConfig: &ssl.UpstreamSslConfig{
						SslSecrets: &ssl.UpstreamSslConfig_SslFiles{
							SslFiles: &ssl.SSLFiles{
								TlsCert: "cert",
								TlsKey:  "key",
								RootCa:  "ca",
							},
						},
					},
				}),
				Entry("Using InitialStreamWindowSize", map[string]string{
					serviceconverter.GlooAnnotationPrefix: `{
						"initialStreamWindowSize": 4096
					}`,
				}, &v1.Upstream{
					InitialStreamWindowSize: &wrappers.UInt32Value{
						Value: 4096,
					},
				}),
				Entry("Using HttpProxyHostname", map[string]string{
					serviceconverter.GlooAnnotationPrefix: `{
						"httpProxyHostname": "test"
					}`,
				}, &v1.Upstream{
					HttpProxyHostname: &wrappers.StringValue{
						Value: "test",
					},
				}),
				Entry("Using IgnoreHealthOnHostRemoval", map[string]string{
					serviceconverter.GlooAnnotationPrefix: `{
						"ignoreHealthOnHostRemoval": true
					}`,
				}, &v1.Upstream{
					IgnoreHealthOnHostRemoval: &wrappers.BoolValue{
						Value: true,
					},
				}),
				Entry("Using CircuitBreakers", map[string]string{
					serviceconverter.GlooAnnotationPrefix: `{
						"circuitBreakers": {
							"maxConnections": 2048,
							"maxPendingRequests": 2048,
							"maxRequests": 2048,
							"maxRetries": 2048
						}
					}`,
				}, &v1.Upstream{
					CircuitBreakers: &v1.CircuitBreakerConfig{
						MaxConnections: &wrappers.UInt32Value{
							Value: 2048,
						},
						MaxPendingRequests: &wrappers.UInt32Value{
							Value: 2048,
						},
						MaxRequests: &wrappers.UInt32Value{
							Value: 2048,
						},
						MaxRetries: &wrappers.UInt32Value{
							Value: 2048,
						},
					},
				}),
			)
		})
	})
})

func testSetUseHttp2Converter() {
	svc := &kubev1.Service{
		Spec: kubev1.ServiceSpec{},
	}
	svc.Annotations = make(map[string]string)
	svc.Annotations[serviceconverter.GlooH2Annotation] = "true"
	svc.Name = "test"
	svc.Namespace = "test-ns"

	port := kubev1.ServicePort{
		Port: 123,
	}
	up := uc.CreateUpstream(context.TODO(), svc, port)
	Expect(up.GetUseHttp2().GetValue()).To(BeTrue())
}

func testSetSslConfig() {
	DescribeTable("should create upstream with upstreamSslConfig when SSL annotations are present", func(annotations map[string]string, expectedCfg *ssl.UpstreamSslConfig) {
		svc := &kubev1.Service{
			Spec: kubev1.ServiceSpec{},
			ObjectMeta: metav1.ObjectMeta{
				Annotations: annotations,
			},
		}
		svc.Name = "test"
		svc.Namespace = "test-ns"

		port := kubev1.ServicePort{
			Port: 123,
		}

		up := uc.CreateUpstream(context.TODO(), svc, port)
		upstreamSslConfig := up.GetSslConfig()
		upstreamSslConfigJson, err := json.Marshal(upstreamSslConfig)
		Expect(err).To(Not(HaveOccurred()))
		expectedCfgJson, err := json.Marshal(expectedCfg)
		Expect(err).To(Not(HaveOccurred()))
		Expect(upstreamSslConfigJson).To(Equal(expectedCfgJson))
	},
		Entry("using ssl secret", map[string]string{
			serviceconverter.GlooSslSecretAnnotation: "mysecret",
		}, &ssl.UpstreamSslConfig{
			SslSecrets: &ssl.UpstreamSslConfig_SecretRef{
				SecretRef: &core.ResourceRef{Name: "mysecret", Namespace: "test-ns"},
			},
		}),
		Entry("using ssl secret on the target port", map[string]string{
			serviceconverter.GlooSslSecretAnnotation: "123:mysecret",
		}, &ssl.UpstreamSslConfig{
			SslSecrets: &ssl.UpstreamSslConfig_SecretRef{
				SecretRef: &core.ResourceRef{Name: "mysecret", Namespace: "test-ns"},
			},
		}),
		Entry("using ssl secret on a different target port", map[string]string{
			serviceconverter.GlooSslSecretAnnotation: "456:mysecret",
		}, nil),
		Entry("using ssl files", map[string]string{
			serviceconverter.GlooSslTlsCertAnnotation: "cert",
			serviceconverter.GlooSslTlsKeyAnnotation:  "key",
			serviceconverter.GlooSslRootCaAnnotation:  "ca",
		}, &ssl.UpstreamSslConfig{
			SslSecrets: &ssl.UpstreamSslConfig_SslFiles{
				SslFiles: &ssl.SSLFiles{
					TlsCert: "cert",
					TlsKey:  "key",
					RootCa:  "ca",
				},
			},
		}),
		Entry("using ssl files on the target port", map[string]string{
			serviceconverter.GlooSslTlsCertAnnotation: "123:cert",
			serviceconverter.GlooSslTlsKeyAnnotation:  "123:key",
			serviceconverter.GlooSslRootCaAnnotation:  "123:ca",
		}, &ssl.UpstreamSslConfig{
			SslSecrets: &ssl.UpstreamSslConfig_SslFiles{
				SslFiles: &ssl.SSLFiles{
					TlsCert: "cert",
					TlsKey:  "key",
					RootCa:  "ca",
				},
			},
		}),
		Entry("using ssl files on a different target port", map[string]string{
			serviceconverter.GlooSslTlsCertAnnotation: "456:cert",
			serviceconverter.GlooSslTlsKeyAnnotation:  "456:key",
			serviceconverter.GlooSslRootCaAnnotation:  "456:ca",
		}, nil),
	)
}

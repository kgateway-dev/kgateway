package kubernetes

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"

	"github.com/golang/protobuf/ptypes/wrappers"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/kubernetes/serviceconverter"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	kubev1 "k8s.io/api/core/v1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
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
			svc.Namespace = "test"

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
			svc.Namespace = "test"

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
			svc.Namespace = "test"

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
				svc.Namespace = "test"

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
				svc.Namespace = "test"

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
					SslConfig: &v1.UpstreamSslConfig{
						SslSecrets: &v1.UpstreamSslConfig_SecretRef{
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
					SslConfig: &v1.UpstreamSslConfig{
						SslSecrets: &v1.UpstreamSslConfig_SslFiles{
							SslFiles: &v1.SSLFiles{
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
					SslConfig: &v1.UpstreamSslConfig{
						SslSecrets: &v1.UpstreamSslConfig_SecretRef{
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
					SslConfig: &v1.UpstreamSslConfig{
						SslSecrets: &v1.UpstreamSslConfig_SslFiles{
							SslFiles: &v1.SSLFiles{
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
	svc.Namespace = "test"

	port := kubev1.ServicePort{
		Port: 123,
	}
	up := uc.CreateUpstream(context.TODO(), svc, port)
	Expect(up.GetUseHttp2().GetValue()).To(BeTrue())
}

func testSetSslConfig() {
	DescribeTable("should create upstream with upstreamSslConfig when SSL annotations are present", func(annotations map[string]string, expectedCfg *v1.UpstreamSslConfig) {
		svc := &kubev1.Service{
			Spec: kubev1.ServiceSpec{},
			ObjectMeta: metav1.ObjectMeta{
				Annotations: annotations,
			},
		}
		svc.Name = "test"
		svc.Namespace = "test"

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
		}, &v1.UpstreamSslConfig{
			SslSecrets: &v1.UpstreamSslConfig_SecretRef{
				SecretRef: &core.ResourceRef{Name: "mysecret", Namespace: "test"},
			},
		}),
		Entry("using ssl secret on the target port", map[string]string{
			serviceconverter.GlooSslSecretAnnotation: "123:mysecret",
		}, &v1.UpstreamSslConfig{
			SslSecrets: &v1.UpstreamSslConfig_SecretRef{
				SecretRef: &core.ResourceRef{Name: "mysecret", Namespace: "test"},
			},
		}),
		Entry("using ssl secret on a different target port", map[string]string{
			serviceconverter.GlooSslSecretAnnotation: "456:mysecret",
		}, nil),
		Entry("using ssl files", map[string]string{
			serviceconverter.GlooSslTlsCertAnnotation: "cert",
			serviceconverter.GlooSslTlsKeyAnnotation:  "key",
			serviceconverter.GlooSslRootCaAnnotation:  "ca",
		}, &v1.UpstreamSslConfig{
			SslSecrets: &v1.UpstreamSslConfig_SslFiles{
				SslFiles: &v1.SSLFiles{
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
		}, &v1.UpstreamSslConfig{
			SslSecrets: &v1.UpstreamSslConfig_SslFiles{
				SslFiles: &v1.SSLFiles{
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

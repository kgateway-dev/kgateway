package kubernetes

import (
	"context"
	"strings"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/kubernetes/serviceconverter"

	kubev1 "k8s.io/api/core/v1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
)

var _ = Describe("UdsConvert", func() {
	createUpstream := DefaultUpstreamConverter().CreateUpstream

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
			up := createUpstream(context.TODO(), svc, port)
			spec := up.GetKube().GetServiceSpec()
			Expect(spec.GetGrpc()).To(BeNil())
		})

		It("should create upstream with use_http2=true when annotation exists", func() {
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
			up := createUpstream(context.TODO(), svc, port)
			Expect(up.GetUseHttp2().GetValue()).To(BeTrue())
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

			up := createUpstream(context.TODO(), svc, port)
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
			up := createUpstream(context.TODO(), svc, port)
			Expect(up.GetUseHttp2().GetValue()).To(BeTrue())
		},
			Entry("exactly grpc", "grpc"),
			Entry("prefix grpc", "grpc-test"),
			Entry("exactly h2", "h2"),
			Entry("prefix h2", "h2-test"),
			Entry("exactly http2", "http2"),
		)

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

			up := createUpstream(context.TODO(), svc, port)
			Expect(up.GetSslConfig()).To(Equal(expectedCfg))
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
	})
})

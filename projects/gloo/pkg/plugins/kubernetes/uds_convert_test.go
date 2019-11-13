package kubernetes

import (
	"context"
	"crypto/md5"
	"fmt"
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

func UpstreamNameOld(serviceNamespace, serviceName string, servicePort int32, extraLabels map[string]string) string {
	const maxLen = 63

	var labelsTag string
	if len(extraLabels) > 0 {
		_, values := keysAndValues(extraLabels)
		labelsTag = fmt.Sprintf("-%v", strings.Join(values, "-"))
	}
	name := fmt.Sprintf("%s-%s%s-%v", serviceNamespace, serviceName, labelsTag, servicePort)
	if len(name) > maxLen {
		hash := md5.Sum([]byte(name))
		hexhash := fmt.Sprintf("%x", hash)
		name = name[:maxLen-len(hexhash)] + hexhash
	}
	name = strings.Replace(name, ".", "-", -1)
	return name
}

var _ = Describe("UdsConvert", func() {
	createUpstream := DefaultUpstreamConverter().CreateUpstream

	It("should get uniq label set", func() {

		svcSelector := map[string]string{"app": "foo"}
		podmetas := []map[string]string{
			{"app": "foo", "env": "prod"},
			{"app": "foo", "env": "prod"},
			{"app": "foo", "env": "dev"},
		}
		result := GetUniqueLabelSetsForObjects(svcSelector, podmetas)
		expected := []map[string]string{
			{"app": "foo"},
			{"app": "foo", "env": "prod"},
			{"app": "foo", "env": "dev"},
		}
		Expect(result).To(Equal(expected))

	})

	It("should truncate long names", func() {
		name := UpstreamName(strings.Repeat("y", 120), "gloo-system", 12, nil)
		Expect(name).To(HaveLen(63))
	})
	It("should truncate long names with lot of labels", func() {
		name := UpstreamName("test", "gloo-system", 12, map[string]string{"test": strings.Repeat("y", 120)})
		Expect(len(name)).To(BeNumerically("<=", 63))
	})

	It("should handle collisions", func() {
		name := UpstreamName(strings.Repeat("y", 120), "gloo-system", 12, nil)
		name2 := UpstreamName(strings.Repeat("y", 120)+"2", "gloo-system", 12, nil)
		Expect(name).ToNot(Equal(name2))
	})

	It("should sanitize the same way", func() {
		name := UpstreamNameOld("ns", "gloo-system", 12, map[string]string{"test": "label"})
		name2 := UpstreamName("ns", "gloo-system", 12, map[string]string{"test": "label"})
		Expect(name).To(Equal(name2))
	})

	It("should ignore ignored labels", func() {

		svcSelector := map[string]string{"app": "foo"}
		podmetas := []map[string]string{
			{"app": "foo", "env": "prod", "release": "first"},
		}
		result := GetUniqueLabelSetsForObjects(svcSelector, podmetas)
		expected := []map[string]string{
			{"app": "foo"},
			{"app": "foo", "env": "prod"},
		}
		Expect(result).To(Equal(expected))
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
			up := createUpstream(context.TODO(), svc, port, map[string]string{"a": "b"})
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
			up := createUpstream(context.TODO(), svc, port, map[string]string{"a": "b"})
			Expect(up.GetUseHttp2()).To(BeTrue())
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
			up := createUpstream(context.TODO(), svc, port, map[string]string{"a": "b"})
			Expect(up.GetUseHttp2()).To(BeTrue())
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

			up := createUpstream(context.TODO(), svc, port, nil)
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

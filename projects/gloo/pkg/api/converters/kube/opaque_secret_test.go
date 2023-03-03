package kubeconverters_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	kubeconverters "github.com/solo-io/gloo/projects/gloo/pkg/api/converters/kube"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kubesecret"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	. "github.com/solo-io/solo-kit/test/matchers"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

var _ = Describe("Opaque Secret Converter", func() {

	var (
		ctx            context.Context
		converter      kubesecret.SecretConverter
		resourceClient *kubesecret.ResourceClient
	)

	BeforeEach(func() {
		ctx = context.TODO()
		converter = &kubeconverters.OpaqueSecretConverter{}

		clientset := fake.NewSimpleClientset()
		coreCache, err := cache.NewKubeCoreCache(ctx, clientset)
		Expect(err).NotTo(HaveOccurred())
		resourceClient, err = kubesecret.NewResourceClient(clientset, &v1.Secret{}, false, coreCache)
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("converting from a Kubernetes secret to a Gloo secret", func() {

		It("ignores secrets that aren't opaque secrets", func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
				},
				Data: map[string][]byte{
					"tls": []byte(tlsValue),
				},
				Type: corev1.SecretTypeTLS,
			}
			glooSecret, err := converter.FromKubeSecret(ctx, resourceClient, secret)
			Expect(err).NotTo(HaveOccurred())
			Expect(glooSecret).To(BeNil())
		})

		It("ignores opaque secrets without resource_kind annotation", func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
				},
				Data: map[string][]byte{
					"tls": []byte(tlsValue),
				},
				Type: corev1.SecretTypeOpaque,
			}
			glooSecret, err := converter.FromKubeSecret(ctx, resourceClient, secret)
			Expect(err).NotTo(HaveOccurred())
			Expect(glooSecret).To(BeNil())
		})

		It("correctly converts an opaque valid TLS secret to a gloo TLS secret", func() {
			// Before we had the TLS secret converter, glooctl would create tls secrets as
			// Opaque, with all the values stuffed into the `tls` field. This test ensures
			// that we can still convert the old format opaque secret into a gloo TLS secret
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "foo",
					Namespace:   "bar",
					Annotations: map[string]string{kubeconverters.GlooKindAnnotationKey: "*v1.Secret"},
				},
				Data: map[string][]byte{
					"tls": []byte(tlsValue),
				},
				Type: corev1.SecretTypeOpaque,
			}
			actual, err := converter.FromKubeSecret(ctx, resourceClient, secret)
			Expect(err).NotTo(HaveOccurred())
			Expect(actual).To(MatchProto(&v1.Secret{
				Metadata: &core.Metadata{
					Name:        "foo",
					Namespace:   "bar",
					Annotations: map[string]string{kubeconverters.GlooKindAnnotationKey: "*v1.Secret"},
				},
				Kind: &v1.Secret_Tls{
					Tls: &v1.TlsSecret{
						CertChain:  certChain,
						PrivateKey: privateKey,
						RootCa:     rootCa,
					},
				},
			}))
		})

		It("does not convert a secret with invalid tls data", func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "foo",
					Namespace:   "bar",
					Annotations: map[string]string{kubeconverters.GlooKindAnnotationKey: "*v1.Secret"},
				},
				Data: map[string][]byte{
					"tls": []byte("just some stuff"),
				},
				Type: corev1.SecretTypeOpaque,
			}
			actual, err := converter.FromKubeSecret(ctx, resourceClient, secret)
			Expect(err).NotTo(HaveOccurred())
			Expect(actual).To(BeNil())
		})
	})

	Describe("converting from a Gloo secret to a Kubernetes one", func() {
		It("does not do any conversion", func() {
			actual, err := converter.ToKubeSecret(ctx, resourceClient, &v1.Secret{
				Metadata: &core.Metadata{Name: "foo"},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(actual).To(BeNil())
		})
	})

})

// example values copied from projects/gloo/cli/pkg/cmd/create/secret/secret_test.go
var (
	privateKey = `
-----BEGIN PRIVATE KEY-----
MIIEvgIBADANBgkqhkiG9w0BAQEFAASCBKgwggSkAgEAAoIBAQDKtfsD/lq/htGu
28ivzLzzmZe8fOiqBn2eNUj8QVOlHLnaxTZvrqlkTAnPsDvdHCSTQrZDGI5am1/c
tMKodDbflybgskKvZvDi6BF1w1sbZ3kPGZEt0EnykoiKDvc++XuiNUvMQG02B03v
4iurrLa6TLapB8NWtmRJ3n/Pba500zu+uDF6WFkNTEawzuMMl7jLX5gmkDQeBVvX
12IRY0E1zyq7oTNxsyppxvknhAVKdLYsKRVCtwruC0ACjdwYgq47Nn1S7d7b7M8T
cRkcsHqpy4fUvski1ykfX45kt3QcH8XEpCLPA6B74/1YDAGKfdfBKTsZbeuUt5Q3
v8u2/VJDAgMBAAECggEBALVb/0phZLt6WVtCE9kFKgAf6JugfWCxEe5b6gKWR8mv
W7CZRMzCzZaIWTbRi52VMjv2Ma7x51q1LB0ANDAWWYnNZ+EcW4Embln0GrrrnzVz
a+HQlA0TDzXRWAt8vERBXRWQ7VG+SndOLbJy/XNIwOsI+1tbMK8B29QjFuJ0VOL7
CUoG1/0BXw6sloh9UcoqM4nVcNaqocti4K7d3Z0/T1zJzx6o4o89DqBRnzTj0b9O
ge+sRqGFxEejr2hqMJ0jF5BPYUOyt/4kz1gJcRst0NI9dttwTPWWodu2a3eQT1SF
GmYaJpZQAhknQ6LbFMIdGxLUjt9Ej0D/YRjAvjrs5SECgYEA8o5MJoq8gIMbYFuS
UpR9/qhOh1EaR1D6SEKBpStvVXkRR0JHU77h5gmZZNjzeoGzR5rON5iq2kuKNomW
jt4S6BdEPoR+Kqxu/55uVhmdrSmP/tMXN0XTQa63HscmWUsadBm/E1xAaivYG7W9
GyzsVW+HApm6mTmL2S+YQ1aB1esCgYEA1fJPTiFifjAuhPKOLHQZ6WmgdAWNAsDi
nIoM9BSaxZYNo7o2nes2FIa88hIAtlyCzLMNI8r7xkD3XJEsWoe+VX1gbeTjARpm
W6yLMkYvQsF6Urt4Jw1m2+mCbbh/aNfdaL3HrHevU8mMUaP9iRekXYNh24CaNK+v
t2YRuWsCJwkCgYBgP+Mr8CW5AU2dwPihWFde9D6lJ6O75QBMKEf12PSHAFHA6yYO
r1JIzEpYYFbNqCYSJfXqzeQOV6dy2MoryyfJfWIRRNYj7OTm/mFePS/6hOGlBvLR
dh3MlJ4J0pD/IfRPWeAeuJ6/AsLwy/9Mh1kI1gbHG2WWY+WAu4g6QFupHQKBgQCK
ODWMIH1lUPN86Md5aLik15zV2BA1yy+cOoQL3JPxOvQs5s0KUT9rG3FOYtsa9cF7
ReIjUaw/dRFaOGATTMdmq810sf8GY2vlph93p2g5FI5WjM8fS8U8JiwhfqSxs2RT
mug5QEmBNCD3TZ8qxp9l2s+J5Be8GhTHw6WHyN5nIQKBgHseKAYNH0SKMTBmD9tC
+DMhw6Ypxe4VsDBFoDr1Wxpt6SmrZcy7JcBO/jmXY/xwsnGyehdbpsMxX03c7QSf
AmoJCgOtm0FUXc+eybFzgjM9dvB/ZaKRk7LtA2KJjFtMPGwKmLwjC4+cD8xL57Ej
ZEhjfeyucd48M+JNbyMuE2ZC
-----END PRIVATE KEY-----
`
	certChain = `
-----BEGIN CERTIFICATE-----
MIICvDCCAaQCCQDro6ZXybhlYDANBgkqhkiG9w0BAQsFADAgMR4wHAYDVQQDDBVw
ZXRzdG9yZTEuZXhhbXBsZS5jb20wHhcNMTkwNDA1MTkyODQ2WhcNMjAwNDA0MTky
ODQ2WjAgMR4wHAYDVQQDDBVwZXRzdG9yZTEuZXhhbXBsZS5jb20wggEiMA0GCSqG
SIb3DQEBAQUAA4IBDwAwggEKAoIBAQDKtfsD/lq/htGu28ivzLzzmZe8fOiqBn2e
NUj8QVOlHLnaxTZvrqlkTAnPsDvdHCSTQrZDGI5am1/ctMKodDbflybgskKvZvDi
6BF1w1sbZ3kPGZEt0EnykoiKDvc++XuiNUvMQG02B03v4iurrLa6TLapB8NWtmRJ
3n/Pba500zu+uDF6WFkNTEawzuMMl7jLX5gmkDQeBVvX12IRY0E1zyq7oTNxsypp
xvknhAVKdLYsKRVCtwruC0ACjdwYgq47Nn1S7d7b7M8TcRkcsHqpy4fUvski1ykf
X45kt3QcH8XEpCLPA6B74/1YDAGKfdfBKTsZbeuUt5Q3v8u2/VJDAgMBAAEwDQYJ
KoZIhvcNAQELBQADggEBAL8m5TjFEb58MEXKGdbyGZEdS0FpNI+fYBYyxkpU5/z3
06hV2ajisgvHGyGun/HLBDXtWnbNWKpSjiJiS9Kpkv6X73hba6Q3p3prjgdXkpSU
ONozwlMM1SM0dj/O5VULkcW4uhSQJEyIRRPiA8fslqWuIlr5KWWPbdIkDex/9Ddf
oC7D1exclZNVDVmJzYFSxb1js/rSsln11VJ7uyozpk23lrAVGIrtg5Xr4vxqUZHU
TOeFSVH6LMC5j/Fff+bEBhbPxJAI0P7VXaphYh/dMyAEq+xRxm6ssuccgCyvttmz
+6sUivvxaDhUCAzAoLSa5Xgn5eNdsePz6PQ5Vy/Yidg=
-----END CERTIFICATE-----
`
	rootCa = "foo"

	// the above 3 fields combined
	tlsValue = `certChain: |2

  -----BEGIN CERTIFICATE-----
  MIICvDCCAaQCCQDro6ZXybhlYDANBgkqhkiG9w0BAQsFADAgMR4wHAYDVQQDDBVw
  ZXRzdG9yZTEuZXhhbXBsZS5jb20wHhcNMTkwNDA1MTkyODQ2WhcNMjAwNDA0MTky
  ODQ2WjAgMR4wHAYDVQQDDBVwZXRzdG9yZTEuZXhhbXBsZS5jb20wggEiMA0GCSqG
  SIb3DQEBAQUAA4IBDwAwggEKAoIBAQDKtfsD/lq/htGu28ivzLzzmZe8fOiqBn2e
  NUj8QVOlHLnaxTZvrqlkTAnPsDvdHCSTQrZDGI5am1/ctMKodDbflybgskKvZvDi
  6BF1w1sbZ3kPGZEt0EnykoiKDvc++XuiNUvMQG02B03v4iurrLa6TLapB8NWtmRJ
  3n/Pba500zu+uDF6WFkNTEawzuMMl7jLX5gmkDQeBVvX12IRY0E1zyq7oTNxsypp
  xvknhAVKdLYsKRVCtwruC0ACjdwYgq47Nn1S7d7b7M8TcRkcsHqpy4fUvski1ykf
  X45kt3QcH8XEpCLPA6B74/1YDAGKfdfBKTsZbeuUt5Q3v8u2/VJDAgMBAAEwDQYJ
  KoZIhvcNAQELBQADggEBAL8m5TjFEb58MEXKGdbyGZEdS0FpNI+fYBYyxkpU5/z3
  06hV2ajisgvHGyGun/HLBDXtWnbNWKpSjiJiS9Kpkv6X73hba6Q3p3prjgdXkpSU
  ONozwlMM1SM0dj/O5VULkcW4uhSQJEyIRRPiA8fslqWuIlr5KWWPbdIkDex/9Ddf
  oC7D1exclZNVDVmJzYFSxb1js/rSsln11VJ7uyozpk23lrAVGIrtg5Xr4vxqUZHU
  TOeFSVH6LMC5j/Fff+bEBhbPxJAI0P7VXaphYh/dMyAEq+xRxm6ssuccgCyvttmz
  +6sUivvxaDhUCAzAoLSa5Xgn5eNdsePz6PQ5Vy/Yidg=
  -----END CERTIFICATE-----
privateKey: |2

  -----BEGIN PRIVATE KEY-----
  MIIEvgIBADANBgkqhkiG9w0BAQEFAASCBKgwggSkAgEAAoIBAQDKtfsD/lq/htGu
  28ivzLzzmZe8fOiqBn2eNUj8QVOlHLnaxTZvrqlkTAnPsDvdHCSTQrZDGI5am1/c
  tMKodDbflybgskKvZvDi6BF1w1sbZ3kPGZEt0EnykoiKDvc++XuiNUvMQG02B03v
  4iurrLa6TLapB8NWtmRJ3n/Pba500zu+uDF6WFkNTEawzuMMl7jLX5gmkDQeBVvX
  12IRY0E1zyq7oTNxsyppxvknhAVKdLYsKRVCtwruC0ACjdwYgq47Nn1S7d7b7M8T
  cRkcsHqpy4fUvski1ykfX45kt3QcH8XEpCLPA6B74/1YDAGKfdfBKTsZbeuUt5Q3
  v8u2/VJDAgMBAAECggEBALVb/0phZLt6WVtCE9kFKgAf6JugfWCxEe5b6gKWR8mv
  W7CZRMzCzZaIWTbRi52VMjv2Ma7x51q1LB0ANDAWWYnNZ+EcW4Embln0GrrrnzVz
  a+HQlA0TDzXRWAt8vERBXRWQ7VG+SndOLbJy/XNIwOsI+1tbMK8B29QjFuJ0VOL7
  CUoG1/0BXw6sloh9UcoqM4nVcNaqocti4K7d3Z0/T1zJzx6o4o89DqBRnzTj0b9O
  ge+sRqGFxEejr2hqMJ0jF5BPYUOyt/4kz1gJcRst0NI9dttwTPWWodu2a3eQT1SF
  GmYaJpZQAhknQ6LbFMIdGxLUjt9Ej0D/YRjAvjrs5SECgYEA8o5MJoq8gIMbYFuS
  UpR9/qhOh1EaR1D6SEKBpStvVXkRR0JHU77h5gmZZNjzeoGzR5rON5iq2kuKNomW
  jt4S6BdEPoR+Kqxu/55uVhmdrSmP/tMXN0XTQa63HscmWUsadBm/E1xAaivYG7W9
  GyzsVW+HApm6mTmL2S+YQ1aB1esCgYEA1fJPTiFifjAuhPKOLHQZ6WmgdAWNAsDi
  nIoM9BSaxZYNo7o2nes2FIa88hIAtlyCzLMNI8r7xkD3XJEsWoe+VX1gbeTjARpm
  W6yLMkYvQsF6Urt4Jw1m2+mCbbh/aNfdaL3HrHevU8mMUaP9iRekXYNh24CaNK+v
  t2YRuWsCJwkCgYBgP+Mr8CW5AU2dwPihWFde9D6lJ6O75QBMKEf12PSHAFHA6yYO
  r1JIzEpYYFbNqCYSJfXqzeQOV6dy2MoryyfJfWIRRNYj7OTm/mFePS/6hOGlBvLR
  dh3MlJ4J0pD/IfRPWeAeuJ6/AsLwy/9Mh1kI1gbHG2WWY+WAu4g6QFupHQKBgQCK
  ODWMIH1lUPN86Md5aLik15zV2BA1yy+cOoQL3JPxOvQs5s0KUT9rG3FOYtsa9cF7
  ReIjUaw/dRFaOGATTMdmq810sf8GY2vlph93p2g5FI5WjM8fS8U8JiwhfqSxs2RT
  mug5QEmBNCD3TZ8qxp9l2s+J5Be8GhTHw6WHyN5nIQKBgHseKAYNH0SKMTBmD9tC
  +DMhw6Ypxe4VsDBFoDr1Wxpt6SmrZcy7JcBO/jmXY/xwsnGyehdbpsMxX03c7QSf
  AmoJCgOtm0FUXc+eybFzgjM9dvB/ZaKRk7LtA2KJjFtMPGwKmLwjC4+cD8xL57Ej
  ZEhjfeyucd48M+JNbyMuE2ZC
  -----END PRIVATE KEY-----
rootCa: foo
`
)

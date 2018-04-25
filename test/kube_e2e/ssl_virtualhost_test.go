package kube_e2e

import (
	"time"

	"io/ioutil"

	. "github.com/onsi/ginkgo"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/coreplugins/service"
	. "github.com/solo-io/gloo/test/helpers"
	kubev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	sslCertificateChainKey = "ca_chain"
	sslPrivateKeyKey       = "private_key"
)

var (
	sslCertChain  []byte //= []byte(``)
	sslPrivateKey []byte //= []byte(``)
)

var _ = FDescribe("SNI VirtualService", func() {
	const helloService = "helloservice"
	const servicePort = 8080
	Context("creating a vService with an ssl config", func() {
		path := "/ssl-route"
		vServiceName := "ssl-config"
		secretName := "test-secret"
		BeforeEach(func() {
			var err error
			sslCertChain, err = ioutil.ReadFile(ServerCert())
			Must(err)
			sslPrivateKey, err = ioutil.ReadFile(ServerKey())
			Must(err)
			_, err = kube.CoreV1().Secrets(namespace).Create(&kubev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: namespace,
				},
				Data: map[string][]byte{
					sslCertificateChainKey: sslCertChain,  //[]byte(base64.StdEncoding.EncodeToString(sslCertChain)),
					sslPrivateKeyKey:       sslPrivateKey, //[]byte(base64.StdEncoding.EncodeToString(sslPrivateKey)),
				},
			})
			Must(err)
			_, err = gloo.V1().Upstreams().Create(&v1.Upstream{
				Name: helloService,
				Type: service.UpstreamTypeService,
				Spec: service.EncodeUpstreamSpec(service.UpstreamSpec{
					Hosts: []service.Host{
						{
							Addr: helloService,
							Port: servicePort,
						},
					},
				}),
			})
			Must(err)
			_, err = gloo.V1().VirtualServices().Create(&v1.VirtualService{
				Name: vServiceName,
				Routes: []*v1.Route{{
					Matcher: &v1.Route_RequestMatcher{
						RequestMatcher: &v1.RequestMatcher{
							Path: &v1.RequestMatcher_PathExact{
								PathExact: path,
							},
							Verbs: []string{"GET"},
						},
					},
					SingleDestination: &v1.Destination{
						DestinationType: &v1.Destination_Upstream{
							Upstream: &v1.UpstreamDestination{
								Name: helloService,
							},
						},
					},
				}},
				SslConfig: &v1.SSLConfig{
					SecretRef: secretName,
				},
			})
			Must(err)
		})
		AfterEach(func() {
			kube.CoreV1().Secrets(namespace).Delete(secretName, nil)
			gloo.V1().Upstreams().Delete(helloService)
			gloo.V1().VirtualServices().Delete(vServiceName)
		})
		It("should get a 200ok on the ssl port (8443)", func() {
			curlEventuallyShouldRespond(curlOpts{protocol: "https", path: path, port: 8443, caFile: "/root.crt"}, "< HTTP/1.1 200", time.Minute*5)
		})
	})
})

package nomad_e2e

import (
	"time"

	"io/ioutil"

	. "github.com/onsi/ginkgo"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/storage/dependencies"
	. "github.com/solo-io/gloo/test/helpers"
)

const (
	sslCertificateChainKey = "ca_chain"
	sslPrivateKeyKey       = "private_key"
)

var (
	sslCertChain  []byte //= []byte(``)
	sslPrivateKey []byte //= []byte(``)
)

var _ = Describe("SNI Virtualhost", func() {
	const helloService = "helloservice"
	Context("creating a vhost with an ssl config", func() {
		path := "/ssl-route"
		vhostName := "ssl-config"
		secretName := "test-secret"
		BeforeEach(func() {
			var err error
			sslCertChain, err = ioutil.ReadFile(ServerCert())
			Must(err)
			sslPrivateKey, err = ioutil.ReadFile(ServerKey())
			Must(err)
			_, err = secrets.Create(&dependencies.Secret{
				Ref: secretName,
				Data: map[string]string{
					sslCertificateChainKey: string(sslCertChain),
					sslPrivateKeyKey:       string(sslPrivateKey),
				},
			})
			Must(err)
			Must(err)
			_, err = gloo.V1().VirtualHosts().Create(&v1.VirtualHost{
				Name: vhostName,
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
			secrets.Delete(secretName)
			gloo.V1().VirtualHosts().Delete(vhostName)
		})
		It("should get a 200ok on the ssl port (8443)", func() {
			CurlEventuallyShouldRespond(CurlOpts{Host: "test-ingress", Protocol: "https", Path: path, CaFile: ServerCert()}, "< HTTP/1.1 200", time.Second*35)
		})
	})
})

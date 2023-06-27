package run_test

import (
	"context"
	"os"
	"path"
	"time"

	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	envoy_service_secret_v3 "github.com/envoyproxy/go-control-plane/envoy/service/secret/v3"
	"github.com/solo-io/gloo/projects/sds/pkg/run"
	"github.com/solo-io/gloo/projects/sds/pkg/server"
	"github.com/solo-io/gloo/projects/sds/pkg/testutils"
	"github.com/spf13/afero"
	"google.golang.org/grpc"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("SDS Server E2E Test", Serial, func() {

	var (
		ctx                                                             context.Context
		cancel                                                          context.CancelFunc
		err                                                             error
		fs                                                              afero.Fs
		dir                                                             string
		keyName, certName, caName, ocspName                             string
		keyNameSymlink, certNameSymlink, caNameSymlink, ocspNameSymlink string
		secret                                                          server.Secret
		testServerAddress                                               = "127.0.0.1:8236"
		sdsClient                                                       = "test-client"
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())

		fileString := []byte("test")
		fs = afero.NewOsFs()
		dir, err = afero.TempDir(fs, "", "")
		Expect(err).To(BeNil())

		// Kubernetes mounts secrets as a symlink to a ..data directory, so we'll mimic that here
		keyName = path.Join(dir, "/", "tls.key-0")
		certName = path.Join(dir, "/", "tls.crt-0")
		caName = path.Join(dir, "/", "ca.crt-0")
		ocspName = path.Join(dir, "/", "tls.ocsp-staple-0")
		err = afero.WriteFile(fs, keyName, fileString, 0644)
		Expect(err).To(BeNil())
		err = afero.WriteFile(fs, certName, fileString, 0644)
		Expect(err).To(BeNil())
		err = afero.WriteFile(fs, caName, fileString, 0644)
		Expect(err).To(BeNil())
		// This is a pre-generated DER-encoded OCSP response using `openssl` to better match actual ocsp staple/response data.
		// This response isn't for the test certs as they are just random data, but it is a syntactically-valid OCSP response.
		ocspResponse, err := os.ReadFile("certs/ocsp_response.der")
		Expect(err).ToNot(HaveOccurred())
		err = afero.WriteFile(fs, ocspName, ocspResponse, 0644)
		Expect(err).To(BeNil())
		keyNameSymlink = path.Join(dir, "/", "tls.key")
		certNameSymlink = path.Join(dir, "/", "tls.crt")
		caNameSymlink = path.Join(dir, "/", "ca.crt")
		ocspNameSymlink = path.Join(dir, "/", "tls.ocsp-staple")
		err = os.Symlink(keyName, keyNameSymlink)
		Expect(err).To(BeNil())
		err = os.Symlink(certName, certNameSymlink)
		Expect(err).To(BeNil())
		err = os.Symlink(caName, caNameSymlink)
		Expect(err).To(BeNil())
		err = os.Symlink(ocspName, ocspNameSymlink)
		Expect(err).To(BeNil())

		secret = server.Secret{
			ServerCert:        "test-cert",
			ValidationContext: "test-validation-context",
			SslCaFile:         caName,
			SslCertFile:       certName,
			SslKeyFile:        keyName,
			SslOcspFile:       ocspName,
		}
	})

	AfterEach(func() {
		cancel()

		_ = fs.RemoveAll(dir)
	})

	It("runs and stops correctly", func() {

		go func() {
			defer GinkgoRecover()

			if err := run.Run(ctx, []server.Secret{secret}, sdsClient, testServerAddress); err != nil {
				Expect(err).To(BeNil())
			}
		}()

		// Connect with the server
		var conn *grpc.ClientConn
		conn, err := grpc.Dial(testServerAddress, grpc.WithInsecure())
		Expect(err).To(BeNil())
		defer conn.Close()
		client := envoy_service_secret_v3.NewSecretDiscoveryServiceClient(conn)

		// Check that we get a good response
		Eventually(func() bool {
			_, err = client.FetchSecrets(ctx, &envoy_service_discovery_v3.DiscoveryRequest{})
			return err != nil
		}, "5s", "1s").Should(BeTrue())

		// Cancel the context in order to stop the gRPC server
		cancel()

		// The gRPC server should stop eventually
		Eventually(func() bool {
			_, err = client.FetchSecrets(ctx, &envoy_service_discovery_v3.DiscoveryRequest{})
			return err != nil
		}, "5s", "1s").Should(BeTrue())

	})

	DescribeTable("correctly picks up cert rotations", func(useOcsp bool, expectedHashes []string) {
		go func() {
			defer GinkgoRecover()
			ocsp := ""
			if useOcsp {
				ocsp = ocspName
			}
			secret.SslOcspFile = ocsp
			_ = run.Run(ctx, []server.Secret{secret}, sdsClient, testServerAddress)
		}()

		// Give it a second to spin up + read the files
		time.Sleep(1 * time.Second)

		// Connect with the server
		var conn *grpc.ClientConn
		conn, err = grpc.Dial(testServerAddress, grpc.WithInsecure())
		Expect(err).To(BeNil())
		defer conn.Close()
		client := envoy_service_secret_v3.NewSecretDiscoveryServiceClient(conn)

		// Read certs
		var certs [][]byte
		if useOcsp {
			certs, err = testutils.FilesToBytes(keyNameSymlink, certNameSymlink, caNameSymlink, ocspNameSymlink)
		} else {
			certs, err = testutils.FilesToBytes(keyNameSymlink, certNameSymlink, caNameSymlink)
		}
		Expect(err).NotTo(HaveOccurred())

		snapshotVersion, err := server.GetSnapshotVersion(certs)
		Expect(err).To(BeNil())
		Expect(snapshotVersion).To(Equal(expectedHashes[0]))

		var resp *envoy_service_discovery_v3.DiscoveryResponse

		Eventually(func() bool {
			resp, err = client.FetchSecrets(ctx, &envoy_service_discovery_v3.DiscoveryRequest{})
			return err == nil
		}, "15s", "1s").Should(BeTrue())

		Eventually(func() bool {
			resp, err = client.FetchSecrets(ctx, &envoy_service_discovery_v3.DiscoveryRequest{})
			return resp.VersionInfo == snapshotVersion
		}, "15s", "1s").Should(BeTrue())

		// Cert rotation #1
		err = os.Remove(keyName)
		Expect(err).To(BeNil())
		err = afero.WriteFile(fs, keyName, []byte("tls.key-1"), 0644)
		Expect(err).To(BeNil())

		// Re-read certs
		certs, err = testutils.FilesToBytes(keyNameSymlink, certNameSymlink, caNameSymlink)
		Expect(err).NotTo(HaveOccurred())
		if useOcsp {
			ocspBytes, err := os.ReadFile(ocspNameSymlink)
			Expect(err).ToNot(HaveOccurred())
			certs = append(certs, ocspBytes)
		}

		snapshotVersion, err = server.GetSnapshotVersion(certs)
		Expect(err).To(BeNil())
		Expect(snapshotVersion).To(Equal(expectedHashes[1]))
		Eventually(func() bool {
			resp, err = client.FetchSecrets(ctx, &envoy_service_discovery_v3.DiscoveryRequest{})
			Expect(err).To(BeNil())
			return resp.VersionInfo == snapshotVersion
		}, "15s", "1s").Should(BeTrue())

		// Cert rotation #2
		err = os.Remove(keyName)
		Expect(err).To(BeNil())
		err = afero.WriteFile(fs, keyName, []byte("tls.key-2"), 0644)
		Expect(err).To(BeNil())

		// Re-read certs again
		certs, err = testutils.FilesToBytes(keyNameSymlink, certNameSymlink, caNameSymlink)
		Expect(err).NotTo(HaveOccurred())
		if useOcsp {
			ocspBytes, err := os.ReadFile(ocspNameSymlink)
			Expect(err).ToNot(HaveOccurred())
			certs = append(certs, ocspBytes)
		}

		snapshotVersion, err = server.GetSnapshotVersion(certs)
		Expect(err).To(BeNil())
		Expect(snapshotVersion).To(Equal(expectedHashes[2]))
		Eventually(func() bool {
			resp, err = client.FetchSecrets(ctx, &envoy_service_discovery_v3.DiscoveryRequest{})
			Expect(err).To(BeNil())
			return resp.VersionInfo == snapshotVersion
		}, "15s", "1s").Should(BeTrue())
	},
		Entry("with ocsp", true, []string{"969835737182439215", "6265739243366543658", "14893951670674740726"}),
		Entry("without ocsp", false, []string{"6730780456972595554", "16241649556325798095", "7644406922477208950"}),
	)
})

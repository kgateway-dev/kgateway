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

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("SDS Server E2E Test", func() {

	var (
		err                                            error
		fs                                             afero.Fs
		dir                                            string
		keyName, certName, caName                      string
		keyNameSymlink, certNameSymlink, caNameSymlink string
		secret                                         server.Secret
		testServerAddress                              = "127.0.0.1:8236"
		sdsClient                                      = "test-client"
	)

	BeforeEach(func() {
		fileString := []byte("test")
		fs = afero.NewOsFs()
		dir, err = afero.TempDir(fs, "", "")
		Expect(err).To(BeNil())

		// Kubernetes mounts secrets as a symlink to a ..data directory, so we'll mimic that here
		keyName = path.Join(dir, "/", "tls.key-0")
		certName = path.Join(dir, "/", "tls.crt-0")
		caName = path.Join(dir, "/", "ca.crt-0")
		err = afero.WriteFile(fs, keyName, fileString, 0644)
		Expect(err).To(BeNil())
		err = afero.WriteFile(fs, certName, fileString, 0644)
		Expect(err).To(BeNil())
		err = afero.WriteFile(fs, caName, fileString, 0644)
		Expect(err).To(BeNil())
		keyNameSymlink = path.Join(dir, "/", "tls.key")
		certNameSymlink = path.Join(dir, "/", "tls.crt")
		caNameSymlink = path.Join(dir, "/", "ca.crt")
		err := os.Symlink(keyName, keyNameSymlink)
		Expect(err).To(BeNil())
		err = os.Symlink(certName, certNameSymlink)
		Expect(err).To(BeNil())
		err = os.Symlink(caName, caNameSymlink)
		Expect(err).To(BeNil())

		secret = server.Secret{
			ServerCert:        "test-cert",
			ValidationContext: "test-validation-context",
			SslCaFile:         caName,
			SslCertFile:       certName,
			SslKeyFile:        keyName,
		}
	})

	AfterEach(func() {
		_ = fs.RemoveAll(dir)
	})

	It("runs and stops correctly", func() {
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
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
			_, err = client.FetchSecrets(context.TODO(), &envoy_service_discovery_v3.DiscoveryRequest{})
			return err != nil
		}, "5s", "1s").Should(BeTrue())

		// Cancel the context in order to stop the gRPC server
		cancel()

		// The gRPC server should stop eventually
		Eventually(func() bool {
			_, err = client.FetchSecrets(context.TODO(), &envoy_service_discovery_v3.DiscoveryRequest{})
			return err != nil
		}, "5s", "1s").Should(BeTrue())

	})

	It("correctly picks up multiple cert rotations", func() {

		go run.Run(context.Background(), []server.Secret{secret}, sdsClient, testServerAddress)

		// Give it a second to spin up + read the files
		time.Sleep(1 * time.Second)

		// Connect with the server
		var conn *grpc.ClientConn
		conn, err = grpc.Dial(testServerAddress, grpc.WithInsecure())
		Expect(err).To(BeNil())
		defer conn.Close()
		client := envoy_service_secret_v3.NewSecretDiscoveryServiceClient(conn)

		// Read certs
		certs, err := testutils.FilesToBytes(keyNameSymlink, certNameSymlink, caNameSymlink)
		Expect(err).NotTo(HaveOccurred())

		snapshotVersion, err := server.GetSnapshotVersion(certs)
		Expect(err).To(BeNil())
		Expect(snapshotVersion).To(Equal("6730780456972595554"))

		var resp *envoy_service_discovery_v3.DiscoveryResponse

		Eventually(func() bool {
			resp, err = client.FetchSecrets(context.TODO(), &envoy_service_discovery_v3.DiscoveryRequest{})
			return err == nil
		}, "5s", "1s").Should(BeTrue())

		Eventually(func() bool {
			resp, err = client.FetchSecrets(context.TODO(), &envoy_service_discovery_v3.DiscoveryRequest{})
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

		snapshotVersion, err = server.GetSnapshotVersion(certs)
		Expect(err).To(BeNil())
		Expect(snapshotVersion).To(Equal("16241649556325798095"))
		Eventually(func() bool {
			resp, err = client.FetchSecrets(context.TODO(), &envoy_service_discovery_v3.DiscoveryRequest{})
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

		snapshotVersion, err = server.GetSnapshotVersion(certs)
		Expect(err).To(BeNil())
		Expect(snapshotVersion).To(Equal("7644406922477208950"))
		Eventually(func() bool {
			resp, err = client.FetchSecrets(context.TODO(), &envoy_service_discovery_v3.DiscoveryRequest{})
			Expect(err).To(BeNil())
			return resp.VersionInfo == snapshotVersion
		}, "15s", "1s").Should(BeTrue())
	})
})

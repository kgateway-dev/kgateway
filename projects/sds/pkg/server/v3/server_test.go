package sds_server_v3_test

import (
	"context"
	"os"
	"path"

	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	envoy_service_secret_v3 "github.com/envoyproxy/go-control-plane/envoy/service/secret/v3"
	"github.com/solo-io/gloo/projects/sds/pkg/run"
	sds_server "github.com/solo-io/gloo/projects/sds/pkg/server"
	sds_server_v3 "github.com/solo-io/gloo/projects/sds/pkg/server/v3"
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
		testServerAddress                              = "127.0.0.1:8236"
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
	})

	AfterEach(func() {
		_ = fs.RemoveAll(dir)
	})

	It("correctly picks up multiple cert rotations", func() {

		sdsServers := []sds_server.EnvoySdsServerFactory{
			sds_server_v3.NewEnvoySdsServerV3,
		}
		go run.Run(context.Background(), keyNameSymlink, certNameSymlink, caNameSymlink, testServerAddress, sdsServers)

		// Connect with the server
		var conn *grpc.ClientConn
		conn, err = grpc.Dial(testServerAddress, grpc.WithInsecure())
		Expect(err).To(BeNil())
		defer conn.Close()
		client := envoy_service_secret_v3.NewSecretDiscoveryServiceClient(conn)
		snapshotVersion, err := sds_server.GetSnapshotVersion(keyName, certName, caName)
		Expect(err).To(BeNil())
		Expect(snapshotVersion).To(Equal("11240719828806193304"))
		resp, err := client.FetchSecrets(context.TODO(), &envoy_service_discovery_v3.DiscoveryRequest{})
		Expect(err).To(BeNil())
		Eventually(func() bool {
			resp, err = client.FetchSecrets(context.TODO(), &envoy_service_discovery_v3.DiscoveryRequest{})
			Expect(err).To(BeNil())
			return resp.VersionInfo == snapshotVersion
		}, "5s", "1s").Should(BeTrue())

		// Cert rotation #1
		err = os.Remove(keyName)
		Expect(err).To(BeNil())
		err = afero.WriteFile(fs, keyName, []byte("tls.key-1"), 0644)
		Expect(err).To(BeNil())

		snapshotVersion, err = sds_server.GetSnapshotVersion(keyName, certName, caName)
		Expect(err).To(BeNil())
		Expect(snapshotVersion).To(Equal("15601967965718698291"))
		resp, err = client.FetchSecrets(context.TODO(), &envoy_service_discovery_v3.DiscoveryRequest{})
		Eventually(func() bool {
			resp, err = client.FetchSecrets(context.TODO(), &envoy_service_discovery_v3.DiscoveryRequest{})
			Expect(err).To(BeNil())
			return resp.VersionInfo == snapshotVersion
		}, "5s", "1s").Should(BeTrue())

		// Cert rotation #2
		err = os.Remove(keyName)
		Expect(err).To(BeNil())
		err = afero.WriteFile(fs, keyName, []byte("tls.key-2"), 0644)
		Expect(err).To(BeNil())

		snapshotVersion, err = sds_server.GetSnapshotVersion(keyName, certName, caName)
		Expect(err).To(BeNil())
		Expect(snapshotVersion).To(Equal("11448956642433776987"))
		resp, err = client.FetchSecrets(context.TODO(), &envoy_service_discovery_v3.DiscoveryRequest{})
		Expect(err).To(BeNil())
		Eventually(func() bool {
			resp, err = client.FetchSecrets(context.TODO(), &envoy_service_discovery_v3.DiscoveryRequest{})
			Expect(err).To(BeNil())
			return resp.VersionInfo == snapshotVersion
		}, "5s", "1s").Should(BeTrue())
	})
})

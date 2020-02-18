package run_test

import (
	"context"

	envoy_api_v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_service_discovery_v2 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v2"
	"github.com/solo-io/gloo/projects/sds/pkg/run"
	"github.com/solo-io/gloo/projects/sds/pkg/server"
	"github.com/spf13/afero"
	"google.golang.org/grpc"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("SDS Server E2E Test", func() {

	var (
		fs                        afero.Fs
		dir                       string
		keyFile, certFile, caFile afero.File
		err                       error
		testServerAddress         = "127.0.0.1:8236"
	)

	BeforeEach(func() {
		fs = afero.NewOsFs()
		dir, err = afero.TempDir(fs, "", "")
		Expect(err).To(BeNil())
		fileString := `test`
		keyFile, err = afero.TempFile(fs, dir, "")
		Expect(err).To(BeNil())
		_, err = keyFile.WriteString(fileString)
		Expect(err).To(BeNil())
		certFile, err = afero.TempFile(fs, dir, "")
		Expect(err).To(BeNil())
		_, err = certFile.WriteString(fileString)
		Expect(err).To(BeNil())
		caFile, err = afero.TempFile(fs, dir, "")
		Expect(err).To(BeNil())
		_, err = caFile.WriteString(fileString)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		_ = fs.RemoveAll(dir)
	})

	It("runs and stops correctly", func() {
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			if err := run.Run(ctx, keyFile.Name(), certFile.Name(), caFile.Name(), testServerAddress); err != nil {
				Expect(err).To(BeNil())
			}
		}()

		// Connect with the server
		var conn *grpc.ClientConn
		conn, err := grpc.Dial(testServerAddress, grpc.WithInsecure())
		Expect(err).To(BeNil())
		defer conn.Close()
		client := envoy_service_discovery_v2.NewSecretDiscoveryServiceClient(conn)
		_, err = client.FetchSecrets(context.TODO(), &envoy_api_v2.DiscoveryRequest{})
		Expect(err).To(BeNil())

		// Cancel the context in order to stop the gRPC server
		cancel()

		// The gRPC server should stop eventually
		Eventually(func() bool {
			_, err = client.FetchSecrets(context.TODO(), &envoy_api_v2.DiscoveryRequest{})
			return err != nil
		}, "5s", "1s").Should(BeTrue())

	})

	It("correctly picks up multiple cert rotations", func() {
		go run.Run(context.Background(), keyFile.Name(), certFile.Name(), caFile.Name(), testServerAddress)

		// Connect with the server
		var conn *grpc.ClientConn
		conn, err := grpc.Dial(testServerAddress, grpc.WithInsecure())
		Expect(err).To(BeNil())
		defer conn.Close()
		client := envoy_service_discovery_v2.NewSecretDiscoveryServiceClient(conn)

		snapshotVersion, err := server.GetSnapshotVersion(keyFile.Name(), certFile.Name(), caFile.Name())
		Expect(err).To(BeNil())
		Expect(snapshotVersion).To(Equal("11240719828806193304"))

		resp, err := client.FetchSecrets(context.TODO(), &envoy_api_v2.DiscoveryRequest{})
		Expect(err).To(BeNil())
		Expect(resp.VersionInfo).To(Equal(snapshotVersion))

		// Cert rotation #1
		_, err = keyFile.WriteString(`newFileString`)
		Expect(err).To(BeNil())
		snapshotVersion, err = server.GetSnapshotVersion(keyFile.Name(), certFile.Name(), caFile.Name())
		Expect(err).To(BeNil())
		Expect(snapshotVersion).To(Equal("15327026688369869607"))
		resp, err = client.FetchSecrets(context.TODO(), &envoy_api_v2.DiscoveryRequest{})
		Eventually(func() bool {
			resp, err = client.FetchSecrets(context.TODO(), &envoy_api_v2.DiscoveryRequest{})
			Expect(err).To(BeNil())
			return resp.VersionInfo == snapshotVersion
		}, "5s", "1s").Should(BeTrue())

		// Cert rotation #2
		_, err = keyFile.WriteString(`newFileString2`)
		Expect(err).To(BeNil())
		snapshotVersion, err = server.GetSnapshotVersion(keyFile.Name(), certFile.Name(), caFile.Name())
		Expect(err).To(BeNil())
		Expect(snapshotVersion).To(Equal("15497820419858244991"))
		resp, err = client.FetchSecrets(context.TODO(), &envoy_api_v2.DiscoveryRequest{})
		Expect(err).To(BeNil())
		Eventually(func() bool {
			resp, err = client.FetchSecrets(context.TODO(), &envoy_api_v2.DiscoveryRequest{})
			Expect(err).To(BeNil())
			return resp.VersionInfo == snapshotVersion
		}, "5s", "1s").Should(BeTrue())
	})
})

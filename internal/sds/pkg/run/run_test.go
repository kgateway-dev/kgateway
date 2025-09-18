package run

import (
	"context"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	envoy_service_secret_v3 "github.com/envoyproxy/go-control-plane/envoy/service/secret/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/kgateway-dev/kgateway/v2/internal/sds/pkg/server"
	"github.com/kgateway-dev/kgateway/v2/internal/sds/pkg/testutils"
)

const (
	testServerAddress = "127.0.0.1:8236"
	sdsClient         = "test-client"
)

var logger = slog.New(slog.DiscardHandler)

func TestServerStartStop(t *testing.T) {
	// These tests use the Serial decorator because they rely on a hard-coded port for the SDS server (8236)
	r := require.New(t)
	data := setup(t)

	t.Cleanup(func() {
		err := os.RemoveAll(data.tmpDir)
		r.NoError(err)
	})

	ctx, cancel := context.WithCancel(context.Background())

	// Start the server
	go func() {
		err := Run(ctx, []server.Secret{data.secret}, sdsClient, testServerAddress, logger)
		r.NoError(err)
	}()
	// Give enough time for the server to start
	time.Sleep(100 * time.Millisecond)

	// Connect with the server
	conn, err := grpc.NewClient(testServerAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	r.NoError(err, "error creating gRPC client")
	defer conn.Close()
	client := envoy_service_secret_v3.NewSecretDiscoveryServiceClient(conn)

	// Check that we get a good response
	r.EventuallyWithT(func(c *assert.CollectT) {
		_, err = client.FetchSecrets(ctx, &envoy_service_discovery_v3.DiscoveryRequest{})
		require.NoError(c, err, "error fetching secrets")
	}, 10*time.Second, 1*time.Second)

	// Cancel the context in order to stop the gRPC server
	cancel()

	// The gRPC server should stop eventually
	r.EventuallyWithT(func(c *assert.CollectT) {
		_, err = client.FetchSecrets(ctx, &envoy_service_discovery_v3.DiscoveryRequest{})
		require.Error(c, err, "expected error fetching secrets")
	}, 10*time.Second, 1*time.Second)
}

func TestCertRotation(t *testing.T) {
	testCases := []struct {
		name           string
		ocsp           bool
		expectedHashes []string
	}{
		{
			name:           "with ocsp",
			ocsp:           true,
			expectedHashes: []string{"969835737182439215", "6265739243366543658", "14893951670674740726"},
		},
		{
			name:           "without ocsp",
			ocsp:           false,
			expectedHashes: []string{"6730780456972595554", "16241649556325798095", "7644406922477208950"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r := require.New(t)
			tmpDir, err := os.MkdirTemp("", "kgateway-test-sds")
			r.NoError(err)

			t.Cleanup(func() {
				err := os.RemoveAll(tmpDir)
				r.NoError(err)
			})

			data := setup(t)
			ctx := t.Context()

			r.EventuallyWithT(func(c *assert.CollectT) {
				open, err := isPortOpen(testServerAddress)
				require.NoError(c, err, "error checking if port is open")
				require.False(c, open, "expected server port to be closed")
			}, 5*time.Second, 500*time.Millisecond)

			go func() {
				ocsp := ""
				if tc.ocsp {
					ocsp = data.ocspName
				}

				data.secret.SslOcspFile = ocsp
				err := Run(ctx, []server.Secret{data.secret}, sdsClient, testServerAddress, logger)
				r.NoError(err, "error starting SDS server")
			}()
			// Give enough time for the server to start
			time.Sleep(100 * time.Millisecond)

			// Connect with the server
			conn, err := grpc.NewClient(testServerAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
			r.NoError(err, "error creating gRPC client")
			defer conn.Close()
			client := envoy_service_secret_v3.NewSecretDiscoveryServiceClient(conn)

			// Read certs
			var certs [][]byte
			if tc.ocsp {
				certs, err = testutils.FilesToBytes(data.keyNameSymlink, data.certNameSymlink, data.caNameSymlink, data.ocspNameSymlink)
			} else {
				certs, err = testutils.FilesToBytes(data.keyNameSymlink, data.certNameSymlink, data.caNameSymlink)
			}
			r.NoError(err, "error converting certs to bytes")

			snapshotVersion, err := server.GetSnapshotVersion(certs)
			r.NoError(err, "error getting snapshot version")
			r.Equal(tc.expectedHashes[0], snapshotVersion, "unexpected snapshot version")

			var resp *envoy_service_discovery_v3.DiscoveryResponse
			r.EventuallyWithT(func(c *assert.CollectT) {
				resp, err = client.FetchSecrets(ctx, &envoy_service_discovery_v3.DiscoveryRequest{})
				require.NoError(c, err, "error fetching secrets")
				require.NotNil(c, resp, "expected non-nil response")
				require.Equal(c, snapshotVersion, resp.VersionInfo, "unexpected snapshot version")
			}, 15*time.Second, 500*time.Millisecond)

			// Cert rotation #1
			err = os.Remove(data.keyName)
			r.NoError(err, "error removing key file")
			err = os.WriteFile(data.keyName, []byte("tls.key-1"), 0o644)
			r.NoError(err, "error writing new key file")

			// Re-read certs
			certs, err = testutils.FilesToBytes(data.keyNameSymlink, data.certNameSymlink, data.caNameSymlink)
			r.NoError(err, "error reading certs")
			if tc.ocsp {
				ocspBytes, err := os.ReadFile(data.ocspNameSymlink)
				r.NoError(err, "error reading OCSP cert")
				certs = append(certs, ocspBytes)
			}

			snapshotVersion, err = server.GetSnapshotVersion(certs)
			r.NoError(err, "error getting snapshot version")
			r.Equal(tc.expectedHashes[1], snapshotVersion, "unexpected snapshot version")

			r.EventuallyWithT(func(c *assert.CollectT) {
				resp, err = client.FetchSecrets(ctx, &envoy_service_discovery_v3.DiscoveryRequest{})
				require.NoError(c, err, "error fetching secrets")
				require.NotNil(c, resp, "expected non-nil response")
				require.Equal(c, snapshotVersion, resp.VersionInfo, "unexpected snapshot version")
			}, 15*time.Second, 500*time.Millisecond)

			// Cert rotation #2
			err = os.Remove(data.keyName)
			r.NoError(err, "error removing key file")
			err = os.WriteFile(data.keyName, []byte("tls.key-2"), 0o644)
			r.NoError(err, "error writing new key file")

			// Re-read certs again
			certs, err = testutils.FilesToBytes(data.keyNameSymlink, data.certNameSymlink, data.caNameSymlink)
			r.NoError(err, "error reading certs")
			if tc.ocsp {
				ocspBytes, err := os.ReadFile(data.ocspNameSymlink)
				r.NoError(err, "error reading OCSP cert")
				certs = append(certs, ocspBytes)
			}

			snapshotVersion, err = server.GetSnapshotVersion(certs)
			r.NoError(err, "error getting snapshot version")
			r.Equal(tc.expectedHashes[2], snapshotVersion, "unexpected snapshot version")

			r.EventuallyWithT(func(c *assert.CollectT) {
				resp, err = client.FetchSecrets(ctx, &envoy_service_discovery_v3.DiscoveryRequest{})
				require.NoError(c, err, "error fetching secrets")
				require.NotNil(c, resp, "expected non-nil response")
				require.Equal(c, snapshotVersion, resp.VersionInfo, "unexpected snapshot version")
			}, 15*time.Second, 500*time.Millisecond)
		})
	}
}

type setupData struct {
	tmpDir          string
	keyName         string
	ocspName        string
	keyNameSymlink  string
	certNameSymlink string
	caNameSymlink   string
	ocspNameSymlink string
	secret          server.Secret
}

func setup(t *testing.T) setupData {
	r := require.New(t)

	fileString := []byte("test")
	dir, err := os.MkdirTemp("", "kgateway-test-sds")
	r.NoError(err)

	// Kubernetes mounts secrets as a symlink to a ..data directory, so we'll mimic that here
	keyName := filepath.Join(dir, "tls.key-0")
	certName := filepath.Join(dir, "tls.crt-0")
	caName := filepath.Join(dir, "ca.crt-0")
	ocspName := filepath.Join(dir, "tls.ocsp-staple-0")
	err = os.WriteFile(keyName, fileString, 0o644)

	r.NoError(err)
	err = os.WriteFile(certName, fileString, 0o644)
	r.NoError(err)
	err = os.WriteFile(caName, fileString, 0o644)
	r.NoError(err)

	// This is a pre-generated DER-encoded OCSP response using `openssl` to better match actual ocsp staple/response data.
	// This response isn't for the test certs as they are just random data, but it is a syntactically-valid OCSP response.
	ocspResponse, err := os.ReadFile(filepath.Join("testdata", "ocsp_response.der"))
	r.NoError(err)
	err = os.WriteFile(ocspName, ocspResponse, 0o644)
	r.NoError(err)

	keyNameSymlink := filepath.Join(dir, "tls.key")
	certNameSymlink := filepath.Join(dir, "tls.crt")
	caNameSymlink := filepath.Join(dir, "ca.crt")
	ocspNameSymlink := filepath.Join(dir, "tls.ocsp-staple")
	err = os.Symlink(keyName, keyNameSymlink)
	r.NoError(err)
	err = os.Symlink(certName, certNameSymlink)
	r.NoError(err)
	err = os.Symlink(caName, caNameSymlink)
	r.NoError(err)
	err = os.Symlink(ocspName, ocspNameSymlink)
	r.NoError(err)

	return setupData{
		tmpDir:          dir,
		keyName:         keyName,
		ocspName:        ocspName,
		keyNameSymlink:  keyNameSymlink,
		certNameSymlink: certNameSymlink,
		caNameSymlink:   caNameSymlink,
		ocspNameSymlink: ocspNameSymlink,
		secret: server.Secret{
			ServerCert:        "test-cert",
			ValidationContext: "test-validation-context",
			SslCaFile:         caName,
			SslCertFile:       certName,
			SslKeyFile:        keyName,
			SslOcspFile:       ocspName,
		},
	}
}

func isPortOpen(address string) (bool, error) {
	conn, err := net.Dial("tcp", address)
	if err == nil {
		err := conn.Close()
		return true, err
	}
	return false, nil
}

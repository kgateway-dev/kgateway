package run

import (
	"context"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	envoy_service_secret_v3 "github.com/envoyproxy/go-control-plane/envoy/service/secret/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/kgateway-dev/kgateway/v2/pkg/sds/server"
	"github.com/kgateway-dev/kgateway/v2/pkg/sds/testutils"
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

func TestRunReturnsWhenContextCanceled(t *testing.T) {
	r := require.New(t)

	keyPEM, certPEM, caPEM := testutils.MustSelfSignedPEM()
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "key.pem")
	certPath := filepath.Join(dir, "cert.pem")
	caPath := filepath.Join(dir, "ca.pem")
	r.NoError(os.WriteFile(keyPath, keyPEM, 0o600))
	r.NoError(os.WriteFile(certPath, certPEM, 0o600))
	r.NoError(os.WriteFile(caPath, caPEM, 0o600))

	secret := server.Secret{
		ServerCert:        "test-cert",
		ValidationContext: "test-validation-context",
		SslCaFile:         caPath,
		SslCertFile:       certPath,
		SslKeyFile:        keyPath,
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, []server.Secret{secret}, "test-client", "127.0.0.1:0", logger)
	}()

	time.Sleep(250 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		r.NoError(err, "Run returned unexpected error after context cancel")
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after context cancellation")
	}
}

func TestCertRotation(t *testing.T) {
	testCases := []struct {
		name string
		ocsp bool
	}{
		{
			name: "with ocsp",
			ocsp: true,
		},
		{
			name: "without ocsp",
			ocsp: false,
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
				if !tc.ocsp {
					data.secret.SslOcspFile = ""
				}
				err := Run(ctx, []server.Secret{data.secret}, sdsClient, testServerAddress, logger)
				r.NoError(err, "error starting SDS server")
			}()
			time.Sleep(2 * time.Second)

			// Connect with the server
			conn, err := grpc.NewClient(testServerAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
			r.NoError(err, "error creating gRPC client")
			defer conn.Close()
			client := envoy_service_secret_v3.NewSecretDiscoveryServiceClient(conn)

			paths := []string{data.keyNameSymlink, data.certNameSymlink, data.caNameSymlink}
			if tc.ocsp {
				paths = append(paths, data.ocspNameSymlink)
			}
			certs, err := testutils.FilesToBytes(paths...)
			r.NoError(err, "error converting certs to bytes")

			snapshotVersion, err := server.GetSnapshotVersion(certs)
			r.NoError(err, "error getting snapshot version")

			var resp *envoy_service_discovery_v3.DiscoveryResponse
			assertOCSPState := func(resp *envoy_service_discovery_v3.DiscoveryResponse) {
				var serverSecret *envoytlsv3.Secret
				for _, resource := range resp.GetResources() {
					parsed := new(envoytlsv3.Secret)
					r.NoError(resource.UnmarshalTo(parsed), "error unmarshalling secret resource")
					if parsed.GetName() == data.secret.ServerCert {
						serverSecret = parsed
						break
					}
				}
				r.NotNil(serverSecret, "expected tls secret in response")
				r.NotNil(serverSecret.GetTlsCertificate(), "expected tls certificate in response")
				if tc.ocsp {
					ocspBytes, err := os.ReadFile(data.ocspNameSymlink)
					r.NoError(err, "error reading ocsp file")
					r.Equal(ocspBytes, serverSecret.GetTlsCertificate().GetOcspStaple().GetInlineBytes(), "unexpected ocsp staple bytes")
				} else {
					r.Nil(serverSecret.GetTlsCertificate().GetOcspStaple(), "expected ocsp staple to be omitted")
				}
			}

			r.EventuallyWithT(func(c *assert.CollectT) {
				resp, err = client.FetchSecrets(ctx, &envoy_service_discovery_v3.DiscoveryRequest{})
				require.NoError(c, err, "error fetching secrets")
				require.NotNil(c, resp, "expected non-nil response")
				require.Equal(c, snapshotVersion, resp.VersionInfo, "unexpected snapshot version")
			}, 20*time.Second, 500*time.Millisecond)
			assertOCSPState(resp)

			// Cert rotation #1
			key1, cert1, ca1 := testutils.MustSelfSignedPEMRotation1()
			err = os.Remove(data.keyName)
			r.NoError(err, "error removing key file")
			err = os.WriteFile(data.keyName, key1, 0o600)
			r.NoError(err, "error writing new key file")
			err = os.Remove(data.certName)
			r.NoError(err, "error removing cert file")
			err = os.WriteFile(data.certName, cert1, 0o600)
			r.NoError(err, "error writing new cert file")
			err = os.Remove(data.caName)
			r.NoError(err, "error removing ca file")
			err = os.WriteFile(data.caName, ca1, 0o600)
			r.NoError(err, "error writing new ca file")

			// Re-read certs
			certs, err = testutils.FilesToBytes(paths...)
			r.NoError(err, "error reading certs")

			snapshotVersion, err = server.GetSnapshotVersion(certs)
			r.NoError(err, "error getting snapshot version")

			r.EventuallyWithT(func(c *assert.CollectT) {
				resp, err = client.FetchSecrets(ctx, &envoy_service_discovery_v3.DiscoveryRequest{})
				require.NoError(c, err, "error fetching secrets")
				require.NotNil(c, resp, "expected non-nil response")
				require.Equal(c, snapshotVersion, resp.VersionInfo, "unexpected snapshot version")
			}, 20*time.Second, 500*time.Millisecond)
			assertOCSPState(resp)

			// Cert rotation #2
			key2, cert2, ca2 := testutils.MustSelfSignedPEMRotation2()
			err = os.Remove(data.keyName)
			r.NoError(err, "error removing key file")
			err = os.WriteFile(data.keyName, key2, 0o600)
			r.NoError(err, "error writing new key file")
			err = os.Remove(data.certName)
			r.NoError(err, "error removing cert file")
			err = os.WriteFile(data.certName, cert2, 0o600)
			r.NoError(err, "error writing new cert file")
			err = os.Remove(data.caName)
			r.NoError(err, "error removing ca file")
			err = os.WriteFile(data.caName, ca2, 0o600)
			r.NoError(err, "error writing new ca file")

			// Re-read certs again
			certs, err = testutils.FilesToBytes(paths...)
			r.NoError(err, "error reading certs")

			snapshotVersion, err = server.GetSnapshotVersion(certs)
			r.NoError(err, "error getting snapshot version")

			r.EventuallyWithT(func(c *assert.CollectT) {
				resp, err = client.FetchSecrets(ctx, &envoy_service_discovery_v3.DiscoveryRequest{})
				require.NoError(c, err, "error fetching secrets")
				require.NotNil(c, resp, "expected non-nil response")
				require.Equal(c, snapshotVersion, resp.VersionInfo, "unexpected snapshot version")
			}, 20*time.Second, 500*time.Millisecond)
			assertOCSPState(resp)
		})
	}
}

type setupData struct {
	tmpDir          string
	keyName         string
	certName        string
	caName          string
	ocspName        string
	keyNameSymlink  string
	certNameSymlink string
	caNameSymlink   string
	ocspNameSymlink string
	secret          server.Secret
}

func setup(t *testing.T) setupData {
	r := require.New(t)

	dir, err := os.MkdirTemp("", "kgateway-test-sds")
	r.NoError(err)

	keyPEM, certPEM, caPEM := testutils.MustSelfSignedPEM()
	ocspResp, err := os.ReadFile(filepath.Join("testdata", "ocsp_response.der"))
	r.NoError(err)

	// Kubernetes mounts secrets as a symlink to a ..data directory, so we'll mimic that here
	keyName := filepath.Join(dir, "tls.key-0")
	certName := filepath.Join(dir, "tls.crt-0")
	caName := filepath.Join(dir, "ca.crt-0")
	ocspName := filepath.Join(dir, "tls.ocsp-staple-0")
	err = os.WriteFile(keyName, keyPEM, 0o600)

	r.NoError(err)
	err = os.WriteFile(certName, certPEM, 0o600)
	r.NoError(err)
	err = os.WriteFile(caName, caPEM, 0o600)
	r.NoError(err)
	err = os.WriteFile(ocspName, ocspResp, 0o600)
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
		certName:        certName,
		caName:          caName,
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

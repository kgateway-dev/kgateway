package validator

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	envoybootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	eiutils "github.com/kgateway-dev/kgateway/v2/internal/envoyinit/pkg/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/bootstrapoverlay"
)

// sdsBootstrapYAML is a proxy bootstrap whose xDS cluster reads its CA and its token
// through file-based SDS, nested in Any fields as in the generated bootstrap.
const sdsBootstrapYAML = `
node:
  cluster: gw
static_resources:
  clusters:
  - name: xds_cluster
    type: STATIC
    connect_timeout: 5s
    load_assignment:
      cluster_name: xds_cluster
      endpoints:
      - lb_endpoints:
        - endpoint: {address: {socket_address: {address: 127.0.0.1, port_value: 9977}}}
    transport_socket:
      name: envoy.transport_sockets.tls
      typed_config:
        "@type": type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.UpstreamTlsContext
        common_tls_context:
          validation_context_sds_secret_config:
            name: xds-ca
            sds_config:
              path_config_source: {path: /etc/envoy/xds_ca_sds.json}
              resource_api_version: V3
    typed_extension_protocol_options:
      envoy.extensions.upstreams.http.v3.HttpProtocolOptions:
        "@type": type.googleapis.com/envoy.extensions.upstreams.http.v3.HttpProtocolOptions
        explicit_http_config: {http2_protocol_options: {}}
        http_filters:
        - name: envoy.filters.http.credential_injector
          typed_config:
            "@type": type.googleapis.com/envoy.extensions.filters.http.credential_injector.v3.CredentialInjector
            credential:
              name: envoy.http.injected_credentials.generic
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.http.injected_credentials.generic.v3.Generic
                credential:
                  name: xds-jwt-token
                  sds_config:
                    path_config_source: {path: /etc/envoy/xds_service_account_token.json}
                    resource_api_version: V3
        - name: envoy.filters.http.upstream_codec
          typed_config:
            "@type": type.googleapis.com/envoy.extensions.filters.http.upstream_codec.v3.UpstreamCodec
`

func parseTestBootstrap(t *testing.T, y string) *envoybootstrapv3.Bootstrap {
	t.Helper()
	bs, err := bootstrapoverlay.ParseBootstrap([]byte(y))
	require.NoError(t, err)
	return bs
}

// configSourcePaths returns each ConfigSource file path in bs, in walk order.
func configSourcePaths(t *testing.T, bs *envoybootstrapv3.Bootstrap) []string {
	t.Helper()
	var paths []string
	err := rewriteConfigSourcePaths(proto.CloneOf(bs).ProtoReflect(), func(p string) (string, error) {
		paths = append(paths, p)
		return p, nil
	})
	require.NoError(t, err)
	return paths
}

func TestPrepareProxyBootstrap(t *testing.T) {
	// Prepare a bootstrap whose second cluster reads the same CA file as the first.
	bs := parseTestBootstrap(t, sdsBootstrapYAML)
	second := proto.CloneOf(bs.GetStaticResources().GetClusters()[0])
	second.Name = "second"
	bs.StaticResources.Clusters = append(bs.StaticResources.Clusters, second)
	original := proto.CloneOf(bs)
	dir := t.TempDir()
	out, err := prepareProxyBootstrap(bs, dir)
	require.NoError(t, err)
	require.True(t, proto.Equal(original, bs), "preparing must not change the bootstrap")

	// The copy has the node id and the system CA secret the proxy pod gets.
	prepared := &envoybootstrapv3.Bootstrap{}
	require.NoError(t, protojson.Unmarshal(out, prepared))
	assert.Equal(t, validationNodeID, prepared.GetNode().GetId())
	assert.Equal(t, "gw", prepared.GetNode().GetCluster(), "the rest of node is kept")
	secrets := prepared.GetStaticResources().GetSecrets()
	require.Len(t, secrets, 1)
	assert.Equal(t, eiutils.SystemCaSecretName, secrets[0].GetName())

	// Each file path, including those nested in Any fields, points at an empty file in
	// dir. The two bootstraps have the same structure, so their paths pair up in walk order.
	originals := configSourcePaths(t, bs)
	assert.ElementsMatch(t, []string{
		"/etc/envoy/xds_ca_sds.json", "/etc/envoy/xds_service_account_token.json",
		"/etc/envoy/xds_ca_sds.json", "/etc/envoy/xds_service_account_token.json",
	}, originals)
	placeholders := configSourcePaths(t, prepared)
	require.Len(t, placeholders, len(originals))
	byOriginal := map[string]string{}
	for i, p := range placeholders {
		assert.True(t, strings.HasPrefix(p, dir+string(os.PathSeparator)), "placeholder %s should be in %s", p, dir)
		data, err := os.ReadFile(p)
		require.NoError(t, err)
		assert.Empty(t, data)
		byOriginal[originals[i]] = p
	}

	// The same path gets the same placeholder, and different paths different ones.
	assert.Len(t, byOriginal, 2)
	assert.NotEqual(t, byOriginal["/etc/envoy/xds_ca_sds.json"], byOriginal["/etc/envoy/xds_service_account_token.json"])
	for i, p := range placeholders {
		assert.Equal(t, byOriginal[originals[i]], p)
	}
}

// fileTLSBootstrapYAML is a proxy bootstrap with a static cluster whose client
// certificate, key and CA are files mounted in the proxy pod, nested in an Any, and a
// static secret that names the same CA file.
const fileTLSBootstrapYAML = `
static_resources:
  clusters:
  - name: user_otel
    type: STATIC
    connect_timeout: 5s
    load_assignment:
      cluster_name: user_otel
      endpoints:
      - lb_endpoints:
        - endpoint: {address: {socket_address: {address: 127.0.0.1, port_value: 4317}}}
    transport_socket:
      name: envoy.transport_sockets.tls
      typed_config:
        "@type": type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.UpstreamTlsContext
        common_tls_context:
          tls_certificates:
          - certificate_chain: {filename: /proxy/only/tls.crt}
            private_key: {filename: /proxy/only/tls.key}
          validation_context:
            trusted_ca: {filename: /proxy/only/ca.crt}
  secrets:
  - name: user_ca
    validation_context:
      trusted_ca: {filename: /proxy/only/ca.crt}
`

func TestPrepareProxyBootstrapKeepsTLSFiles(t *testing.T) {
	// Prepare a bootstrap that names TLS files only the proxy pod has.
	bs := parseTestBootstrap(t, fileTLSBootstrapYAML)
	out, err := prepareProxyBootstrap(bs, t.TempDir())
	require.NoError(t, err)

	// The copy keeps the paths, so Envoy opens the files where the controller runs it.
	prepared := &envoybootstrapv3.Bootstrap{}
	require.NoError(t, protojson.Unmarshal(out, prepared))
	assert.Equal(t, "/proxy/only/ca.crt", prepared.GetStaticResources().GetSecrets()[0].GetValidationContext().GetTrustedCa().GetFilename(),
		"the CA should stay a file path")
}

func TestValidateProxyBootstrapPlaceholdersReadable(t *testing.T) {
	// Validate a bootstrap with file-based SDS, recording the placeholders' modes as
	// Envoy would see them.
	var dirMode os.FileMode
	var fileModes []os.FileMode
	err := validateProxyBootstrap(t.Context(), parseTestBootstrap(t, sdsBootstrapYAML), time.Minute,
		func(_ context.Context, dir string, _ []string, _ []byte) (string, error) {
			info, err := os.Stat(dir)
			require.NoError(t, err)
			dirMode = info.Mode().Perm()
			entries, err := os.ReadDir(dir)
			require.NoError(t, err)
			for _, e := range entries {
				info, err := e.Info()
				require.NoError(t, err)
				fileModes = append(fileModes, info.Mode().Perm())
			}
			return "", nil
		})
	require.NoError(t, err)

	// Envoy in the validator's image runs as its own user, which must be able to open
	// the directory and read each placeholder.
	assert.Equal(t, os.FileMode(0o755), dirMode, "the placeholder directory should be open to other users")
	require.Len(t, fileModes, 2)
	for _, m := range fileModes {
		assert.Equal(t, os.FileMode(0o004), m&0o004, "placeholder mode %v should be readable by other users", m)
	}
}

func TestBinaryValidator_ValidateBootstrap(t *testing.T) {
	tests := []struct {
		name      string
		bootstrap string
		script    string
		// wantErr is the rejection; empty when Envoy accepts the bootstrap.
		wantErr string
	}{
		{
			name:   "accepted",
			script: "#!/bin/sh\nexit 0\n",
		},
		{
			name: "rejected",
			script: `#!/bin/sh
echo "[error] something earlier" >&2
echo "error initializing configuration '': Proto constraint validation failed" >&2
exit 1
`,
			wantErr: "Envoy rejected the bootstrap: error initializing configuration '': Proto constraint validation failed",
		},
		{
			// The mock echoes its arguments, so the rejection shows them.
			name: "runs at critical level with a bare log format",
			script: `#!/bin/sh
echo "$@" >&2
exit 1
`,
			wantErr: "Envoy rejected the bootstrap: --mode validate --config-path /dev/fd/0 -l critical --log-format %v",
		},
		{
			// Envoy rejects the proxy's log format before applying it, so it writes the
			// reason with its default log prefix, and without the log format it passes.
			// The mock echoes its arguments, so the rejection shows them.
			name:      "keeps the proxy's log format",
			bootstrap: `{application_log_config: {log_format: {json_format: {message: "%v"}}}}`,
			script: `#!/bin/sh
case "$*" in *--log-format*) exit 0 ;; esac
echo "[2026-11-24 05:01:30.246][1][critical][main] [source/server/config_validation/server.cc:76] error initializing configuration '': setJsonLogFormat error: $*" >&2
exit 1
`,
			wantErr: "Envoy rejected the bootstrap: error initializing configuration '': setJsonLogFormat error: --mode validate --config-path /dev/fd/0 -l critical",
		},
		{
			// The proxy's log format leaves Envoy's reason out, so the reason comes from
			// validating again without it.
			name:      "a reason the proxy's log format leaves out",
			bootstrap: `{application_log_config: {log_format: {json_format: {message: "%LOCAL_REPLY_BODY%"}}}}`,
			script: `#!/bin/sh
case "$*" in *--log-format*) echo "error initializing configuration '': duplicate cluster" >&2; exit 1 ;; esac
echo '{"message":"COCAL_REPLY_BODY%"}' >&2
exit 1
`,
			wantErr: "Envoy rejected the bootstrap: error initializing configuration '': duplicate cluster",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Validate with a mock Envoy.
			bs := &envoybootstrapv3.Bootstrap{}
			if tt.bootstrap != "" {
				bs = parseTestBootstrap(t, tt.bootstrap)
			}
			err := NewBinary(createMockBinary(t, tt.script)).(BootstrapValidator).ValidateBootstrap(t.Context(), bs)

			// Check the verdict.
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, ErrInvalidBootstrap)
			assert.Equal(t, tt.wantErr, err.Error())
		})
	}

	t.Run("Envoy cannot be run", func(t *testing.T) {
		err := NewBinary("/nonexistent/envoy").(BootstrapValidator).ValidateBootstrap(t.Context(), &envoybootstrapv3.Bootstrap{})
		require.ErrorIs(t, err, ErrBootstrapValidatorUnavailable)
		assert.NotErrorIs(t, err, ErrInvalidBootstrap, "a failure to run Envoy is not a verdict on the bootstrap")
	})
}

func TestBinaryValidator_ValidateBootstrapFailures(t *testing.T) {
	validate := func(t *testing.T, ctx context.Context, script string) error {
		t.Helper()
		return NewBinary(createMockBinary(t, script)).(BootstrapValidator).ValidateBootstrap(ctx, &envoybootstrapv3.Bootstrap{})
	}

	t.Run("a crash rejects the bootstrap", func(t *testing.T) {
		// Envoy crashes on a signal after logging why.
		err := validate(t, t.Context(), "#!/bin/sh\necho 'caught Segmentation fault' >&2\nkill -SEGV $$\n")

		// The crash is a verdict on the bootstrap, since the proxy would crash too.
		require.ErrorIs(t, err, ErrInvalidBootstrap)
		assert.Equal(t, `Envoy rejected the bootstrap: Envoy crashed with signal "segmentation fault": caught Segmentation fault`, err.Error())
	})

	t.Run("SIGKILL is not a verdict", func(t *testing.T) {
		// Envoy is killed from outside, as by the out-of-memory killer.
		err := validate(t, t.Context(), "#!/bin/sh\nkill -KILL $$\n")

		// Validating again can succeed, so the error is neither a rejection nor permanent.
		require.EqualError(t, err, "Envoy was killed while validating the bootstrap, possibly because the controller ran out of memory")
		assert.NotErrorIs(t, err, ErrInvalidBootstrap)
		assert.NotErrorIs(t, err, ErrBootstrapValidatorUnavailable)
	})

	t.Run("the caller's context ending is not a verdict", func(t *testing.T) {
		// The caller stops waiting while Envoy runs; Envoy is killed with SIGKILL.
		ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
		defer cancel()
		err := validate(t, ctx, "#!/bin/sh\nsleep 5\n")

		// The error is the context's.
		require.ErrorIs(t, err, context.DeadlineExceeded)
		assert.NotErrorIs(t, err, ErrInvalidBootstrap)
	})

	t.Run("the validation timeout is not a verdict", func(t *testing.T) {
		// Run an Envoy that outlasts the timeout.
		path := createMockBinary(t, "#!/bin/sh\nsleep 5\n")
		err := validateProxyBootstrap(t.Context(), &envoybootstrapv3.Bootstrap{}, 200*time.Millisecond,
			func(ctx context.Context, _ string, args []string, config []byte) (string, error) {
				return runEnvoy(exec.CommandContext(ctx, path, args...), config)
			})

		// The timeout can pass on a less busy controller, so it is neither a rejection
		// nor permanent, and not the caller's context.
		require.EqualError(t, err, "Envoy did not finish validating the bootstrap within 200ms")
		assert.NotErrorIs(t, err, ErrInvalidBootstrap)
		assert.NotErrorIs(t, err, ErrBootstrapValidatorUnavailable)
		assert.NotErrorIs(t, err, context.DeadlineExceeded)
	})

	t.Run("a temporary directory that cannot be created is permanent", func(t *testing.T) {
		// Point the temporary directory at a path that does not exist, after writing
		// the mock Envoy.
		path := createMockBinary(t, "#!/bin/sh\nexit 0\n")
		t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "missing"))
		err := NewBinary(path).(BootstrapValidator).ValidateBootstrap(t.Context(), &envoybootstrapv3.Bootstrap{})

		// Validating again fails the same way until the controller's Deployment changes.
		require.ErrorIs(t, err, ErrBootstrapValidatorUnavailable)
	})
}

func TestDockerValidator_ValidateBootstrapDockerFailures(t *testing.T) {
	tests := []struct {
		name     string
		exitCode int
		// permanent is whether validating again fails the same way.
		permanent bool
		wantErr   string
	}{
		{
			name:     "docker cannot run the container",
			exitCode: 125,
			wantErr:  "failed to run Envoy to validate the bootstrap: docker could not run the Envoy container: Cannot connect to the Docker daemon",
		},
		{
			name:      "the image cannot run Envoy",
			exitCode:  126,
			permanent: true,
		},
		{
			name:      "the image has no Envoy",
			exitCode:  127,
			permanent: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Put a docker CLI on PATH that fails with the exit code.
			bin := t.TempDir()
			script := fmt.Sprintf("#!/bin/sh\necho 'Cannot connect to the Docker daemon' >&2\nexit %d\n", tt.exitCode)
			require.NoError(t, os.WriteFile(filepath.Join(bin, "docker"), []byte(script), 0o755)) //nolint:gosec // G306: the script must be executable
			t.Setenv("PATH", bin)

			// Validate with it.
			err := NewDocker().(BootstrapValidator).ValidateBootstrap(t.Context(), &envoybootstrapv3.Bootstrap{})

			// Docker's failure is never a verdict on the bootstrap.
			require.Error(t, err)
			assert.NotErrorIs(t, err, ErrInvalidBootstrap)
			if tt.permanent {
				assert.ErrorIs(t, err, ErrBootstrapValidatorUnavailable)
			} else {
				assert.NotErrorIs(t, err, ErrBootstrapValidatorUnavailable)
			}
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
			}
		})
	}
}

// stubBootstrapValidator is an inner validator that counts proxy bootstrap
// validations and returns a programmed response.
type stubBootstrapValidator struct {
	stubValidator
	bootstrapCalls int
	bootstrapErr   error
}

func (s *stubBootstrapValidator) ValidateBootstrap(context.Context, *envoybootstrapv3.Bootstrap) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bootstrapCalls++
	return s.bootstrapErr
}

func TestCachingValidator_ValidateBootstrap(t *testing.T) {
	t.Run("memoizes acceptance", func(t *testing.T) {
		// Validate the same content twice and different content once.
		stub := &stubBootstrapValidator{}
		v := NewCaching(stub, 0).(BootstrapValidator)
		for range 2 {
			require.NoError(t, v.ValidateBootstrap(t.Context(), bootstrapForNode("a")))
		}
		require.NoError(t, v.ValidateBootstrap(t.Context(), bootstrapForNode("b")))

		// Envoy ran once per content, and xDS validation was not used.
		assert.Equal(t, 2, stub.bootstrapCalls)
		assert.Equal(t, 0, stub.Calls())
	})

	t.Run("memoizes rejection", func(t *testing.T) {
		// Validate a rejected bootstrap twice.
		stub := &stubBootstrapValidator{bootstrapErr: errors.Join(ErrInvalidBootstrap, errors.New("bad"))}
		v := NewCaching(stub, 0).(BootstrapValidator)
		err1 := v.ValidateBootstrap(t.Context(), bootstrapForNode("a"))
		err2 := v.ValidateBootstrap(t.Context(), bootstrapForNode("a"))

		// Both get the same rejection, from one run.
		require.ErrorIs(t, err1, ErrInvalidBootstrap)
		require.ErrorIs(t, err2, ErrInvalidBootstrap)
		assert.Equal(t, err1.Error(), err2.Error())
		assert.Equal(t, 1, stub.bootstrapCalls)
	})

	t.Run("does not memoize a failure to run Envoy", func(t *testing.T) {
		// Fail to run Envoy twice.
		stub := &stubBootstrapValidator{bootstrapErr: errors.New("exec format error")}
		v := NewCaching(stub, 0).(BootstrapValidator)
		for range 2 {
			require.Error(t, v.ValidateBootstrap(t.Context(), bootstrapForNode("a")))
		}
		assert.Equal(t, 2, stub.bootstrapCalls)
	})

	t.Run("refuses when the inner validator cannot validate bootstraps", func(t *testing.T) {
		err := NewCaching(&stubValidator{}, 0).(BootstrapValidator).ValidateBootstrap(t.Context(), bootstrapForNode("a"))
		require.ErrorIs(t, err, ErrBootstrapValidatorUnavailable)
		require.EqualError(t, err, "the controller cannot run Envoy to validate the bootstrap: the configured validator cannot validate proxy bootstraps")
	})
}

func TestBootstrapValidatorFor(t *testing.T) {
	// A validator that can validate bootstraps is used as is.
	binary := NewBinary()
	assert.Same(t, binary, BootstrapValidatorFor(binary))
	caching := NewCaching(NewBinary(), 0)
	assert.Same(t, caching, BootstrapValidatorFor(caching))

	// Otherwise, the Envoy binary is used behind a cache.
	for _, v := range []Validator{nil, &stubValidator{}, NewCaching(&stubValidator{}, 0)} {
		bv := BootstrapValidatorFor(v)
		cv, ok := bv.(*cachingValidator)
		require.True(t, ok, "expected a caching validator, got %T", bv)
		assert.IsType(t, &binaryValidator{}, cv.inner)
	}
}

func TestDockerValidator_ValidateBootstrap(t *testing.T) {
	v := NewDocker().(BootstrapValidator)

	t.Run("file-based SDS passes with placeholder files", func(t *testing.T) {
		// Envoy requires each path_config_source file to exist, so without the
		// placeholders the bootstrap fails.
		bs := parseTestBootstrap(t, sdsBootstrapYAML)
		err := NewDocker().Validate(t.Context(), bs)
		require.ErrorIs(t, err, ErrInvalidXDS)
		require.Contains(t, err.Error(), "does not exist")

		// With them, it passes.
		require.NoError(t, v.ValidateBootstrap(t.Context(), bs))
	})

	t.Run("TLS files only the proxy pod has are rejected", func(t *testing.T) {
		// Envoy opens each TLS file, and the validator does not have them.
		bs := parseTestBootstrap(t, fileTLSBootstrapYAML)
		err := v.ValidateBootstrap(t.Context(), bs)
		require.ErrorIs(t, err, ErrInvalidBootstrap)
		assert.Contains(t, err.Error(), "/proxy/only/")
	})

	t.Run("an invalid log format is rejected", func(t *testing.T) {
		bs := parseTestBootstrap(t, `{application_log_config: {log_format: {json_format: {message: "%v"}}}}`)
		err := v.ValidateBootstrap(t.Context(), bs)
		require.ErrorIs(t, err, ErrInvalidBootstrap)
		assert.Contains(t, err.Error(), "%v")
	})

	t.Run("the reason is readable with a log format that leaves it out", func(t *testing.T) {
		// Validate a bootstrap with duplicate clusters, whose log format writes only an
		// access log operator that application logs do not have.
		bs := parseTestBootstrap(t, `{application_log_config: {log_format: {json_format: {message: "%LOCAL_REPLY_BODY%"}}}, static_resources: {clusters: [{name: dup}, {name: dup}]}}`)
		err := v.ValidateBootstrap(t.Context(), bs)

		// Envoy's reason is reported.
		require.ErrorIs(t, err, ErrInvalidBootstrap)
		assert.Contains(t, err.Error(), "duplicate cluster")
	})
}

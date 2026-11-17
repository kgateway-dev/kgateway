package validator

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	envoybootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	eiutils "github.com/kgateway-dev/kgateway/v2/internal/envoyinit/pkg/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/bootstrappatch"
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
	bs, err := bootstrappatch.ParseBootstrap([]byte(y))
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
	// dir. The two bootstraps have the same structure, so their paths pair up in walk
	// order.
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

func TestBinaryValidator_ValidateBootstrap(t *testing.T) {
	tests := []struct {
		name      string
		bootstrap string
		script    string
		// wantErr is the rejection; empty when Envoy accepts the bootstrap.
		wantErr string
		guards  []string
	}{
		{
			name: "accepted, reporting removed runtime guards",
			script: `#!/bin/sh
echo "[error] envoy bug failure: false. Details: Using a removed guard envoy.reloadable_features.b. In future version of Envoy this will be treated as invalid configuration" >&2
echo '{"msg":"envoy bug failure: false. Details: Using a removed guard envoy.reloadable_features.a. In future version of Envoy this will be treated as invalid configuration"}' >&2
echo "[error] envoy bug failure: false. Details: Using a removed guard envoy.reloadable_features.b. In future version of Envoy this will be treated as invalid configuration" >&2
exit 0
`,
			guards: []string{"envoy.reloadable_features.a", "envoy.reloadable_features.b"},
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
			name: "runs at error level with a bare log format",
			script: `#!/bin/sh
echo "$@" >&2
exit 1
`,
			wantErr: "Envoy rejected the bootstrap: --mode validate --config-path /dev/fd/0 -l error --log-format %v",
		},
		{
			name:      "keeps the proxy's log format",
			bootstrap: `{application_log_config: {log_format: {text_format: "%v"}}}`,
			script: `#!/bin/sh
echo "$@" >&2
exit 1
`,
			wantErr: "Envoy rejected the bootstrap: --mode validate --config-path /dev/fd/0 -l error",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Validate with a mock Envoy.
			bs := &envoybootstrapv3.Bootstrap{}
			if tt.bootstrap != "" {
				bs = parseTestBootstrap(t, tt.bootstrap)
			}
			result, err := NewBinary(createMockBinary(t, tt.script)).(BootstrapValidator).ValidateBootstrap(t.Context(), bs)

			// Check the verdict.
			if tt.wantErr == "" {
				require.NoError(t, err)
				assert.Equal(t, tt.guards, result.RemovedRuntimeGuards)
				return
			}
			require.ErrorIs(t, err, ErrInvalidBootstrap)
			assert.Equal(t, tt.wantErr, err.Error())
		})
	}

	t.Run("Envoy cannot be run", func(t *testing.T) {
		_, err := NewBinary("/nonexistent/envoy").(BootstrapValidator).ValidateBootstrap(t.Context(), &envoybootstrapv3.Bootstrap{})
		require.Error(t, err)
		assert.NotErrorIs(t, err, ErrInvalidBootstrap, "a failure to run Envoy is not a verdict on the bootstrap")
	})
}

// stubBootstrapValidator is an inner validator that counts proxy bootstrap
// validations and returns a programmed response.
type stubBootstrapValidator struct {
	stubValidator
	bootstrapCalls int
	result         BootstrapResult
	bootstrapErr   error
}

func (s *stubBootstrapValidator) ValidateBootstrap(context.Context, *envoybootstrapv3.Bootstrap) (BootstrapResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bootstrapCalls++
	return s.result, s.bootstrapErr
}

func TestCachingValidator_ValidateBootstrap(t *testing.T) {
	t.Run("memoizes acceptance with its removed guards", func(t *testing.T) {
		// Validate the same content twice and different content once.
		stub := &stubBootstrapValidator{result: BootstrapResult{RemovedRuntimeGuards: []string{"envoy.reloadable_features.a"}}}
		v := NewCaching(stub, 0).(BootstrapValidator)
		for range 2 {
			result, err := v.ValidateBootstrap(t.Context(), bootstrapForNode("a"))
			require.NoError(t, err)
			assert.Equal(t, []string{"envoy.reloadable_features.a"}, result.RemovedRuntimeGuards)
		}
		_, err := v.ValidateBootstrap(t.Context(), bootstrapForNode("b"))
		require.NoError(t, err)

		// Envoy ran once per content, and xDS validation was not used.
		assert.Equal(t, 2, stub.bootstrapCalls)
		assert.Equal(t, 0, stub.Calls())
	})

	t.Run("memoizes rejection", func(t *testing.T) {
		// Validate a rejected bootstrap twice.
		stub := &stubBootstrapValidator{bootstrapErr: errors.Join(ErrInvalidBootstrap, errors.New("bad"))}
		v := NewCaching(stub, 0).(BootstrapValidator)
		_, err1 := v.ValidateBootstrap(t.Context(), bootstrapForNode("a"))
		_, err2 := v.ValidateBootstrap(t.Context(), bootstrapForNode("a"))

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
			_, err := v.ValidateBootstrap(t.Context(), bootstrapForNode("a"))
			require.Error(t, err)
		}
		assert.Equal(t, 2, stub.bootstrapCalls)
	})

	t.Run("refuses when the inner validator cannot validate bootstraps", func(t *testing.T) {
		_, err := NewCaching(&stubValidator{}, 0).(BootstrapValidator).ValidateBootstrap(t.Context(), bootstrapForNode("a"))
		require.EqualError(t, err, "the configured validator cannot validate proxy bootstraps")
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
		result, err := v.ValidateBootstrap(t.Context(), bs)
		require.NoError(t, err)
		assert.Empty(t, result.RemovedRuntimeGuards)
	})

	t.Run("an invalid log format is rejected", func(t *testing.T) {
		bs := parseTestBootstrap(t, `{application_log_config: {log_format: {json_format: {message: "%v"}}}}`)
		_, err := v.ValidateBootstrap(t.Context(), bs)
		require.ErrorIs(t, err, ErrInvalidBootstrap)
		assert.Contains(t, err.Error(), "%v")
	})

	t.Run("a removed runtime guard is reported and accepted", func(t *testing.T) {
		// If a future Envoy rejects this guard outright, switch to a guard removed in
		// that Envoy version; see TestDockerValidator_FailOnWarnings.
		bs := parseTestBootstrap(t, `
layered_runtime:
  layers:
  - name: static_layer
    static_layer:
      envoy.restart_features.use_eds_cache_for_ads: true
`)
		result, err := v.ValidateBootstrap(t.Context(), bs)
		require.NoError(t, err)
		assert.Equal(t, []string{"envoy.restart_features.use_eds_cache_for_ads"}, result.RemovedRuntimeGuards)
	})
}

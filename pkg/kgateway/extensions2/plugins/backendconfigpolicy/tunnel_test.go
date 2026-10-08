package backendconfigpolicy

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/kube/krt/krttest"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1b1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
	apifake "github.com/kgateway-dev/kgateway/v2/pkg/apiclient/fake"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	"github.com/kgateway-dev/kgateway/v2/test/testutils/equalstest"
)

// newTunnelSecretIndex builds a synced Secret index with permissive reference grants.
func newTunnelSecretIndex(t *testing.T, secrets ...*corev1.Secret) *krtcollections.SecretIndex {
	t.Helper()
	objs := make([]any, 0, len(secrets))
	for _, s := range secrets {
		objs = append(objs, s)
	}
	mock := krttest.NewMock(t, objs)
	secretCol := krttest.GetMockCollection[*corev1.Secret](mock)
	refGrantCol := krttest.GetMockCollection[*gwv1b1.ReferenceGrant](mock)
	refGrants := krtcollections.NewRefGrantIndex(refGrantCol, apisettings.ReferenceGrantPermissive)
	secretCols := map[schema.GroupKind]krt.Collection[ir.Secret]{
		corev1.SchemeGroupVersion.WithKind("Secret").GroupKind(): krt.NewCollection(secretCol, func(_ krt.HandlerContext, s *corev1.Secret) *ir.Secret {
			return &ir.Secret{
				ObjectSource: ir.ObjectSource{Kind: "Secret", Namespace: s.Namespace, Name: s.Name},
				Obj:          s,
				Data:         s.Data,
			}
		}),
	}
	index := krtcollections.NewSecretIndex(secretCols, refGrants)
	secretCol.WaitUntilSynced(nil)
	require.Eventually(t, index.HasSynced, 5*time.Second, 10*time.Millisecond, "secret index should sync")
	return index
}

var tunnelPolicySource = ir.ObjectSource{
	Group:     "gateway.kgateway.dev",
	Kind:      "BackendConfigPolicy",
	Namespace: "default",
	Name:      "tunnel",
}

// TestTranslateTunnel covers ordered inline and Secret headers, plus missing-Secret errors.
func TestTranslateTunnel(t *testing.T) {
	secrets := newTunnelSecretIndex(t, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "proxy-creds", Namespace: "default"},
		Data:       map[string][]byte{"authorization": []byte("Basic dXNlcjpwYXNz")},
	})
	proxy := gwv1.BackendObjectReference{
		Name:      "egress-proxy",
		Namespace: new(gwv1.Namespace("egress")),
		Port:      new(gwv1.PortNumber(3128)),
	}

	tests := []struct {
		name    string
		spec    *kgateway.Tunnel
		want    *tunnelIR
		wantErr string
	}{
		{
			name: "inline and secret headers keep declaration order",
			spec: &kgateway.Tunnel{
				Proxy: kgateway.TunnelProxy{BackendRef: proxy},
				Headers: []shared.HTTPHeader{
					{Name: new(gwv1.HTTPHeaderName("X-Proxy-Client")), Value: new("gateway")},
					{
						Name:      new(gwv1.HTTPHeaderName("Proxy-Authorization")),
						SecretRef: &shared.SecretRefWithKey{Name: "proxy-creds", Key: new("authorization")},
					},
				},
			},
			want: &tunnelIR{
				source: tunnelPolicySource,
				proxy:  proxy,
				headers: []gwv1.HTTPHeader{
					{Name: "X-Proxy-Client", Value: "gateway"},
					{Name: "Proxy-Authorization", Value: "Basic dXNlcjpwYXNz"},
				},
			},
		},
		{
			name: "missing secret",
			spec: &kgateway.Tunnel{
				Proxy: kgateway.TunnelProxy{BackendRef: proxy},
				Headers: []shared.HTTPHeader{{
					Name:      new(gwv1.HTTPHeaderName("Proxy-Authorization")),
					SecretRef: &shared.SecretRefWithKey{Name: "missing"},
				}},
			},
			want:    &tunnelIR{source: tunnelPolicySource, proxy: proxy},
			wantErr: "tunnel headers",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := translateTunnel(krt.TestingDummyContext{}, secrets, "default", "tunnel", tt.spec)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			require.NotNil(t, got, "an invalid tunnel must not read as absent")
			assert.True(t, tt.want.Equals(got), "got %+v, want %+v", got, tt.want)
		})
	}
}

// TestNormalizeTunnelHeaders checks newline normalization and credential-safe errors.
func TestNormalizeTunnelHeaders(t *testing.T) {
	// Detect credential values leaking into errors.
	const marker = "c2VjcmV0"
	tests := []struct {
		name    string
		value   string
		want    string
		wantErr string
	}{
		{name: "drops a terminal LF", value: "Basic " + marker + "\n", want: "Basic " + marker},
		{name: "rejects a non-printable character", value: "Basic\x01" + marker, wantErr: "printable ASCII"},
		{name: "rejects an empty value", value: "", wantErr: "empty value"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headers := []gwv1.HTTPHeader{{Name: "Proxy-Authorization", Value: tt.value}}
			err := normalizeTunnelHeaders(headers)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				assert.Contains(t, err.Error(), "Proxy-Authorization", "the error must name the header")
				assert.NotContains(t, err.Error(), marker, "the error must not include the value")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, headers[0].Value)
		})
	}
}

func baseHarnessTunnel() *tunnelIR {
	return &tunnelIR{
		source:  tunnelPolicySource,
		proxy:   gwv1.BackendObjectReference{Name: "egress-proxy", Port: new(gwv1.PortNumber(3128))},
		headers: []gwv1.HTTPHeader{{Name: "Proxy-Authorization", Value: "Basic dXNlcjpwYXNz"}},
	}
}

// TestHarnessTunnelIREquals checks change detection for the source, proxy, and headers.
func TestHarnessTunnelIREquals(t *testing.T) {
	cases := []equalstest.Case[*tunnelIR]{
		{Field: "source", Mutate: func(p **tunnelIR) { (*p).source.Namespace = "other" }},
		{Field: "proxy", Mutate: func(p **tunnelIR) { (*p).proxy.Name = "other-proxy" }},
		{Field: "proxy", Mutate: func(p **tunnelIR) { (*p).proxy.Namespace = new(gwv1.Namespace("egress")) }},
		{Field: "headers", Mutate: func(p **tunnelIR) { (*p).headers[0].Value = "Basic cm90YXRlZA==" }},
		{Field: "headers", Mutate: func(p **tunnelIR) { (*p).headers = nil }},
	}
	equalstest.Run(
		t,
		baseHarnessTunnel,
		func(a, b *tunnelIR) bool { return a.Equals(b) },
		cases,
		nil,
		equalstest.IncludeUnexported(),
	)
}

// TestKrtDebugRedactsInlineTunnelHeaders checks that policy dumps hide credentials
// in both the current spec and the kubectl apply annotation.
func TestKrtDebugRedactsInlineTunnelHeaders(t *testing.T) {
	const inlineValue = "Basic aW5saW5lOnBhc3M="
	ctx := t.Context()

	fakeClient := apifake.NewClient(t, &kgateway.BackendConfigPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "external-egress",
			Namespace: "default",
			Annotations: map[string]string{
				corev1.LastAppliedConfigAnnotation: `{"spec":{"tunnel":{"headers":[{"name":"Proxy-Authorization","value":"` + inlineValue + `"}]}}}`,
			},
		},
		Spec: kgateway.BackendConfigPolicySpec{
			TargetRefs: []shared.LocalPolicyTargetReference{{Group: "gateway.kgateway.dev", Kind: "Backend", Name: "external"}},
			Tunnel: &kgateway.Tunnel{
				Proxy: kgateway.TunnelProxy{BackendRef: gwv1.BackendObjectReference{
					Name: "egress-proxy",
					Port: new(gwv1.PortNumber(3128)),
				}},
				Headers: []shared.HTTPHeader{{
					Name:  new(gwv1.HTTPHeaderName("Proxy-Authorization")),
					Value: new(inlineValue),
				}},
			},
		},
	})

	debugger := new(krt.DebugHandler)
	settings := apisettings.Settings{}
	commoncol, err := collections.NewCommonCollections(
		ctx, krtutil.NewKrtOptions(ctx.Done(), debugger), fakeClient, wellknown.DefaultGatewayControllerName, settings,
	)
	require.NoError(t, err)
	commoncol.InitPlugins(ctx, NewPlugin(ctx, commoncol, nil), settings)
	fakeClient.RunAndWait(ctx.Done())

	// Avoid printing credentials on assertion failure.
	var dump string
	require.Eventually(t, func() bool {
		out, err := json.Marshal(debugger)
		if err != nil {
			return false
		}
		dump = string(out)
		return strings.Contains(dump, `"external-egress"`)
	}, 10*time.Second, 50*time.Millisecond, "the policy must stay visible in KRT debug output")
	assert.False(t, strings.Contains(dump, inlineValue), "inline tunnel header values must be redacted from KRT debug output")
	assert.True(t, strings.Contains(dump, redactedValue), "the redacted header must stay visible in KRT debug output")
}

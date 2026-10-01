package krtcollections

import (
	"errors"
	"slices"
	"testing"

	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/kube/krt/krttest"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1b1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

var (
	// sourceGK is the identity the reference is resolved under, and otherGK a kind
	// that is not it.
	sourceGK = schema.GroupKind{Group: "gateway.kgateway.dev", Kind: "TrafficPolicy"}
	otherGK  = schema.GroupKind{Group: "example.io", Kind: "OtherPolicy"}

	secretGK = corev1.SchemeGroupVersion.WithKind("Secret").GroupKind()
)

// secretRefGrant builds a ReferenceGrant in ns permitting references to Secrets from
// fromGK in fromNs.
func secretRefGrant(ns string, fromGK schema.GroupKind, fromNs string) *gwv1b1.ReferenceGrant {
	return &gwv1b1.ReferenceGrant{
		ObjectMeta: metav1.ObjectMeta{Name: "grant", Namespace: ns},
		Spec: gwv1b1.ReferenceGrantSpec{
			From: []gwv1b1.ReferenceGrantFrom{{
				Group:     gwv1.Group(fromGK.Group),
				Kind:      gwv1.Kind(fromGK.Kind),
				Namespace: gwv1.Namespace(fromNs),
			}},
			To: []gwv1b1.ReferenceGrantTo{{Group: "", Kind: "Secret"}},
		},
	}
}

func newTestSecretIndex(t *testing.T, objs ...any) *SecretIndex {
	t.Helper()
	return newTestSecretIndexWithMode(t, apisettings.ReferenceGrantPermissive, objs...)
}

func newTestSecretIndexWithMode(t *testing.T, mode apisettings.ReferenceGrantMode, objs ...any) *SecretIndex {
	t.Helper()
	mock := krttest.NewMock(t, objs)
	secretCol := krttest.GetMockCollection[*corev1.Secret](mock)
	refgrants := NewRefGrantIndex(krttest.GetMockCollection[*gwv1b1.ReferenceGrant](mock), mode)
	secretsCol := map[schema.GroupKind]krt.Collection[ir.Secret]{
		secretGK: krt.NewCollection(secretCol, func(kctx krt.HandlerContext, i *corev1.Secret) *ir.Secret {
			return &ir.Secret{
				ObjectSource: ir.ObjectSource{Kind: "Secret", Namespace: i.Namespace, Name: i.Name},
				Obj:          i,
				Data:         i.Data,
			}
		}),
	}
	idx := NewSecretIndex(secretsCol, refgrants)
	secretCol.WaitUntilSynced(nil)
	for !idx.HasSynced() {
	}
	return idx
}

func testSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "api-keys",
			Namespace: "secrets-ns",
			Labels:    map[string]string{"app": "keys"},
		},
		Data: map[string][]byte{"user": []byte("k1")},
	}
}

// TestSecretIndexReferenceGrantSourceIdentity pins that both secret paths permit a
// reference only when a grant names the identity in From - a grant naming any other
// kind stays inert, since from.kind is what scopes the permission.
func TestSecretIndexReferenceGrantSourceIdentity(t *testing.T) {
	tests := []struct {
		name    string
		grants  []any
		allowed bool
	}{
		{
			name:    "grant names the source identity",
			grants:  []any{secretRefGrant("secrets-ns", sourceGK, "app-ns")},
			allowed: true,
		},
		{
			name:   "grant names another kind",
			grants: []any{secretRefGrant("secrets-ns", otherGK, "app-ns")},
		},
		{
			name: "no grant",
		},
		{
			name:   "grant sits in the referrer namespace instead of the referent one",
			grants: []any{secretRefGrant("app-ns", sourceGK, "app-ns")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			idx := newTestSecretIndex(t, append([]any{testSecret()}, tt.grants...)...)
			from := From{GroupKind: sourceGK, Namespace: "app-ns"}
			krtctx := krt.TestingDummyContext{}

			// secretRef, as spec.basicAuth.secretRef and spec.apiKeyAuth.secretRef resolve it.
			ns := gwv1.Namespace("secrets-ns")
			got, err := idx.GetSecret(krtctx, from, gwv1.SecretObjectReference{Name: "api-keys", Namespace: &ns})
			switch {
			case tt.allowed && err != nil:
				t.Fatalf("GetSecret() = %v, want the reference to be permitted", err)
			case tt.allowed && got.Name != "api-keys":
				t.Errorf("GetSecret() returned secret %q, want %q", got.Name, "api-keys")
			case !tt.allowed && !errors.Is(err, ErrMissingReferenceGrant):
				t.Fatalf("GetSecret() = %v, want a missing reference grant error", err)
			}

			// secretSelector, as spec.apiKeyAuth.secretSelector resolves it.
			secrets, err := idx.GetSecretsBySelector(krtctx, from, secretGK, map[string]string{"app": "keys"})
			want := 0
			if tt.allowed {
				want = 1
			}
			if err != nil {
				t.Fatalf("GetSecretsBySelector() = %v, want no error", err)
			}
			if len(secrets) != want {
				t.Errorf("GetSecretsBySelector() returned %d secrets, want %d", len(secrets), want)
			}
		})
	}
}

// TestSecretsBySelectorDeniedMatchReadsAsNoMatch pins that a selector whose only
// match sits in a namespace with no grant gives the same result as a selector that
// matches nothing. Which secrets carry a label is not observable without a grant, so
// if the two differed, a referrer could probe labels to learn that a secret exists in
// a namespace that never granted it access.
func TestSecretsBySelectorDeniedMatchReadsAsNoMatch(t *testing.T) {
	from := From{GroupKind: sourceGK, Namespace: "app-ns"}
	selector := map[string]string{"app": "keys"}

	// A matching secret exists, but in a namespace with no grant.
	denied, errDenied := newTestSecretIndex(t, testSecret()).
		GetSecretsBySelector(krt.TestingDummyContext{}, from, secretGK, selector)
	// No secret carries the labels anywhere.
	absent, errAbsent := newTestSecretIndex(t).
		GetSecretsBySelector(krt.TestingDummyContext{}, from, secretGK, selector)

	if errDenied != nil || errAbsent != nil {
		t.Fatalf("GetSecretsBySelector() errors = (denied: %v, absent: %v), want none for either", errDenied, errAbsent)
	}
	if len(denied) != 0 || len(absent) != 0 {
		t.Fatalf("GetSecretsBySelector() returned (denied: %d, absent: %d) secrets, want none for either", len(denied), len(absent))
	}
}

// TestSecretsBySelectorSearchScope covers which namespaces a selector reaches: the
// referrer's own, those granting it access (limited to the named secrets when the
// grant names any), and every namespace when grants are not enforced.
func TestSecretsBySelectorSearchScope(t *testing.T) {
	secretIn := func(ns, name string) *corev1.Secret {
		sec := testSecret()
		sec.Namespace, sec.Name = ns, name
		return sec
	}
	namedGrant := secretRefGrant("secrets-ns", sourceGK, "app-ns")
	namedGrant.Spec.To[0].Name = ptr.To(gwv1.ObjectName("allowed"))

	tests := []struct {
		name string
		mode apisettings.ReferenceGrantMode
		objs []any
		want []string
	}{
		{
			name: "own namespace needs no grant",
			objs: []any{secretIn("app-ns", "local"), secretIn("secrets-ns", "remote")},
			want: []string{"app-ns/local"},
		},
		{
			name: "granting namespace is searched alongside the own one",
			objs: []any{
				secretIn("app-ns", "local"), secretIn("secrets-ns", "remote"), secretIn("other-ns", "ungranted"),
				secretRefGrant("secrets-ns", sourceGK, "app-ns"),
			},
			want: []string{"app-ns/local", "secrets-ns/remote"},
		},
		{
			name: "grant limited to a name permits only that secret",
			objs: []any{secretIn("secrets-ns", "allowed"), secretIn("secrets-ns", "other"), namedGrant},
			want: []string{"secrets-ns/allowed"},
		},
		{
			name: "grant from another namespace does not apply",
			objs: []any{secretIn("secrets-ns", "remote"), secretRefGrant("secrets-ns", sourceGK, "elsewhere-ns")},
		},
		{
			name: "grants not enforced searches every namespace",
			mode: apisettings.ReferenceGrantOff,
			objs: []any{secretIn("secrets-ns", "remote"), secretIn("other-ns", "ungranted")},
			want: []string{"other-ns/ungranted", "secrets-ns/remote"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mode := tt.mode
			if mode == "" {
				mode = apisettings.ReferenceGrantPermissive
			}
			idx := newTestSecretIndexWithMode(t, mode, tt.objs...)
			secrets, err := idx.GetSecretsBySelector(krt.TestingDummyContext{},
				From{GroupKind: sourceGK, Namespace: "app-ns"}, secretGK, map[string]string{"app": "keys"})
			if err != nil {
				t.Fatalf("GetSecretsBySelector() = %v, want no error", err)
			}
			var got []string
			for _, sec := range secrets {
				got = append(got, sec.Namespace+"/"+sec.Name)
			}
			slices.Sort(got)
			if !slices.Equal(got, tt.want) {
				t.Errorf("GetSecretsBySelector() = %v, want %v", got, tt.want)
			}
		})
	}
}

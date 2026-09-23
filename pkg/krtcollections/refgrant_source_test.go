package krtcollections

import (
	"errors"
	"slices"
	"strings"
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
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
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
			switch {
			case tt.allowed && err != nil:
				t.Fatalf("GetSecretsBySelector() = %v, want the reference to be permitted", err)
			case tt.allowed && len(secrets) != 1:
				t.Errorf("GetSecretsBySelector() returned %d secrets, want 1", len(secrets))
			case !tt.allowed && !errors.As(err, new(*SelectorNoMatchError)):
				t.Fatalf("GetSecretsBySelector() = %v, want a selector no-match error", err)
			case !tt.allowed && len(secrets) != 0:
				t.Errorf("GetSecretsBySelector() returned %d secrets, want none", len(secrets))
			}
		})
	}
}

// TestMissingReferenceGrantErrorNamesTheGrant covers the message for a reference that
// names its referent: the user wrote that name, so repeating it discloses nothing, and
// it is what makes the grant to create readable off the policy status.
func TestMissingReferenceGrantErrorNamesTheGrant(t *testing.T) {
	idx := newTestSecretIndex(t, testSecret())

	ns := gwv1.Namespace("secrets-ns")
	_, err := idx.GetSecret(krt.TestingDummyContext{}, From{GroupKind: sourceGK, Namespace: "app-ns"},
		gwv1.SecretObjectReference{Name: "api-keys", Namespace: &ns})
	if !errors.Is(err, ErrMissingReferenceGrant) {
		t.Fatalf("GetSecret() = %v, want a missing reference grant error", err)
	}
	for _, want := range []string{`namespace "secrets-ns"`, `Secret "api-keys"`, `kind "TrafficPolicy"`, `namespace "app-ns"`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("GetSecret() error = %q, want it to mention %s", err, want)
		}
	}
}

// TestSecretsBySelectorErrorOmitsMatchedSecrets pins that a denied selector says
// nothing about what it matched, down to whether it matched anything. Which secrets
// carry a label is not observable without a grant, so if a match in an ungranted
// namespace read differently from no match at all, a referrer could probe labels to
// learn that a secret exists in a namespace that never granted it access.
func TestSecretsBySelectorErrorOmitsMatchedSecrets(t *testing.T) {
	from := From{GroupKind: sourceGK, Namespace: "app-ns"}
	selector := map[string]string{"app": "keys"}

	// A matching secret exists, but in a namespace with no grant.
	_, errDenied := newTestSecretIndex(t, testSecret()).
		GetSecretsBySelector(krt.TestingDummyContext{}, from, secretGK, selector)
	// No secret carries the labels anywhere.
	_, errAbsent := newTestSecretIndex(t).
		GetSecretsBySelector(krt.TestingDummyContext{}, from, secretGK, selector)

	for name, err := range map[string]error{"denied": errDenied, "absent": errAbsent} {
		if !errors.As(err, new(*SelectorNoMatchError)) {
			t.Fatalf("%s: GetSecretsBySelector() = %v, want a selector no-match error", name, err)
		}
		if errors.Is(err, ErrMissingReferenceGrant) {
			t.Errorf("%s: GetSecretsBySelector() = %v, want it not to claim a missing grant, which would reveal a match", name, err)
		}
	}
	if errDenied.Error() != errAbsent.Error() {
		t.Fatalf("a denied match and no match read differently:\n  denied: %s\n  absent: %s", errDenied, errAbsent)
	}
	for _, leaked := range []string{"api-keys", "secrets-ns"} {
		if strings.Contains(errDenied.Error(), leaked) {
			t.Errorf("GetSecretsBySelector() error = %q, want it to disclose nothing about the matched secrets, but it names %q", errDenied, leaked)
		}
	}
	// The source side and the selector are entirely user-authored, so they stay in the
	// message: they say which grant would widen the search.
	for _, want := range []string{`"app=keys"`, `kind "TrafficPolicy"`, `namespace "app-ns"`} {
		if !strings.Contains(errDenied.Error(), want) {
			t.Errorf("GetSecretsBySelector() error = %q, want it to mention %s", errDenied, want)
		}
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
			if len(tt.want) == 0 {
				if !errors.As(err, new(*SelectorNoMatchError)) {
					t.Fatalf("GetSecretsBySelector() = %v, want a selector no-match error", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("GetSecretsBySelector() = %v, want no error", err)
			}
			var got []string
			for _, sec := range secrets {
				got = append(got, sec.Namespace+"/"+sec.Name)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("GetSecretsBySelector() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestSelectorNoMatchErrorAnyNamespace pins that with grants not enforced the message
// does not point at a ReferenceGrant, which would change nothing.
func TestSelectorNoMatchErrorAnyNamespace(t *testing.T) {
	idx := newTestSecretIndexWithMode(t, apisettings.ReferenceGrantOff)
	_, err := idx.GetSecretsBySelector(krt.TestingDummyContext{},
		From{GroupKind: sourceGK, Namespace: "app-ns"}, secretGK, map[string]string{"app": "keys"})
	if want := `no Secrets matching "app=keys" in any namespace`; err == nil || err.Error() != want {
		t.Fatalf("GetSecretsBySelector() = %v, want %q", err, want)
	}
}

// TestBackendRefMissingReferenceGrantNamesTheGrant covers backend refs, such as a
// GatewayExtension's: the message names the grant to create, including the kind that
// holds the reference, rather than a bare "missing reference grant".
func TestBackendRefMissingReferenceGrantNamesTheGrant(t *testing.T) {
	mock := krttest.NewMock(t, nil)
	refgrants := NewRefGrantIndex(krttest.GetMockCollection[*gwv1b1.ReferenceGrant](mock), apisettings.ReferenceGrantPermissive)
	backends := NewBackendIndex(krtutil.KrtOptions{}, nil, refgrants)

	src := ir.ObjectSource{Group: "gateway.kgateway.dev", Kind: "GatewayExtension", Namespace: "app-ns", Name: "ext-auth"}
	ns := gwv1.Namespace("svc-ns")
	_, err := backends.GetBackendFromRef(krt.TestingDummyContext{}, src, gwv1.BackendObjectReference{Name: "auth-svc", Namespace: &ns})

	var grantErr *MissingReferenceGrantError
	if !errors.As(err, &grantErr) {
		t.Fatalf("GetBackendFromRef() = %v, want a MissingReferenceGrantError", err)
	}
	for _, want := range []string{`namespace "svc-ns"`, `Service "auth-svc"`, `kind "GatewayExtension"`, `namespace "app-ns"`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("GetBackendFromRef() error = %q, want it to mention %s", err, want)
		}
	}
}

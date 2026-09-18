package krtcollections

import (
	"errors"
	"strings"
	"testing"

	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/kube/krt/krttest"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
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
	mock := krttest.NewMock(t, objs)
	secretCol := krttest.GetMockCollection[*corev1.Secret](mock)
	refgrants := NewRefGrantIndex(krttest.GetMockCollection[*gwv1b1.ReferenceGrant](mock), apisettings.ReferenceGrantPermissive)
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
			case !tt.allowed && !errors.Is(err, ErrMissingReferenceGrant):
				t.Fatalf("GetSecretsBySelector() = %v, want a missing reference grant error", err)
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
// nothing about what it matched. Which secrets carry a label is not observable without
// a grant, so naming one in policy status would let a referrer probe labels to learn
// that a secret exists in a namespace that never granted it access.
func TestSecretsBySelectorErrorOmitsMatchedSecrets(t *testing.T) {
	idx := newTestSecretIndex(t, testSecret())

	secrets, err := idx.GetSecretsBySelector(krt.TestingDummyContext{},
		From{GroupKind: sourceGK, Namespace: "app-ns"}, secretGK, map[string]string{"app": "keys"})
	if !errors.Is(err, ErrMissingReferenceGrant) {
		t.Fatalf("GetSecretsBySelector() = %v, want a missing reference grant error", err)
	}
	if len(secrets) != 0 {
		t.Fatalf("GetSecretsBySelector() returned %d secrets, want none", len(secrets))
	}
	for _, leaked := range []string{"api-keys", "secrets-ns"} {
		if strings.Contains(err.Error(), leaked) {
			t.Errorf("GetSecretsBySelector() error = %q, want it to disclose nothing about the matched secrets, but it names %q", err, leaked)
		}
	}
	// The source side is entirely user-authored, so it stays in the message.
	for _, want := range []string{`kind "TrafficPolicy"`, `namespace "app-ns"`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("GetSecretsBySelector() error = %q, want it to mention %s", err, want)
		}
	}
}

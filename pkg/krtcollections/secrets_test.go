package krtcollections

import (
	"slices"
	"strings"
	"testing"

	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/kube/krt/krttest"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1b1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func TestSecretIndex_GetSecretWithoutRefGrant(t *testing.T) {
	tests := []struct {
		name           string
		secretName     string
		ns             string
		secrets        []*corev1.Secret
		refGrants      []any
		wantErr        bool
		wantErrMsg     string
		expectedSecret *ir.Secret
	}{
		{
			name:       "success - secret found in same namespace",
			secretName: "my-secret",
			ns:         "default",
			secrets: []*corev1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-secret",
						Namespace: "default",
					},
					Data: map[string][]byte{
						"key1": []byte("value1"),
					},
				},
			},
			wantErr: false,
			expectedSecret: &ir.Secret{
				ObjectSource: ir.ObjectSource{
					Group:     "",
					Kind:      "Secret",
					Namespace: "default",
					Name:      "my-secret",
				},
				Data: map[string][]byte{
					"key1": []byte("value1"),
				},
			},
		},
		{
			name:       "error - secret not found",
			secretName: "missing-secret",
			ns:         "default",
			secrets: []*corev1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "other-secret",
						Namespace: "default",
					},
					Data: map[string][]byte{
						"key1": []byte("value1"),
					},
				},
			},
			wantErr:    true,
			wantErrMsg: "not found",
		},
		{
			name:       "error - secret not found when looking in wrong namespace",
			secretName: "my-secret",
			ns:         "other-ns",
			secrets: []*corev1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-secret",
						Namespace: "default", // Secret is in default, but we're looking in other-ns
					},
					Data: map[string][]byte{
						"key1": []byte("value1"),
					},
				},
			},
			wantErr:    true,
			wantErrMsg: "not found",
		},
		{
			name:       "success - multiple secrets in same namespace",
			secretName: "target-secret",
			ns:         "default",
			secrets: []*corev1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "other-secret",
						Namespace: "default",
					},
					Data: map[string][]byte{
						"key1": []byte("value1"),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "target-secret",
						Namespace: "default",
					},
					Data: map[string][]byte{
						"key1": []byte("value1"),
						"key2": []byte("value2"),
					},
				},
			},
			wantErr: false,
			expectedSecret: &ir.Secret{
				ObjectSource: ir.ObjectSource{
					Group:     "",
					Kind:      "Secret",
					Namespace: "default",
					Name:      "target-secret",
				},
				Data: map[string][]byte{
					"key1": []byte("value1"),
					"key2": []byte("value2"),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup test objects
			var initObjs []any
			for _, secret := range tt.secrets {
				initObjs = append(initObjs, secret)
			}
			initObjs = append(initObjs, tt.refGrants...)

			// Create mock KRT context
			mock := krttest.NewMock(t, initObjs)
			secretCol := krttest.GetMockCollection[*corev1.Secret](mock)

			// Create SecretIndex
			// ReferenceGrants are not needed for same-namespace lookups, but we still need to create the index
			// Import the correct type for ReferenceGrant
			refGrantCol := krttest.GetMockCollection[*gwv1b1.ReferenceGrant](mock)
			refgrants := NewRefGrantIndex(refGrantCol, apisettings.ReferenceGrantPermissive)
			secretsCol := map[schema.GroupKind]krt.Collection[ir.Secret]{
				corev1.SchemeGroupVersion.WithKind("Secret").GroupKind(): krt.NewCollection(secretCol, func(kctx krt.HandlerContext, i *corev1.Secret) *ir.Secret {
					return &ir.Secret{
						ObjectSource: ir.ObjectSource{
							Group:     "",
							Kind:      "Secret",
							Namespace: i.Namespace,
							Name:      i.Name,
						},
						Obj:  i,
						Data: i.Data,
					}
				}),
			}
			secretIndex := NewSecretIndex(secretsCol, refgrants)

			// Wait for collections to sync
			secretCol.WaitUntilSynced(nil)

			// Wait for SecretIndex to be synced
			for !secretIndex.HasSynced() {
				// Poll until synced
			}

			// Create handler context
			krtctx := krt.TestingDummyContext{}

			// Call GetSecretWithoutRefGrant
			result, err := secretIndex.GetSecretWithoutRefGrant(krtctx, tt.secretName, tt.ns)

			// Verify results
			if tt.wantErr {
				if err == nil {
					t.Errorf("GetSecretWithoutRefGrant() expected error but got none")
					return
				}
				if tt.wantErrMsg != "" && !strings.Contains(err.Error(), tt.wantErrMsg) {
					t.Errorf("GetSecretWithoutRefGrant() error = %v, want error containing %q", err, tt.wantErrMsg)
				}
				return
			}

			if err != nil {
				t.Errorf("GetSecretWithoutRefGrant() unexpected error = %v", err)
				return
			}

			if result == nil {
				t.Errorf("GetSecretWithoutRefGrant() returned nil secret")
				return
			}

			// Verify secret details
			if result.Name != tt.expectedSecret.Name {
				t.Errorf("GetSecretWithoutRefGrant() secret.Name = %v, want %v", result.Name, tt.expectedSecret.Name)
			}
			if result.Namespace != tt.expectedSecret.Namespace {
				t.Errorf("GetSecretWithoutRefGrant() secret.Namespace = %v, want %v", result.Namespace, tt.expectedSecret.Namespace)
			}
			if result.Kind != tt.expectedSecret.Kind {
				t.Errorf("GetSecretWithoutRefGrant() secret.Kind = %v, want %v", result.Kind, tt.expectedSecret.Kind)
			}

			// Verify secret data
			if len(result.Data) != len(tt.expectedSecret.Data) {
				t.Errorf("GetSecretWithoutRefGrant() secret.Data length = %v, want %v", len(result.Data), len(tt.expectedSecret.Data))
			}
			for key, expectedValue := range tt.expectedSecret.Data {
				if actualValue, ok := result.Data[key]; !ok {
					t.Errorf("GetSecretWithoutRefGrant() secret.Data missing key %q", key)
				} else if string(actualValue) != string(expectedValue) {
					t.Errorf("GetSecretWithoutRefGrant() secret.Data[%q] = %q, want %q", key, string(actualValue), string(expectedValue))
				}
			}
		})
	}
}

// TestSecretsBySelectorLabelMatch pins that a selector matches the way the API server
// matches it: every label has to be present on the secret with the selected value. A
// marker label has an empty value, and a secret without that label must not pass for
// one that carries it.
func TestSecretsBySelectorLabelMatch(t *testing.T) {
	labeled := func(name string, labels map[string]string) *corev1.Secret {
		return &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: "app-ns", Name: name, Labels: labels}}
	}
	objs := []any{
		labeled("gold", map[string]string{"keys.example.com/gold": "", "tier": "gold"}),
		labeled("free", map[string]string{"keys.example.com/free": "", "tier": "free"}),
		labeled("gold-valued", map[string]string{"keys.example.com/gold": "true"}),
		labeled("unlabeled", nil),
	}

	mock := krttest.NewMock(t, objs)
	secretCol := krttest.GetMockCollection[*corev1.Secret](mock)
	refGrantCol := krttest.GetMockCollection[*gwv1b1.ReferenceGrant](mock)
	secretGK := corev1.SchemeGroupVersion.WithKind("Secret").GroupKind()
	secretIndex := NewSecretIndex(map[schema.GroupKind]krt.Collection[ir.Secret]{
		secretGK: krt.NewCollection(secretCol, func(kctx krt.HandlerContext, i *corev1.Secret) *ir.Secret {
			return &ir.Secret{
				ObjectSource: ir.ObjectSource{Kind: "Secret", Namespace: i.Namespace, Name: i.Name},
				Obj:          i,
				Data:         i.Data,
			}
		}),
	}, NewRefGrantIndex(refGrantCol, apisettings.ReferenceGrantPermissive))
	for !secretIndex.HasSynced() {
	}

	tests := []struct {
		name     string
		selector map[string]string
		want     []string
	}{
		{
			name:     "value",
			selector: map[string]string{"tier": "gold"},
			want:     []string{"gold"},
		},
		{
			name:     "empty value needs the label to be present",
			selector: map[string]string{"keys.example.com/gold": ""},
			want:     []string{"gold"},
		},
		{
			name:     "empty value of a label no secret has",
			selector: map[string]string{"keys.example.com/platinum": ""},
		},
		{
			name:     "all labels have to match",
			selector: map[string]string{"keys.example.com/gold": "", "tier": "free"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			secrets, err := secretIndex.GetSecretsBySelector(krt.TestingDummyContext{},
				From{GroupKind: schema.GroupKind{Group: "gateway.kgateway.dev", Kind: "TrafficPolicy"}, Namespace: "app-ns"},
				secretGK, tt.selector)
			if err != nil {
				t.Fatalf("GetSecretsBySelector() = %v, want no error", err)
			}
			var got []string
			for _, sec := range secrets {
				got = append(got, sec.Name)
			}
			slices.Sort(got)
			if !slices.Equal(got, tt.want) {
				t.Errorf("GetSecretsBySelector() = %v, want %v", got, tt.want)
			}
		})
	}
}

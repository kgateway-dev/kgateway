package pluginutils_test

import (
	"testing"

	mutation_rulesv3 "github.com/envoyproxy/go-control-plane/envoy/config/common/mutation_rules/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	matcherv3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"github.com/stretchr/testify/assert"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/kube/krt/krttest"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1b1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	sharedv1alpha1 "github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/extensions2/pluginutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

type convertMutationsTestCase struct {
	name     string
	input    *gwv1.HTTPHeaderFilter
	expected []*mutation_rulesv3.HeaderMutation
}

func TestConvertHeaderMutations(t *testing.T) {
	cases := []convertMutationsTestCase{
		{
			name: "converts all options in the correct order",
			input: &gwv1.HTTPHeaderFilter{
				Set: []gwv1.HTTPHeader{
					{
						Name:  "X-Set-1",
						Value: "Set-Value-1",
					},
					{
						Name:  "X-Set-2",
						Value: "Set-Value-2",
					},
				},
				Add: []gwv1.HTTPHeader{
					{
						Name:  "X-Add-1",
						Value: "Add-Value-1",
					},
					{
						Name:  "X-Add-2",
						Value: "Add-Value-2",
					},
				},
				Remove: []string{
					"X-Remove-1",
					"X-Remove-2",
				},
			},
			expected: []*mutation_rulesv3.HeaderMutation{
				// Required order: Add first, then potentially overwrite with Set, finally Remove:
				{
					Action: &mutation_rulesv3.HeaderMutation_Append{
						Append: &envoycorev3.HeaderValueOption{
							Header: &envoycorev3.HeaderValue{
								Key:   "X-Add-1",
								Value: "Add-Value-1",
							},
							AppendAction: envoycorev3.HeaderValueOption_APPEND_IF_EXISTS_OR_ADD,
						},
					},
				},
				{
					Action: &mutation_rulesv3.HeaderMutation_Append{
						Append: &envoycorev3.HeaderValueOption{
							Header: &envoycorev3.HeaderValue{
								Key:   "X-Add-2",
								Value: "Add-Value-2",
							},
							AppendAction: envoycorev3.HeaderValueOption_APPEND_IF_EXISTS_OR_ADD,
						},
					},
				},
				{
					Action: &mutation_rulesv3.HeaderMutation_Append{
						Append: &envoycorev3.HeaderValueOption{
							Header: &envoycorev3.HeaderValue{
								Key:   "X-Set-1",
								Value: "Set-Value-1",
							},
							AppendAction: envoycorev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD,
						},
					},
				},
				{
					Action: &mutation_rulesv3.HeaderMutation_Append{
						Append: &envoycorev3.HeaderValueOption{
							Header: &envoycorev3.HeaderValue{
								Key:   "X-Set-2",
								Value: "Set-Value-2",
							},
							AppendAction: envoycorev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD,
						},
					},
				},
				{
					Action: &mutation_rulesv3.HeaderMutation_Remove{
						Remove: "X-Remove-1",
					},
				},
				{
					Action: &mutation_rulesv3.HeaderMutation_Remove{
						Remove: "X-Remove-2",
					},
				},
			},
		},
		{
			name: "returns no mutations if all options are empty",
			input: &gwv1.HTTPHeaderFilter{
				Set:    []gwv1.HTTPHeader{},
				Add:    []gwv1.HTTPHeader{},
				Remove: []string{},
			},
			expected: nil,
		},
		{
			name:     "returns no mutations when input is nil",
			input:    nil,
			expected: nil,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			converted := pluginutils.ConvertMutations(c.input)
			assert.EqualExportedValues(t, c.expected, converted)
		})
	}
}

type convertFilterTestCase struct {
	name          string
	input         *sharedv1alpha1.HTTPHeaderFilter
	expected      *gwv1.HTTPHeaderFilter
	expectedError *string
}

func TestConvertHeaderFilter(t *testing.T) {
	secrets := []any{
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "headers",
				Namespace: "default",
			},
			Data: map[string][]byte{
				"explicit-key":      []byte("Explicit-Key-Value"),
				"Implicit-Key":      []byte("Implicit-Key-Value"),
				"Additional-Header": []byte("Additional-Header-Value"),
			},
		},
	}

	// Create mock KRT context
	mock := krttest.NewMock(t, secrets)
	secretCol := krttest.GetMockCollection[*corev1.Secret](mock)

	// Create SecretIndex
	// ReferenceGrants are not needed for same-namespace lookups, but we still need to create the index
	// Import the correct type for ReferenceGrant
	refGrantCol := krttest.GetMockCollection[*gwv1b1.ReferenceGrant](mock)
	refgrants := krtcollections.NewRefGrantIndex(refGrantCol, apisettings.ReferenceGrantPermissive)
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
	secretIndex := krtcollections.NewSecretIndex(secretsCol, refgrants)

	// Wait for collections to sync
	secretCol.WaitUntilSynced(nil)

	// Wait for SecretIndex to be synced
	for !secretIndex.HasSynced() {
		// Poll until synced
	}

	cases := []convertFilterTestCase{
		{
			name: "converts header additions",
			input: &sharedv1alpha1.HTTPHeaderFilter{
				Add: []sharedv1alpha1.HTTPHeader{
					{
						Name:  new(gwv1.HTTPHeaderName("X-Add-1")),
						Value: new("Add-Value-1"),
					},
					{
						Name:  new(gwv1.HTTPHeaderName("X-Add-2")),
						Value: new("Add-Value-2"),
					},
				},
			},
			expected: &gwv1.HTTPHeaderFilter{
				Add: []gwv1.HTTPHeader{
					{
						Name:  "X-Add-1",
						Value: "Add-Value-1",
					},
					{
						Name:  "X-Add-2",
						Value: "Add-Value-2",
					},
				},
			},
		},
		{
			name: "converts header sets",
			input: &sharedv1alpha1.HTTPHeaderFilter{
				Set: []sharedv1alpha1.HTTPHeader{
					{
						Name:  new(gwv1.HTTPHeaderName("X-Set-1")),
						Value: new("Set-Value-1"),
					},
					{
						Name:  new(gwv1.HTTPHeaderName("X-Set-2")),
						Value: new("Set-Value-2"),
					},
				},
			},
			expected: &gwv1.HTTPHeaderFilter{
				Set: []gwv1.HTTPHeader{
					{
						Name:  "X-Set-1",
						Value: "Set-Value-1",
					},
					{
						Name:  "X-Set-2",
						Value: "Set-Value-2",
					},
				},
			},
		},
		{
			name: "converts header removals",
			input: &sharedv1alpha1.HTTPHeaderFilter{
				Remove: []string{"X-Remove-1", "X-Remove-2"},
			},
			expected: &gwv1.HTTPHeaderFilter{
				Remove: []string{"X-Remove-1", "X-Remove-2"},
			},
		},
		{
			name: "converts mixed filter",
			input: &sharedv1alpha1.HTTPHeaderFilter{
				Add: []sharedv1alpha1.HTTPHeader{
					{
						Name:  new(gwv1.HTTPHeaderName("X-Add-1")),
						Value: new("Add-Value-1"),
					},
					{
						Name:  new(gwv1.HTTPHeaderName("X-Add-2")),
						Value: new("Add-Value-2"),
					},
				},
				Set: []sharedv1alpha1.HTTPHeader{
					{
						Name:  new(gwv1.HTTPHeaderName("X-Set-1")),
						Value: new("Set-Value-1"),
					},
					{
						Name:  new(gwv1.HTTPHeaderName("X-Set-2")),
						Value: new("Set-Value-2"),
					},
				},
				Remove: []string{"X-Remove-1", "X-Remove-2"},
			},
			expected: &gwv1.HTTPHeaderFilter{
				Add: []gwv1.HTTPHeader{
					{
						Name:  "X-Add-1",
						Value: "Add-Value-1",
					},
					{
						Name:  "X-Add-2",
						Value: "Add-Value-2",
					},
				},
				Set: []gwv1.HTTPHeader{
					{
						Name:  "X-Set-1",
						Value: "Set-Value-1",
					},
					{
						Name:  "X-Set-2",
						Value: "Set-Value-2",
					},
				},
				Remove: []string{"X-Remove-1", "X-Remove-2"},
			},
		},
		{
			name: "resolves secret keys",
			input: &sharedv1alpha1.HTTPHeaderFilter{
				Add: []sharedv1alpha1.HTTPHeader{
					{
						Name: new(gwv1.HTTPHeaderName("X-Add-1")),
						SecretRef: &sharedv1alpha1.SecretRefWithKey{
							Name: "headers",
							Key:  new("explicit-key"),
						},
					},
					{
						Name: new(gwv1.HTTPHeaderName("Implicit-Key")),
						SecretRef: &sharedv1alpha1.SecretRefWithKey{
							Name: "headers",
						},
					},
				},
			},
			expected: &gwv1.HTTPHeaderFilter{
				Add: []gwv1.HTTPHeader{
					{
						Name:  "X-Add-1",
						Value: "Explicit-Key-Value",
					},
					{
						Name:  "Implicit-Key",
						Value: "Implicit-Key-Value",
					},
				},
			},
		},
		{
			name: "resolves entire secret without keys or header name",
			input: &sharedv1alpha1.HTTPHeaderFilter{
				Add: []sharedv1alpha1.HTTPHeader{
					{
						SecretRef: &sharedv1alpha1.SecretRefWithKey{
							Name: "headers",
						},
					},
				},
			},
			expected: &gwv1.HTTPHeaderFilter{
				// Order is lexicographical (uppercase before lowercase)
				Add: []gwv1.HTTPHeader{
					{
						Name:  "Additional-Header",
						Value: "Additional-Header-Value",
					},
					{
						Name:  "Implicit-Key",
						Value: "Implicit-Key-Value",
					},
					{
						Name:  "explicit-key",
						Value: "Explicit-Key-Value",
					},
				},
			},
		},
		{
			name: "errors on unknown secret referenced",
			input: &sharedv1alpha1.HTTPHeaderFilter{
				Add: []sharedv1alpha1.HTTPHeader{
					{
						SecretRef: &sharedv1alpha1.SecretRefWithKey{
							Name: "unknown-headers",
						},
					},
				},
			},
			expectedError: new("failed to resolve header value(s): secret default/unknown-headers: Secret default/unknown-headers not found"),
		},
		{
			name: "errors on unknown secret key referenced",
			input: &sharedv1alpha1.HTTPHeaderFilter{
				Add: []sharedv1alpha1.HTTPHeader{
					{
						Name: new(gwv1.HTTPHeaderName("X-Add-1")),
						SecretRef: &sharedv1alpha1.SecretRefWithKey{
							Name: "headers",
							Key:  new("unknown-key"),
						},
					},
				},
			},
			expectedError: new(`failed to resolve header 'X-Add-1': secret default/headers does not contain key "unknown-key"`),
		},
		{
			name:     "returns nil for nil filter",
			input:    nil,
			expected: nil,
		},
		{
			name: "returns nil for filter with empty options",
			input: &sharedv1alpha1.HTTPHeaderFilter{
				Add:    []sharedv1alpha1.HTTPHeader{},
				Set:    []sharedv1alpha1.HTTPHeader{},
				Remove: []string{},
			},
			expected: nil,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			converted, err := pluginutils.ConvertHeaderFilter(
				krt.TestingDummyContext{},
				krtcollections.From{Namespace: "default"},
				secretIndex,
				c.input,
			)
			if c.expectedError != nil {
				assert.EqualError(t, err, *c.expectedError)
			} else {
				assert.NoError(t, err)
			}
			assert.EqualExportedValues(t, c.expected, converted)
		})
	}
}

type mutationsToOptionsTestCase struct {
	name          string
	input         []*mutation_rulesv3.HeaderMutation
	expected      []*envoycorev3.HeaderValueOption
	expectedError error
}

func TestConvertMutationsToOptions(t *testing.T) {
	cases := []mutationsToOptionsTestCase{
		{
			name: "extracts append actions",
			input: []*mutation_rulesv3.HeaderMutation{
				{
					Action: &mutation_rulesv3.HeaderMutation_Append{
						Append: &envoycorev3.HeaderValueOption{
							Header: &envoycorev3.HeaderValue{
								Key:   "X-Add-1",
								Value: "Add-Value-1",
							},
							AppendAction: envoycorev3.HeaderValueOption_APPEND_IF_EXISTS_OR_ADD,
						},
					},
				},
				{
					Action: &mutation_rulesv3.HeaderMutation_Append{
						Append: &envoycorev3.HeaderValueOption{
							Header: &envoycorev3.HeaderValue{
								Key:   "X-Set-1",
								Value: "Set-Value-1",
							},
							AppendAction: envoycorev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD,
						},
					},
				},
			},
			expected: []*envoycorev3.HeaderValueOption{
				{
					Header: &envoycorev3.HeaderValue{
						Key:   "X-Add-1",
						Value: "Add-Value-1",
					},
					AppendAction: envoycorev3.HeaderValueOption_APPEND_IF_EXISTS_OR_ADD,
				},
				{
					Header: &envoycorev3.HeaderValue{
						Key:   "X-Set-1",
						Value: "Set-Value-1",
					},
					AppendAction: envoycorev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD,
				},
			},
		},
		{
			name: "errors on remove action",
			input: []*mutation_rulesv3.HeaderMutation{
				{
					Action: &mutation_rulesv3.HeaderMutation_Append{
						Append: &envoycorev3.HeaderValueOption{
							Header: &envoycorev3.HeaderValue{
								Key:   "X-Add-1",
								Value: "Add-Value-1",
							},
							AppendAction: envoycorev3.HeaderValueOption_APPEND_IF_EXISTS_OR_ADD,
						},
					},
				},
				{
					Action: &mutation_rulesv3.HeaderMutation_Remove{
						Remove: "X-Remove-1",
					},
				},
				{
					Action: &mutation_rulesv3.HeaderMutation_Append{
						Append: &envoycorev3.HeaderValueOption{
							Header: &envoycorev3.HeaderValue{
								Key:   "X-Set-1",
								Value: "Set-Value-1",
							},
							AppendAction: envoycorev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD,
						},
					},
				},
			},
			expectedError: pluginutils.ErrUnsupportedRemoveHeaderMutation,
		},
		{
			name: "errors on unknown action",
			input: []*mutation_rulesv3.HeaderMutation{
				{
					Action: &mutation_rulesv3.HeaderMutation_Append{
						Append: &envoycorev3.HeaderValueOption{
							Header: &envoycorev3.HeaderValue{
								Key:   "X-Add-1",
								Value: "Add-Value-1",
							},
							AppendAction: envoycorev3.HeaderValueOption_APPEND_IF_EXISTS_OR_ADD,
						},
					},
				},
				{
					Action: &mutation_rulesv3.HeaderMutation_RemoveOnMatch_{
						RemoveOnMatch: &mutation_rulesv3.HeaderMutation_RemoveOnMatch{
							KeyMatcher: &matcherv3.StringMatcher{
								MatchPattern: &matcherv3.StringMatcher_Exact{
									Exact: "X-Remove-1",
								},
							},
						},
					},
				},
				{
					Action: &mutation_rulesv3.HeaderMutation_Append{
						Append: &envoycorev3.HeaderValueOption{
							Header: &envoycorev3.HeaderValue{
								Key:   "X-Set-1",
								Value: "Set-Value-1",
							},
							AppendAction: envoycorev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD,
						},
					},
				},
			},
			expectedError: pluginutils.ErrUnknownHeaderMutation,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			converted, err := pluginutils.ConvertMutationsToOptions(c.input)
			assert.ErrorIs(t, err, c.expectedError)
			assert.EqualExportedValues(t, c.expected, converted)
		})
	}
}

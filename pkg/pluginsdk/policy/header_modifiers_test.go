package policy

import (
	"testing"

	mutation_rulesv3 "github.com/envoyproxy/go-control-plane/envoy/config/common/mutation_rules/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	header_mutationv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/header_mutation/v3"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/testing/protocmp"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
)

func TestBuildHeaderModifiersPolicy(t *testing.T) {
	tests := []struct {
		name  string
		input *v1alpha1.HeaderModifiers
		want  *header_mutationv3.HeaderMutationPerRoute
	}{
		{
			name:  "empty policy",
			input: &v1alpha1.HeaderModifiers{},
			want:  &header_mutationv3.HeaderMutationPerRoute{},
		},
		{
			name: "request add header modifier only",
			input: &v1alpha1.HeaderModifiers{
				Request: &gwv1.HTTPHeaderFilter{
					Add: []gwv1.HTTPHeader{{
						Name:  "x-request-id",
						Value: "test-request-id",
					}},
				},
			},
			want: &header_mutationv3.HeaderMutationPerRoute{
				Mutations: &header_mutationv3.Mutations{
					RequestMutations: []*mutation_rulesv3.HeaderMutation{{
						Action: &mutation_rulesv3.HeaderMutation_Append{
							Append: &envoycorev3.HeaderValueOption{
								Header: &envoycorev3.HeaderValue{
									Key:   "x-request-id",
									Value: "test-request-id",
								},
								AppendAction: envoycorev3.HeaderValueOption_APPEND_IF_EXISTS_OR_ADD,
							},
						},
					}},
				},
			},
		},
		{
			name: "response add header modifier only",
			input: &v1alpha1.HeaderModifiers{
				Response: &gwv1.HTTPHeaderFilter{
					Add: []gwv1.HTTPHeader{{
						Name:  "x-response-id",
						Value: "test-response-id",
					}},
				},
			},
			want: &header_mutationv3.HeaderMutationPerRoute{
				Mutations: &header_mutationv3.Mutations{
					ResponseMutations: []*mutation_rulesv3.HeaderMutation{{
						Action: &mutation_rulesv3.HeaderMutation_Append{
							Append: &envoycorev3.HeaderValueOption{
								Header: &envoycorev3.HeaderValue{
									Key:   "x-response-id",
									Value: "test-response-id",
								},
								AppendAction: envoycorev3.HeaderValueOption_APPEND_IF_EXISTS_OR_ADD,
							},
						},
					}},
				},
			},
		},
		{
			name: "request and response add header modifiers",
			input: &v1alpha1.HeaderModifiers{
				Request: &gwv1.HTTPHeaderFilter{
					Add: []gwv1.HTTPHeader{{
						Name:  "x-request-id",
						Value: "test-request-id",
					}},
				},
				Response: &gwv1.HTTPHeaderFilter{
					Add: []gwv1.HTTPHeader{{
						Name:  "x-response-id",
						Value: "test-response-id",
					}},
				},
			},
			want: &header_mutationv3.HeaderMutationPerRoute{
				Mutations: &header_mutationv3.Mutations{
					RequestMutations: []*mutation_rulesv3.HeaderMutation{{
						Action: &mutation_rulesv3.HeaderMutation_Append{
							Append: &envoycorev3.HeaderValueOption{
								Header: &envoycorev3.HeaderValue{
									Key:   "x-request-id",
									Value: "test-request-id",
								},
								AppendAction: envoycorev3.HeaderValueOption_APPEND_IF_EXISTS_OR_ADD,
							},
						},
					}},
					ResponseMutations: []*mutation_rulesv3.HeaderMutation{{
						Action: &mutation_rulesv3.HeaderMutation_Append{
							Append: &envoycorev3.HeaderValueOption{
								Header: &envoycorev3.HeaderValue{
									Key:   "x-response-id",
									Value: "test-response-id",
								},
								AppendAction: envoycorev3.HeaderValueOption_APPEND_IF_EXISTS_OR_ADD,
							},
						},
					}},
				},
			},
		},
		{
			name: "request and response remove header modifiers",
			input: &v1alpha1.HeaderModifiers{
				Request: &gwv1.HTTPHeaderFilter{
					Remove: []string{"x-request-id"},
				},
				Response: &gwv1.HTTPHeaderFilter{
					Remove: []string{"x-response-id"},
				},
			},
			want: &header_mutationv3.HeaderMutationPerRoute{
				Mutations: &header_mutationv3.Mutations{
					RequestMutations: []*mutation_rulesv3.HeaderMutation{{
						Action: &mutation_rulesv3.HeaderMutation_Remove{
							Remove: "x-request-id",
						},
					}},
					ResponseMutations: []*mutation_rulesv3.HeaderMutation{{
						Action: &mutation_rulesv3.HeaderMutation_Remove{
							Remove: "x-response-id",
						},
					}},
				},
			},
		},
		{
			name: "request and response set header modifiers",
			input: &v1alpha1.HeaderModifiers{
				Request: &gwv1.HTTPHeaderFilter{
					Set: []gwv1.HTTPHeader{{
						Name:  "x-request-id",
						Value: "test-request-id",
					}},
				},
				Response: &gwv1.HTTPHeaderFilter{
					Set: []gwv1.HTTPHeader{{
						Name:  "x-response-id",
						Value: "test-response-id",
					}},
				},
			},
			want: &header_mutationv3.HeaderMutationPerRoute{
				Mutations: &header_mutationv3.Mutations{
					RequestMutations: []*mutation_rulesv3.HeaderMutation{{
						Action: &mutation_rulesv3.HeaderMutation_Append{
							Append: &envoycorev3.HeaderValueOption{
								Header: &envoycorev3.HeaderValue{
									Key:   "x-request-id",
									Value: "test-request-id",
								},
								AppendAction: envoycorev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD,
							},
						},
					}},
					ResponseMutations: []*mutation_rulesv3.HeaderMutation{{
						Action: &mutation_rulesv3.HeaderMutation_Append{
							Append: &envoycorev3.HeaderValueOption{
								Header: &envoycorev3.HeaderValue{
									Key:   "x-response-id",
									Value: "test-response-id",
								},
								AppendAction: envoycorev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD,
							},
						},
					}},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := assert.New(t)
			got := BuildHeaderModifiersPolicy(tt.input)
			diff := cmp.Diff(got, tt.want, protocmp.Transform())
			a.Empty(diff)
		})
	}
}

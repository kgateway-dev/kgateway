package plugins

import (
	"testing"

	"github.com/agentgateway/agentgateway/go/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/utils/ptr"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
)

func TestToJSONValue(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{
			name: "regular string",
			in:   `hello`,
			want: `"hello"`,
		},
		{
			name: "JSON string",
			in:   `"hello"`,
			want: `"hello"`,
		},
		{
			name: "JSON number",
			in:   `0.8`,
			want: `0.8`,
		},
		{
			name:    "invalid JSON value",
			in:      `{"key": "value"`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := assert.New(t)

			got, err := toJSONValue(tt.in)
			if tt.wantErr {
				a.Error(err)
				return
			}
			a.NoError(err)
			a.JSONEq(tt.want, got)
		})
	}
}

func TestProcessTransformationPolicy(t *testing.T) {
	tests := []struct {
		name         string
		policy       *v1alpha1.TrafficPolicy
		policyName   string
		policyTarget *api.PolicyTarget
		wantErr      bool
		errContains  string
		validate     func(t *testing.T, policies []AgwPolicy, err error)
	}{
		{
			name: "valid request transformation with set headers",
			policy: &v1alpha1.TrafficPolicy{
				Spec: v1alpha1.TrafficPolicySpec{
					Transformation: &v1alpha1.TransformationPolicy{
						Request: &v1alpha1.Transform{
							Set: []v1alpha1.HeaderTransformation{
								{
									Name:  "x-custom-header",
									Value: "request.headers['x-forwarded-for']",
								},
								{
									Name:  "x-user-id",
									Value: "request.headers['authorization'].split(' ')[1]",
								},
							},
						},
					},
				},
			},
			policyName: "test-policy",
			policyTarget: &api.PolicyTarget{
				Kind: &api.PolicyTarget_Route{
					Route: "test-route",
				},
			},
			validate: func(t *testing.T, policies []AgwPolicy, err error) {
				require.NoError(t, err)
				require.Len(t, policies, 1)

				policy := policies[0].Policy
				assert.Equal(t, "test-policy:transformation:test-route", policy.Name)
				assert.Equal(t, "test-route", policy.Target.GetRoute())

				transformation := policy.Spec.GetTransformation()
				require.NotNil(t, transformation)
				require.NotNil(t, transformation.Request)
				require.Len(t, transformation.Request.Set, 2)

				assert.Equal(t, "x-custom-header", transformation.Request.Set[0].Name)
				assert.Equal(t, "request.headers['x-forwarded-for']", transformation.Request.Set[0].Expression)
				assert.Equal(t, "x-user-id", transformation.Request.Set[1].Name)
				assert.Equal(t, "request.headers['authorization'].split(' ')[1]", transformation.Request.Set[1].Expression)
			},
		},
		{
			name: "valid response transformation with add headers and remove",
			policy: &v1alpha1.TrafficPolicy{
				Spec: v1alpha1.TrafficPolicySpec{
					Transformation: &v1alpha1.TransformationPolicy{
						Response: &v1alpha1.Transform{
							Add: []v1alpha1.HeaderTransformation{
								{
									Name:  "x-response-time",
									Value: "string(timestamp(response.complete_time) - timestamp(request.start_time))",
								},
							},
							Remove: []string{"server", "x-internal-header"},
						},
					},
				},
			},
			policyName: "test-policy",
			policyTarget: &api.PolicyTarget{
				Kind: &api.PolicyTarget_Gateway{
					Gateway: "test-gateway",
				},
			},
			validate: func(t *testing.T, policies []AgwPolicy, err error) {
				require.NoError(t, err)
				require.Len(t, policies, 1)

				policy := policies[0].Policy
				assert.Equal(t, "test-policy:transformation:test-gateway", policy.Name)
				assert.Equal(t, "test-gateway", policy.Target.GetGateway())

				transformation := policy.Spec.GetTransformation()
				require.NotNil(t, transformation)
				require.NotNil(t, transformation.Response)
				require.Len(t, transformation.Response.Add, 1)
				require.Len(t, transformation.Response.Remove, 2)

				assert.Equal(t, "x-response-time", transformation.Response.Add[0].Name)
				assert.Equal(t, "string(timestamp(response.complete_time) - timestamp(request.start_time))", transformation.Response.Add[0].Expression)
				assert.Contains(t, transformation.Response.Remove, "server")
				assert.Contains(t, transformation.Response.Remove, "x-internal-header")
			},
		},
		{
			name: "valid body transformation",
			policy: &v1alpha1.TrafficPolicy{
				Spec: v1alpha1.TrafficPolicySpec{
					Transformation: &v1alpha1.TransformationPolicy{
						Request: &v1alpha1.Transform{
							Body: &v1alpha1.BodyTransformation{
								ParseAs: v1alpha1.BodyParseBehaviorAsJSON, // Should trigger warning but not error
								Value:   ptr.To(v1alpha1.Template("json({'modified': true, 'original': json(request.body)})")),
							},
						},
					},
				},
			},
			policyName: "test-policy",
			policyTarget: &api.PolicyTarget{
				Kind: &api.PolicyTarget_Route{
					Route: "test-route",
				},
			},
			validate: func(t *testing.T, policies []AgwPolicy, err error) {
				require.NoError(t, err)
				require.Len(t, policies, 1)

				policy := policies[0].Policy
				transformation := policy.Spec.GetTransformation()
				require.NotNil(t, transformation)
				require.NotNil(t, transformation.Request)
				require.NotNil(t, transformation.Request.Body)

				assert.Equal(t, "json({'modified': true, 'original': json(request.body)})", transformation.Request.Body.Expression)
			},
		},
		{
			name: "both request and response transformations",
			policy: &v1alpha1.TrafficPolicy{
				Spec: v1alpha1.TrafficPolicySpec{
					Transformation: &v1alpha1.TransformationPolicy{
						Request: &v1alpha1.Transform{
							Set: []v1alpha1.HeaderTransformation{
								{
									Name:  "x-request-id",
									Value: "uuid()",
								},
							},
						},
						Response: &v1alpha1.Transform{
							Add: []v1alpha1.HeaderTransformation{
								{
									Name:  "x-processed",
									Value: "'true'",
								},
							},
						},
					},
				},
			},
			policyName: "test-policy",
			policyTarget: &api.PolicyTarget{
				Kind: &api.PolicyTarget_Route{
					Route: "test-route",
				},
			},
			validate: func(t *testing.T, policies []AgwPolicy, err error) {
				require.NoError(t, err)
				require.Len(t, policies, 1)

				policy := policies[0].Policy
				transformation := policy.Spec.GetTransformation()
				require.NotNil(t, transformation)
				require.NotNil(t, transformation.Request)
				require.NotNil(t, transformation.Response)

				require.Len(t, transformation.Request.Set, 1)
				assert.Equal(t, "x-request-id", transformation.Request.Set[0].Name)
				assert.Equal(t, "uuid()", transformation.Request.Set[0].Expression)

				require.Len(t, transformation.Response.Add, 1)
				assert.Equal(t, "x-processed", transformation.Response.Add[0].Name)
				assert.Equal(t, "'true'", transformation.Response.Add[0].Expression)
			},
		},
		{
			name: "invalid CEL expression in header",
			policy: &v1alpha1.TrafficPolicy{
				Spec: v1alpha1.TrafficPolicySpec{
					Transformation: &v1alpha1.TransformationPolicy{
						Request: &v1alpha1.Transform{
							Set: []v1alpha1.HeaderTransformation{
								{
									Name:  "x-custom-header",
									Value: "invalid.cel.expression.(",
								},
							},
						},
					},
				},
			},
			policyName: "test-policy",
			policyTarget: &api.PolicyTarget{
				Kind: &api.PolicyTarget_Route{
					Route: "test-route",
				},
			},
			wantErr:     true,
			errContains: "invalid header value",
			validate: func(t *testing.T, policies []AgwPolicy, err error) {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "invalid header value: invalid.cel.expression.(")
				// only one invalid transformation, no policy should be translated
				require.Nil(t, policies)
			},
		},
		{
			name: "partially valid CEL expression in header",
			policy: &v1alpha1.TrafficPolicy{
				Spec: v1alpha1.TrafficPolicySpec{
					Transformation: &v1alpha1.TransformationPolicy{
						Request: &v1alpha1.Transform{
							Set: []v1alpha1.HeaderTransformation{
								{
									Name:  "x-custom-header",
									Value: "foolen_{{header(\"content-length\")}}",
								},
								{
									Name:  "x-valid-header",
									Value: "'foolen_' + request.headers['content-length']",
								},
							},
						},
					},
				},
			},
			policyName: "test-policy",
			policyTarget: &api.PolicyTarget{
				Kind: &api.PolicyTarget_Route{
					Route: "test-route",
				},
			},
			wantErr:     true,
			errContains: "invalid header value",
			validate: func(t *testing.T, policies []AgwPolicy, err error) {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "invalid header value: foolen_{{header(\"content-length\")}}")
				// partially valid transformation, one policy should still be translated
				require.Len(t, policies, 1)
			},
		},
		{
			name: "invalid CEL expression in body",
			policy: &v1alpha1.TrafficPolicy{
				Spec: v1alpha1.TrafficPolicySpec{
					Transformation: &v1alpha1.TransformationPolicy{
						Request: &v1alpha1.Transform{
							Body: &v1alpha1.BodyTransformation{
								Value: ptr.To(v1alpha1.Template("invalid body expression }")),
							},
						},
					},
				},
			},
			policyName: "test-policy",
			policyTarget: &api.PolicyTarget{
				Kind: &api.PolicyTarget_Route{
					Route: "test-route",
				},
			},
			wantErr:     true,
			errContains: "invalid body value",
			validate: func(t *testing.T, policies []AgwPolicy, err error) {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "invalid body value: invalid body expression }")
				// only one invalid transformation, no policy should be translated
				require.Nil(t, policies)
			},
		},
		{
			name: "empty transformation spec",
			policy: &v1alpha1.TrafficPolicy{
				Spec: v1alpha1.TrafficPolicySpec{
					Transformation: &v1alpha1.TransformationPolicy{
						Request:  &v1alpha1.Transform{},
						Response: &v1alpha1.Transform{},
					},
				},
			},
			policyName: "test-policy",
			policyTarget: &api.PolicyTarget{
				Kind: &api.PolicyTarget_Route{
					Route: "test-route",
				},
			},
			validate: func(t *testing.T, policies []AgwPolicy, err error) {
				require.NoError(t, err)

				// Empty transforms should not create policies
				require.Nil(t, policies)
			},
		},
		{
			name: "nil request and response specs",
			policy: &v1alpha1.TrafficPolicy{
				Spec: v1alpha1.TrafficPolicySpec{
					Transformation: &v1alpha1.TransformationPolicy{
						Request:  nil,
						Response: nil,
					},
				},
			},
			policyName: "test-policy",
			policyTarget: &api.PolicyTarget{
				Kind: &api.PolicyTarget_Route{
					Route: "test-route",
				},
			},
			validate: func(t *testing.T, policies []AgwPolicy, err error) {
				require.NoError(t, err)
				assert.Nil(t, policies)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policies, err := processTransformationPolicy(tt.policy, tt.policyName, tt.policyTarget)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			}

			if tt.validate != nil {
				tt.validate(t, policies, err)
			}
		})
	}
}

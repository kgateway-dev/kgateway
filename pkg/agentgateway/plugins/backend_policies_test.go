package plugins

import (
	"testing"

	"github.com/agentgateway/agentgateway/go/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
)

func TestTranslateBackendMCPAuthorization(t *testing.T) {
	targetGateway := &api.PolicyTarget{
		Kind: &api.PolicyTarget_Gateway{Gateway: "gw"},
	}
	targetRoute := &api.PolicyTarget{
		Kind: &api.PolicyTarget_Route{Route: "route-1"},
	}

	tests := []struct {
		name     string
		pol      *v1alpha1.AgentgatewayPolicy
		target   *api.PolicyTarget
		validate func(t *testing.T, policies []AgwPolicy)
	}{
		{
			name: "nil backend",
			pol:  &v1alpha1.AgentgatewayPolicy{},
			target: &api.PolicyTarget{
				Kind: &api.PolicyTarget_Gateway{Gateway: "gw"},
			},
			validate: func(t *testing.T, policies []AgwPolicy) {
				require.Nil(t, policies)
			},
		},
		{
			name: "nil mcp",
			pol: &v1alpha1.AgentgatewayPolicy{
				Spec: v1alpha1.AgentgatewayPolicySpec{
					Backend: &v1alpha1.AgentgatewayPolicyBackend{},
				},
			},
			target: targetGateway,
			validate: func(t *testing.T, policies []AgwPolicy) {
				require.Nil(t, policies)
			},
		},
		{
			name: "nil authorization",
			pol: &v1alpha1.AgentgatewayPolicy{
				Spec: v1alpha1.AgentgatewayPolicySpec{
					Backend: &v1alpha1.AgentgatewayPolicyBackend{
						MCP: &v1alpha1.BackendMCP{},
					},
				},
			},
			target: targetGateway,
			validate: func(t *testing.T, policies []AgwPolicy) {
				require.Nil(t, policies)
			},
		},
		{
			name: "allow with two expressions; traffic authorization ignored",
			pol: &v1alpha1.AgentgatewayPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "agw",
				},
				Spec: v1alpha1.AgentgatewayPolicySpec{
					Traffic: &v1alpha1.AgentgatewayPolicyTraffic{
						Authorization: &v1alpha1.Authorization{
							Action: v1alpha1.AuthorizationPolicyActionDeny,
							Policy: v1alpha1.AuthorizationPolicy{
								MatchExpressions: []v1alpha1.CELExpression{"should_be_ignored"},
							},
						},
					},
					Backend: &v1alpha1.AgentgatewayPolicyBackend{
						MCP: &v1alpha1.BackendMCP{
							Authorization: &v1alpha1.Authorization{
								Policy: v1alpha1.AuthorizationPolicy{
									MatchExpressions: []v1alpha1.CELExpression{
										`mcp.tool.name == "echo"`,
										`jwt.sub == "test-user" && mcp.tool.name == "add"`,
									},
								},
							},
						},
					},
				},
			},
			target: targetGateway,
			validate: func(t *testing.T, policies []AgwPolicy) {
				require.Len(t, policies, 1)
				agwPol := policies[0].Policy
				assert.Equal(t, "gw", agwPol.Target.GetGateway())
				b := agwPol.GetBackend()
				require.NotNil(t, b)
				mcp := b.GetMcpAuthorization()
				require.NotNil(t, mcp)
				assert.ElementsMatch(t, []string{
					`mcp.tool.name == "echo"`,
					`jwt.sub == "test-user" && mcp.tool.name == "add"`,
				}, mcp.Allow)
				assert.Empty(t, mcp.Deny)
				assert.Contains(t, agwPol.Name, "mcp-authorization")
			},
		},
		{
			name: "deny single expression on route target",
			pol: &v1alpha1.AgentgatewayPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "name",
				},
				Spec: v1alpha1.AgentgatewayPolicySpec{
					Backend: &v1alpha1.AgentgatewayPolicyBackend{
						MCP: &v1alpha1.BackendMCP{
							Authorization: &v1alpha1.Authorization{
								Action: v1alpha1.AuthorizationPolicyActionDeny,
								Policy: v1alpha1.AuthorizationPolicy{
									MatchExpressions: []v1alpha1.CELExpression{
										`mcp.tool.name == "block_me"`,
									},
								},
							},
						},
					},
				},
			},
			target: targetRoute,
			validate: func(t *testing.T, policies []AgwPolicy) {
				require.Len(t, policies, 1)
				agwPol := policies[0].Policy
				assert.Equal(t, "route-1", agwPol.Target.GetRoute())
				b := agwPol.GetBackend()
				require.NotNil(t, b)
				mcp := b.GetMcpAuthorization()
				require.NotNil(t, mcp)
				assert.ElementsMatch(t, []string{`mcp.tool.name == "block_me"`}, mcp.Deny)
				assert.Empty(t, mcp.Allow)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policies := translateBackendMCPAuthorization(tt.pol, tt.target)
			if tt.validate != nil {
				tt.validate(t, policies)
			}
		})
	}
}

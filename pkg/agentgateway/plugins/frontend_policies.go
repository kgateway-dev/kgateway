package plugins

import (
	"errors"

	"github.com/agentgateway/agentgateway/go/api"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
)

func translateFrontendPolicyToAgw(
	ctx PolicyCtx,
	policy *v1alpha1.AgentgatewayPolicy,
	policyTarget *api.PolicyTarget,
) ([]AgwPolicy, error) {
	frontend := policy.Spec.Frontend
	if frontend == nil {
		return nil, nil
	}
	agwPolicies := make([]AgwPolicy, 0)
	var errs []error

	policyName := getFrontendPolicyName(policy.Namespace, policy.Name)

	if s := frontend.HTTP; s != nil {
		pol, err := translateFrontendHTTP(ctx, policy, policyName, policyTarget)
		if err != nil {
			logger.Error("error processing frontend HTTP", "err", err)
			errs = append(errs, err)
		}
		agwPolicies = append(agwPolicies, pol...)
	}

	if s := frontend.TLS; s != nil {
		pol, err := translateFrontendTLS(ctx, policy, policyName, policyTarget)
		if err != nil {
			logger.Error("error processing frontend TLS", "err", err)
			errs = append(errs, err)
		}
		agwPolicies = append(agwPolicies, pol...)
	}

	if s := frontend.TCP; s != nil {
		pol, err := translateFrontendTCP(ctx, policy, policyName, policyTarget)
		if err != nil {
			logger.Error("error processing frontend TCP", "err", err)
			errs = append(errs, err)
		}
		agwPolicies = append(agwPolicies, pol...)
	}

	if s := frontend.AccessLog; s != nil {
		pol, err := translateFrontendAccessLog(ctx, policy, policyName, policyTarget)
		if err != nil {
			logger.Error("error processing frontend AccessLog", "err", err)
			errs = append(errs, err)
		}
		agwPolicies = append(agwPolicies, pol...)
	}

	if s := frontend.Tracing; s != nil {
		pol, err := translateFrontendTracing(ctx, policy, policyName, policyTarget)
		if err != nil {
			logger.Error("error processing frontend Tracing", "err", err)
			errs = append(errs, err)
		}
		agwPolicies = append(agwPolicies, pol...)
	}

	return agwPolicies, errors.Join(errs...)
}

func translateFrontendTracing(ctx PolicyCtx, policy *v1alpha1.AgentgatewayPolicy, name string, target *api.PolicyTarget) ([]AgwPolicy, error) {
	// TODO
	return nil, nil
}

func translateFrontendAccessLog(ctx PolicyCtx, policy *v1alpha1.AgentgatewayPolicy, name string, target *api.PolicyTarget) ([]AgwPolicy, error) {
	// TODO
	return nil, nil
}

func translateFrontendTCP(ctx PolicyCtx, policy *v1alpha1.AgentgatewayPolicy, name string, target *api.PolicyTarget) ([]AgwPolicy, error) {
	// TODO
	return nil, nil
}

func translateFrontendTLS(ctx PolicyCtx, policy *v1alpha1.AgentgatewayPolicy, name string, target *api.PolicyTarget) ([]AgwPolicy, error) {
	// TODO
	return nil, nil
}

func translateFrontendHTTP(ctx PolicyCtx, policy *v1alpha1.AgentgatewayPolicy, name string, target *api.PolicyTarget) ([]AgwPolicy, error) {
	// TODO
	return nil, nil
}

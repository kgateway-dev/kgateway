package plugins

import (
	"fmt"

	"github.com/agentgateway/agentgateway/go/api"
	wrappers "google.golang.org/protobuf/types/known/wrapperspb"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/runtime/schema"
	inf "sigs.k8s.io/gateway-api-inference-extension/api/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
)

// NewInferencePlugin creates a new InferencePool policy plugin
func NewInferencePlugin(agw *AgwCollections) AgwPlugin {
	domainSuffix := kubeutils.GetClusterDomainName()
	policyCol := krt.NewManyCollection(agw.InferencePools, func(krtctx krt.HandlerContext, infPool *inf.InferencePool) []AgwPolicy {
		return translatePoliciesForInferencePool(infPool, domainSuffix)
	})
	return AgwPlugin{
		ContributesPolicies: map[schema.GroupKind]PolicyPlugin{
			wellknown.InferencePoolGVK.GroupKind(): {
				Policies: policyCol,
			},
		},
		ExtraHasSynced: func() bool {
			return policyCol.HasSynced()
		},
	}
}

// translatePoliciesForInferencePool generates policies for a single inference pool
func translatePoliciesForInferencePool(pool *inf.InferencePool, domainSuffix string) []AgwPolicy {
	var infPolicies []AgwPolicy

	// 'service/{namespace}/{hostname}:{port}'
	svc := fmt.Sprintf("service/%v/%v.%v.inference.%v:%v",
		// Note: InferencePool v1 only supports a single target port
		pool.Namespace, pool.Name, pool.Namespace, domainSuffix, pool.Spec.TargetPorts[0].Number)

	epr := pool.Spec.EndpointPickerRef
	if epr.Group != nil && *epr.Group != "" {
		logger.Warn("inference pool endpoint picker ref has non-empty group, skipping", "pool", pool.Name, "group", *epr.Group)
		return nil
	}

	if epr.Kind != wellknown.ServiceKind {
		logger.Warn("inference pool extension ref is not a Service, skipping", "pool", pool.Name, "kind", epr.Kind)
		return nil
	}

	if epr.Port == nil {
		logger.Warn("inference pool extension ref port must be specified, skipping", "pool", pool.Name, "kind", epr.Kind)
		return nil
	}

	eppPort := epr.Port.Number

	eppSvc := fmt.Sprintf("%v/%v.%v.svc.%v",
		pool.Namespace, epr.Name, pool.Namespace, domainSuffix)
	eppPolicyTarget := fmt.Sprintf("service/%v:%v",
		eppSvc, eppPort)

	failureMode := api.PolicySpec_InferenceRouting_FAIL_CLOSED
	if epr.FailureMode == inf.EndpointPickerFailOpen {
		failureMode = api.PolicySpec_InferenceRouting_FAIL_OPEN
	}

	// Create the inference routing policy
	inferencePolicy := &api.Policy{
		Name:   pool.Namespace + "/" + pool.Name + ":inference",
		Target: &api.PolicyTarget{Kind: &api.PolicyTarget_Backend{Backend: svc}},
		Spec: &api.PolicySpec{
			Kind: &api.PolicySpec_InferenceRouting_{
				InferenceRouting: &api.PolicySpec_InferenceRouting{
					EndpointPicker: &api.BackendReference{
						Kind: &api.BackendReference_Service{Service: eppSvc},
						Port: uint32(eppPort), //nolint:gosec // G115: eppPort is derived from validated port numbers
					},
					FailureMode: failureMode,
				},
			},
		},
	}
	infPolicies = append(infPolicies, AgwPolicy{Policy: inferencePolicy})

	// Create the TLS policy for the endpoint picker
	// TODO: we would want some way if they explicitly set a BackendTLSPolicy for the EPP to respect that
	inferencePolicyTLS := &api.Policy{
		Name:   pool.Namespace + "/" + pool.Name + ":inferencetls",
		Target: &api.PolicyTarget{Kind: &api.PolicyTarget_Backend{Backend: eppPolicyTarget}},
		Spec: &api.PolicySpec{
			Kind: &api.PolicySpec_BackendTls{
				BackendTls: &api.PolicySpec_BackendTLS{
					// The spec mandates this :vomit:
					Insecure: wrappers.Bool(true),
				},
			},
		},
	}
	infPolicies = append(infPolicies, AgwPolicy{Policy: inferencePolicyTLS})

	logger.Debug("generated inference pool policies",
		"pool", pool.Name,
		"namespace", pool.Namespace,
		"inference_policy", inferencePolicy.Name,
		"tls_policy", inferencePolicyTLS.Name)

	return infPolicies
}

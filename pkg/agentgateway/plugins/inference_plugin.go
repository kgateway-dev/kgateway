package plugins

import (
	"fmt"

	"github.com/agentgateway/agentgateway/go/api"
	wrappers "google.golang.org/protobuf/types/known/wrapperspb"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/ptr"
	"k8s.io/apimachinery/pkg/runtime/schema"
	inf "sigs.k8s.io/gateway-api-inference-extension/api/v1alpha2"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
)

// NewInferencePlugin creates a new InferencePool policy plugin
func NewInferencePlugin(agw *AgwCollections) AgentgatewayPlugin {
	domainSuffix := kubeutils.GetClusterDomainName()
	policyCol := krt.NewManyCollection(agw.InferencePools, func(krtctx krt.HandlerContext, infPool *inf.InferencePool) []ADPPolicy {
		return translatePoliciesForInferencePool(infPool, domainSuffix)
	})
	return AgentgatewayPlugin{
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
func translatePoliciesForInferencePool(pool *inf.InferencePool, domainSuffix string) []ADPPolicy {
	logger := logging.New("agentgateway/plugins/inference")
	var infPolicies []ADPPolicy

	// 'service/{namespace}/{hostname}:{port}'
	svc := fmt.Sprintf("service/%v/%v.%v.inference.%v:%v",
		pool.Namespace, pool.Name, pool.Namespace, domainSuffix, pool.Spec.TargetPortNumber)

	er := pool.Spec.ExtensionRef
	if er == nil {
		logger.Warn("inference pool has no extension ref", "pool", pool.Name)
		return nil
	}

	erf := er.ExtensionReference
	if erf.Group != nil && *erf.Group != "" {
		logger.Warn("inference pool extension ref has non-empty group, skipping", "pool", pool.Name, "group", *erf.Group)
		return nil
	}

	if erf.Kind != nil && *erf.Kind != "Service" {
		logger.Warn("inference pool extension ref is not a Service, skipping", "pool", pool.Name, "kind", *erf.Kind)
		return nil
	}

	eppPort := ptr.OrDefault(erf.PortNumber, 9002)

	eppSvc := fmt.Sprintf("%v/%v.%v.svc.%v",
		pool.Namespace, erf.Name, pool.Namespace, domainSuffix)
	eppPolicyTarget := fmt.Sprintf("service/%v:%v",
		eppSvc, eppPort)

	failureMode := api.PolicySpec_InferenceRouting_FAIL_CLOSED
	if er.FailureMode == nil || *er.FailureMode == inf.FailOpen {
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
						Port: uint32(eppPort),
					},
					FailureMode: failureMode,
				},
			},
		},
	}
	infPolicies = append(infPolicies, ADPPolicy{Policy: inferencePolicy})

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
	infPolicies = append(infPolicies, ADPPolicy{Policy: inferencePolicyTLS})

	logger.Debug("generated inference pool policies",
		"pool", pool.Name,
		"namespace", pool.Namespace,
		"inference_policy", inferencePolicy.Name,
		"tls_policy", inferencePolicyTLS.Name)

	return infPolicies
}

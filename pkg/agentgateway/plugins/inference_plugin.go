package plugins

import (
	"fmt"

	"github.com/agentgateway/agentgateway/go/api"
	wrappers "google.golang.org/protobuf/types/known/wrapperspb"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/ptr"
	"k8s.io/apimachinery/pkg/runtime/schema"
	inf "sigs.k8s.io/gateway-api-inference-extension/api/v1alpha2"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
)

const (
	inferencePluginName = "inference-pool-policy-plugin"
)

// InferencePlugin converts an inference pool to an agentgateway inference policy
type InferencePlugin struct{}

// NewInferencePlugin creates a new InferencePool policy plugin
func NewInferencePlugin() *InferencePlugin {
	return &InferencePlugin{}
}

// GroupKind returns the GroupKind of the policy this plugin handles
func (p *InferencePlugin) GroupKind() schema.GroupKind {
	return schema.GroupKind{
		Group: wellknown.InferencePoolGVK.GroupKind().Group,
		Kind:  wellknown.InferencePoolGVK.GroupKind().Kind,
	}
}

// Name returns the name of this plugin
func (p *InferencePlugin) Name() string {
	return inferencePluginName
}

// GeneratePolicies generates ADP policies for inference pools
func (p *InferencePlugin) GeneratePolicies(ctx krt.HandlerContext, agw *AgwCollections) ([]ADPPolicy, error) {
	logger := logging.New("agentgateway/plugins/inference")

	inferencePools := agw.InferencePools
	if inferencePools == nil {
		logger.Debug("inference pools collection is nil, skipping inference policy generation")
		return nil, nil
	}

	domainSuffix := kubeutils.GetClusterDomainName()
	return p.GenerateInferencePoolPolicies(ctx, inferencePools, domainSuffix)
}

// GenerateInferencePoolPolicies generates policies for inference pools
func (p *InferencePlugin) GenerateInferencePoolPolicies(ctx krt.HandlerContext, inferencePools krt.Collection[*inf.InferencePool], domainSuffix string) ([]ADPPolicy, error) {
	logger := logging.New("agentgateway/plugins/inference")
	logger.Debug("generating inference pool policies")

	var inferencePolicies []ADPPolicy

	// Fetch all inference pools and process them
	allInferencePools := krt.Fetch(ctx, inferencePools)

	for _, pool := range allInferencePools {
		policies := p.generatePoliciesForInferencePool(pool, domainSuffix)
		inferencePolicies = append(inferencePolicies, policies...)
	}

	logger.Info("generated inference pool policies", "count", len(inferencePolicies))
	return inferencePolicies, nil
}

// generatePoliciesForInferencePool generates policies for a single inference pool
func (p *InferencePlugin) generatePoliciesForInferencePool(pool *inf.InferencePool, domainSuffix string) []ADPPolicy {
	logger := logging.New("agentgateway/plugins/inference")

	// 'service/{namespace}/{hostname}:{port}'
	svc := fmt.Sprintf("service/%v/%v.%v.inference.%v:%v",
		pool.Namespace, pool.Name, pool.Namespace, domainSuffix, pool.Spec.TargetPortNumber)

	er := pool.Spec.ExtensionRef
	if er == nil {
		logger.Debug("inference pool has no extension ref", "pool", pool.Name)
		return nil
	}

	erf := er.ExtensionReference
	if erf.Group != nil && *erf.Group != "" {
		logger.Debug("inference pool extension ref has non-empty group, skipping", "pool", pool.Name, "group", *erf.Group)
		return nil
	}

	if erf.Kind != nil && *erf.Kind != "Service" {
		logger.Debug("inference pool extension ref is not a Service, skipping", "pool", pool.Name, "kind", *erf.Kind)
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

	logger.Debug("generated inference pool policies",
		"pool", pool.Name,
		"namespace", pool.Namespace,
		"inference_policy", inferencePolicy.Name,
		"tls_policy", inferencePolicyTLS.Name)

	return []ADPPolicy{
		{Policy: inferencePolicy},
		{Policy: inferencePolicyTLS},
	}
}

// Verify that InferencePlugin implements the required interfaces
var _ PolicyPlugin = (*InferencePlugin)(nil)

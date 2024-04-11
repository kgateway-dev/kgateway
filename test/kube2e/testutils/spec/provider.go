package spec

import (
	"context"
	"github.com/solo-io/gloo/test/testutils/kubeutils"
)

type ScenarioProvider struct {
	clusterContext *kubeutils.ClusterContext
}

func NewProvider() *ScenarioProvider {
	return &ScenarioProvider{
		clusterContext: nil,
	}
}

// WithClusterContext sets the ScenarioProvider to point to the provided cluster
func (p *ScenarioProvider) WithClusterContext(clusterContext *kubeutils.ClusterContext) *ScenarioProvider {
	p.clusterContext = clusterContext
	return p
}

func (p *ScenarioProvider) NewScenario(options ...Option) Scenario {

	properties := &specProperties{
		name:     "unnamed-test-scenario",
		manifest: "",
	}

	for _, opt := range options {
		opt(properties)
	}

}

var _ Scenario = new(scenarioImpl)

type scenarioImpl struct {
	name string
}

func (s scenarioImpl) Name() string {
	return s.name
}

func (s scenarioImpl) InitializeResources() func(ctx context.Context) error {
	//TODO implement me
	panic("implement me")
}

func (s scenarioImpl) InitializedAssertion() ScenarioAssertion {
	//TODO implement me
	panic("implement me")
}

func (s scenarioImpl) Assertion() ScenarioAssertion {
	//TODO implement me
	panic("implement me")
}

func (s scenarioImpl) ChildScenario() Scenario {
	//TODO implement me
	panic("implement me")
}

func (s scenarioImpl) FinalizeResources() func(ctx context.Context) error {
	//TODO implement me
	panic("implement me")
}

func (s scenarioImpl) FinalizedAssertion() ScenarioAssertion {
	//TODO implement me
	panic("implement me")
}

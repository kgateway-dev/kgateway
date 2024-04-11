package spec

import (
	"context"
	"github.com/solo-io/gloo/pkg/utils/kubeutils/kubectl"
	"github.com/solo-io/gloo/test/testutils/kubeutils"
	"os"
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

func (p *ScenarioProvider) NewScenario(options ...Option) (Scenario, error) {
	properties := &specProperties{
		name:     "unnamed-test-scenario",
		manifest: "",
		initializedAssertion: func(ctx context.Context) {
			// do nothing
		},
		assertion: func(ctx context.Context) {
			// do nothing
		},
		finalizedAssertion: func(ctx context.Context) {
			// do nothing
		},
	}

	for _, opt := range options {
		opt(properties)
	}

	_, err := os.Stat(properties.manifest)
	if err != nil {
		return nil, err
	}

	return &scenarioImpl{
		kubeCli:              p.clusterContext.Cli,
		name:                 properties.name,
		manifestFile:         properties.manifest,
		initializedAssertion: properties.initializedAssertion,
		assertion:            properties.assertion,
		finalizedAssertion:   properties.finalizedAssertion,
	}, nil
}

var _ Scenario = new(scenarioImpl)

type scenarioImpl struct {
	kubeCli              *kubectl.Cli
	name                 string
	manifestFile         string
	initializedAssertion ScenarioAssertion
	assertion            ScenarioAssertion
	finalizedAssertion   ScenarioAssertion
}

func (s scenarioImpl) Name() string {
	return s.name
}

func (s scenarioImpl) InitializeResources() func(ctx context.Context) error {
	return func(ctx context.Context) error {
		return s.kubeCli.ApplyFile(ctx, s.manifestFile)
	}
}

func (s scenarioImpl) InitializedAssertion() ScenarioAssertion {
	return s.initializedAssertion
}

func (s scenarioImpl) Assertion() ScenarioAssertion {
	return s.assertion
}

func (s scenarioImpl) ChildScenario() Scenario {
	// not yet supported
	return nil
}

func (s scenarioImpl) FinalizeResources() func(ctx context.Context) error {
	return func(ctx context.Context) error {
		return s.kubeCli.DeleteFile(ctx, s.manifestFile)
	}
}

func (s scenarioImpl) FinalizedAssertion() ScenarioAssertion {
	return s.finalizedAssertion
}

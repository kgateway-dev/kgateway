package manifest

import (
	"context"
	"os"

	"github.com/solo-io/gloo/pkg/utils/kubeutils/kubectl"
	"github.com/solo-io/gloo/test/kubernetes/testutils/assertions"
	"github.com/solo-io/gloo/test/kubernetes/testutils/operations"
)

type OperationProvider struct {
	kubeCli *kubectl.Cli
}

func NewProvider() *OperationProvider {
	return &OperationProvider{
		kubeCli: nil,
	}
}

// WithClusterCli sets the OperationProvider to use a Cli
func (p *OperationProvider) WithClusterCli(kubeCli *kubectl.Cli) *OperationProvider {
	p.kubeCli = kubeCli
	return p
}

func (p *OperationProvider) NewReversibleOperation(options ...Option) (operations.ReversibleOperation, error) {
	props := &properties{
		name:     "unnamed-manifest-operation",
		manifest: "",
		initializedObjectsAssertion: func(ctx context.Context) {
			// do nothing
		},
		finalizedObjectsAssertion: func(ctx context.Context) {
			// do nothing
		},
	}

	for _, opt := range options {
		opt(props)
	}

	_, err := os.Stat(props.manifest)
	if err != nil {
		return operations.ReversibleOperation{}, err
	}

	return operations.ReversibleOperation{
		Do: &operation{
			name:         props.name,
			manifestFile: props.manifest,
			execute: func(ctx context.Context) error {
				return p.kubeCli.ApplyFile(ctx, props.manifest)
			},
			assertion: props.initializedObjectsAssertion,
		},
		Undo: &operation{
			name:         props.name,
			manifestFile: props.manifest,
			execute: func(ctx context.Context) error {
				return p.kubeCli.DeleteFile(ctx, props.manifest)
			},
			assertion: props.finalizedObjectsAssertion,
		},
	}, nil
}

var _ operations.Operation = new(operation)

type operation struct {
	name         string
	manifestFile string
	execute      func(ctx context.Context) error
	assertion    assertions.DiscreteAssertion
}

func (s *operation) Name() string {
	return s.name
}

func (s *operation) Execute() func(ctx context.Context) error {
	return s.execute
}

func (s *operation) ExecutionAssertion() assertions.DiscreteAssertion {
	return s.assertion
}

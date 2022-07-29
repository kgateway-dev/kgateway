package runner

import (
	"github.com/solo-io/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/projects/gloo/pkg/runner"
)

func NewRunnerFactory() bootstrap.RunnerFactory {
	return runner.NewRunnerFactoryWithRun(RunUDS)
}

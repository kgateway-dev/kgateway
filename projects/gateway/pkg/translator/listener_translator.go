package translator

import (
	"context"

	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
)

var _ ListenerTranslator = new(NoOpTranslator)

type ListenerTranslator interface {
	Name() string
	ComputeListener(params Params, proxyName string, gateway *v1.Gateway) *gloov1.Listener
}

type Params struct {
	ctx      context.Context
	snapshot *v1.ApiSnapshot
	reports  reporter.ResourceReports
}

func NewTranslatorParams(ctx context.Context, snapshot *v1.ApiSnapshot, reports reporter.ResourceReports) Params {
	return Params{
		ctx:      ctx,
		snapshot: snapshot,
		reports:  reports,
	}
}

type NoOpTranslator struct{}

func (n NoOpTranslator) Name() string {
	return "no-op"
}

func (n NoOpTranslator) ComputeListener(params Params, proxyName string, gateway *v1.Gateway) *gloov1.Listener {
	return nil
}

package translator

import (
	"context"

	"github.com/solo-io/gloo/pkg/utils"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
)

var _ ListenerTranslator = new(NoOpTranslator)

type ListenerTranslator interface {
	Name() string
	ComputeListener(params Params, proxyName string, gateway *v1.Gateway, reports reporter.ResourceReports) *gloov1.Listener
}

type Params struct {
	ctx      context.Context
	snapshot *v1.ApiSnapshot

	// VirtualServices tend to grow in number and are frequently accessed during translation
	// The generic `Find` method works in O(n), whereas this indexed store supports O(1) lookups.
	// We likely want to move all our resources towards a similar paradigm
	// I don't think now is the appropriate time, nor am I sure this is the proper solution,
	// but this will improve VirtualService processing in the short term
	virtualServiceStore utils.ResourceStore
}

func NewTranslatorParams(ctx context.Context, snapshot *v1.ApiSnapshot) Params {
	virtualServiceStore := utils.NewResourceStore()
	virtualServiceStore.Load(snapshot.VirtualServices.AsResources())

	return Params{
		ctx:                 ctx,
		snapshot:            snapshot,
		virtualServiceStore: virtualServiceStore,
	}
}

type NoOpTranslator struct{}

func (n NoOpTranslator) Name() string {
	return "no-op"
}

func (n NoOpTranslator) ComputeListener(params Params, proxyName string, gateway *v1.Gateway, reports reporter.ResourceReports) *gloov1.Listener {
	return nil
}

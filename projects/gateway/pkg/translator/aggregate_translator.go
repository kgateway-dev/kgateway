package translator

import (
	"errors"

	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

var _ ListenerTranslator = new(AggregateTranslator)

type AggregateTranslator struct {
}

func (a *AggregateTranslator) ComputeListener(params Params, proxyName string, gateway *v1.Gateway) *gloov1.Listener {
	params.reports.AddError(gateway, errors.New("not implemented"))
	return nil
}

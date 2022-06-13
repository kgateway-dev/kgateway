package translator

import (
	"context"
	"errors"

	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
)

var _ ListenerTranslator = new(InvalidGatewayTypeTranslator)

var MissingGatewayTypeErr = errors.New("invalid gateway: gateway must contain gatewayType")

// ListenerTranslator converts a Gateway into a Listener
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

type InvalidGatewayTypeTranslator struct{}

func (n InvalidGatewayTypeTranslator) Name() string {
	return "invalid-gateway-type"
}

func (n InvalidGatewayTypeTranslator) ComputeListener(params Params, proxyName string, gateway *v1.Gateway) *gloov1.Listener {
	params.reports.AddError(gateway, MissingGatewayTypeErr)
	return nil
}

package functiontypes

import (
	"github.com/solo-io/gloo/internal/function-discovery/updater/openfaas"
	"github.com/solo-io/gloo/internal/function-discovery/updater/swagger"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/plugins/aws"
	"github.com/solo-io/gloo/pkg/plugins/azure"
	"github.com/solo-io/gloo/pkg/plugins/google"
)

type FunctionType string

const (
	FunctionTypeLambda   FunctionType = "functionTypeLambda"
	FunctionTypeGfuncs   FunctionType = "functionTypeGfuncs"
	FunctionTypeSwagger  FunctionType = "functionTypeSwagger"
	FunctionTypeOpenFaas FunctionType = "functionTypeFaas"
	FunctionTypeAzure    FunctionType = "functionTypeAzure"
	NonFunctional        FunctionType = "nonFunctional"
)

func GetFunctionType(us *v1.Upstream) FunctionType {
	switch {
	case us.Type == aws.UpstreamTypeAws:
		return FunctionTypeLambda
	case us.Type == gfunc.UpstreamTypeGoogle:
		return FunctionTypeGfuncs
	case us.Type == azure.UpstreamTypeAzure:
		return FunctionTypeAzure
	case swagger.IsSwagger(us):
		return FunctionTypeSwagger
	case openfaas.IsOpenFaas(us):
		return FunctionTypeOpenFaas
	}
	return NonFunctional
}

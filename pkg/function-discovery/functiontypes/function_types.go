package functiontypes

import (
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/function-discovery/updater/fission"
	"github.com/solo-io/gloo/pkg/function-discovery/updater/grpc"
	"github.com/solo-io/gloo/pkg/function-discovery/updater/openfaas"
	"github.com/solo-io/gloo/pkg/function-discovery/updater/projectfn"
	"github.com/solo-io/gloo/pkg/function-discovery/updater/swagger"
	"github.com/solo-io/gloo/pkg/plugins/aws"
	"github.com/solo-io/gloo/pkg/plugins/azure"
	"github.com/solo-io/gloo/pkg/plugins/google"
)

type FunctionType string

const (
	FunctionTypeLambda    FunctionType = "functionTypeLambda"
	FunctionTypeGfuncs    FunctionType = "functionTypeGfuncs"
	FunctionTypeSwagger   FunctionType = "functionTypeSwagger"
	FunctionTypeOpenFaaS  FunctionType = "functionTypeFaaS"
	FunctionTypeAzure     FunctionType = "functionTypeAzure"
	FunctionTypeFission   FunctionType = "functionTypeFission"
	FunctionTypeProjectFn FunctionType = "functionTypeProjectFn"
	NonFunctional         FunctionType = "nonFunctional"
	FunctionTypeGRPC      FunctionType = "functionTypeGRPC"
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
	case grpc.IsGRPC(us):
		return FunctionTypeGRPC
	case openfaas.IsOpenFaaSGateway(us):
		return FunctionTypeOpenFaaS
	case fission.IsFissionUpstream(us):
		return FunctionTypeFission
	case projectfn.IsFnUpstream(us):
		return FunctionTypeProjectFn
	}
	return NonFunctional
}

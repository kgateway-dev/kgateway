package fission

import (
	"fmt"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/function-discovery/resolver"
	"github.com/solo-io/gloo/pkg/plugins/kubernetes"
	"github.com/solo-io/gloo/pkg/plugins/rest"

	"github.com/solo-io/gloo/pkg/function-discovery/updater/fission/imported"
)

func GetFuncs(resolve resolver.Resolver, us *v1.Upstream) ([]*v1.Function, error) {
	client, err := getFissionClient()
	if err != nil {
		return nil, err
	}
	fr := NewFissionRetreiver(client)
	return fr.GetFuncs(resolve, us)
}

// ideally we want to use the function UrlForFunction from here:
// https://github.com/fission/fission/blob/master/common.go
// but importing it makes dep go crazy.
func UrlForFunction(name string) string {
	prefix := "/fission-function"
	return fmt.Sprintf("%v/%v", prefix, name)
}

func IsFissionUpstream(us *v1.Upstream) bool {

	if us.Type != kubernetes.UpstreamTypeKube {
		return false
	}

	spec, err := kubernetes.DecodeUpstreamSpec(us.Spec)
	if err != nil {
		return false
	}

	if spec.ServiceNamespace != "fission" || spec.ServiceName != "router" {
		return false
	}
	return true

}

type FissionFunctions []fission_imported.Function

type FissionRetreiver struct {
	fissionClient fission_imported.FissionClient
}

func NewFissionRetreiver(c fission_imported.FissionClient) *FissionRetreiver {
	return &FissionRetreiver{fissionClient: c}
}
func (fr *FissionRetreiver) listFissionFunctions(ns string) (FissionFunctions, error) {
	var opts metav1.ListOptions
	l, err := fr.fissionClient.Functions(ns).List(opts)
	if err != nil {
		return nil, err
	}
	return l.Items, nil
}

func (fr *FissionRetreiver) GetFuncs(_ resolver.Resolver, us *v1.Upstream) ([]*v1.Function, error) {

	if !IsFissionUpstream(us) {
		return nil, nil
	}

	functions, err := fr.listFissionFunctions("default")
	if err != nil {
		return nil, errors.Wrap(err, "error fetching functions")
	}

	var funcs []*v1.Function

	for _, fn := range functions {
		if fn.Metadata.Name != "" {
			funcs = append(funcs, createFunction(fn))
		}
	}
	return funcs, nil
}

func createFunction(fn fission_imported.Function) *v1.Function {

	headersTemplate := map[string]string{":method": "POST"}

	return &v1.Function{
		Name: fn.Metadata.Name,
		Spec: rest.EncodeFunctionSpec(rest.TransformationSpec{
			Path:            UrlForFunction(fn.Metadata.Name),
			Header:          headersTemplate,
			PassthroughBody: true,
		}),
	}
}

func getFissionClient() (fission_imported.FissionClient, error) {
	fissionClient, _, _, err := fission_imported.MakeFissionClient()
	if err != nil {
		return nil, err
	}
	return fissionClient, nil
}

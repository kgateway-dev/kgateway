package openfaas

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"

	"github.com/pkg/errors"

	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/function-discovery/resolver"
	"github.com/solo-io/gloo/pkg/plugins/kubernetes"
	"github.com/solo-io/gloo/pkg/plugins/rest"
)

type function struct {
	Name            string `json:"name"`
	Image           string `json:"image"`
	InvocationCount int64  `json:"invocationCount"`
	Replicas        int64  `json:"replicas"`
}

func GetFuncs(resolve resolver.Resolver, us *v1.Upstream) ([]*v1.Function, error) {
	if !IsOpenFaaSGateway(us) {
		return nil, nil
	}

	gwAddr, err := resolve.Resolve(us)
	if err != nil {
		return nil, errors.Wrap(err, "error getting faas service")
	}

	// convert it to an http address
	gwUrl := "http://" + gwAddr

	functions, err := listFuncs(gwUrl)
	if err != nil {
		return nil, errors.Wrap(err, "error fetching functions")
	}

	var funcs []*v1.Function

	for _, fn := range functions {
		if fn.Name != "" {
			funcs = append(funcs, createFunction(fn))
		}
	}
	return funcs, nil
}

func IsOpenFaaSGateway(us *v1.Upstream) bool {
	if us.Type != kubernetes.UpstreamTypeKube {
		return false
	}

	return isOpenFaaSGatewayUpstream(us)
}

func listFuncs(gwUrl string) ([]function, error) {
	u, err := url.Parse(gwUrl)
	if err != nil {
		return nil, errors.Wrap(err, "parsing url")
	}
	u.Path = "/system/functions"
	s := u.String()
	resp, err := http.Get(s)
	if err != nil {
		return nil, errors.Wrap(err, "performing http get")
	}
	if resp.StatusCode != 200 {
		return nil, errors.Wrapf(err, "unexpected status code %v", resp.StatusCode)
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "reading response body")
	}
	defer resp.Body.Close()
	var funcs []function
	if err := json.Unmarshal(body, &funcs); err != nil {
		return nil, errors.Wrap(err, "decoding json body "+s+" "+string(body))
	}

	return funcs, nil
}

func isOpenFaaSGatewayUpstream(us *v1.Upstream) bool {
	spec, err := kubernetes.DecodeUpstreamSpec(us.Spec)
	if err != nil {
		return false
	}

	if spec.ServiceNamespace != "openfaas" || spec.ServiceName != "gateway" {
		return false
	}
	return true
}

func createFunction(fn function) *v1.Function {
	headersTemplate := map[string]string{":method": "POST"}

	return &v1.Function{
		Name: fn.Name,
		Spec: rest.EncodeFunctionSpec(rest.Template{
			Path:            path.Join("/function", fn.Name),
			Header:          headersTemplate,
			PassthroughBody: true,
		}),
	}
}

package fission

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"

	"github.com/pkg/errors"

	"github.com/solo-io/gloo/internal/function-discovery/resolver"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/plugins/rest"
)

/*


router api:

curl http://192.168.99.100:32406/fission-function/hello


controller api:

 curl http://192.168.99.100:31313/v2/functions | jq
  % Total    % Received % Xferd  Average Speed   Time    Time     Time  Current
                                 Dload  Upload   Total   Spent    Left  Speed
100   628  100   628    0     0    628      0  0:00:01 --:--:--  0:00:01 78500
[
  {
    "kind": "Function",
    "apiVersion": "fission.io/v1",
    "metadata": {
      "name": "hello",
      "namespace": "default",
      "selfLink": "/apis/fission.io/v1/namespaces/default/functions/hello",
      "uid": "9944e5f6-43ce-11e8-8cf5-0800276fb67d",
      "resourceVersion": "718",
      "creationTimestamp": "2018-04-19T12:38:44Z"
    },
    "spec": {
      "environment": {
        "namespace": "default",
        "name": "nodejs"
      },
      "package": {
        "packageref": {
          "namespace": "default",
          "name": "hello-js-4hav",
          "resourceversion": "717"
        }
      },
      "secrets": null,
      "configmaps": null,
      "resources": {},
      "InvokeStrategy": {
        "ExecutionStrategy": {
          "ExecutorType": "poolmgr",
          "MinScale": 0,
          "MaxScale": 1,
          "TargetCPUPercent": 80
        },
        "StrategyType": "execution"
      }
    }
  }
]





*/

// ideally we want to use the function UrlForFunction from here:
// https://github.com/fission/fission/blob/master/common.go
// but importing it makes dep go crazy.
func UrlForFunction(name string) string {
	prefix := "/fission-function"
	return fmt.Sprintf("%v/%v", prefix, name)
}

func isFissionUpstream(us *v1.Upstream) bool {
	panic("TODO")
}

type FissionFunction struct {
	Name string `json:"name"`
}

type FissionFunctions []FissionFunction

type FissionRetreiver struct {
	Lister func(from string) (FissionFunctions, error)
}

func (fr *FissionRetreiver) GetFuncs(resolve resolver.Resolver, us *v1.Upstream) ([]*v1.Function, error) {

	if !isFissionUpstream(us) {
		return nil, nil
	}

	gw, err := resolve.Resolve(us)
	if err != nil {
		return nil, errors.Wrap(err, "error getting faas service")
	}

	if gw == "" {
		return nil, nil
	}

	// convert it to an http address
	httpgw := "http://" + gw

	functions, err := fr.Lister(httpgw)
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

func createFunction(fn FissionFunction) *v1.Function {

	headersTemplate := map[string]string{":method": "POST"}

	return &v1.Function{
		Name: fn.Name,
		Spec: rest.EncodeFunctionSpec(rest.Template{
			Path:            UrlForFunction(fn.Name),
			Header:          headersTemplate,
			PassthroughBody: true,
		}),
	}
}

func listGatewayFunctions(httpget func(string) (io.ReadCloser, error)) func(gw string) (FissionFunctions, error) {

	return func(gw string) (FissionFunctions, error) {
		u, err := url.Parse(gw)
		if err != nil {
			return nil, err
		}
		u.Path = path.Join(u.Path, "v2", "functions")
		s := u.String()
		body, err := httpget(s)

		if err != nil {
			return nil, err
		}
		defer body.Close()
		var funcs FissionFunctions
		err = json.NewDecoder(body).Decode(&funcs)

		if err != nil {
			return nil, err
		}

		return funcs, nil
	}
}

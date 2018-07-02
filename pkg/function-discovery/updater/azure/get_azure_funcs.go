package azure

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"strings"

	"github.com/pkg/errors"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	azureplugin "github.com/solo-io/gloo/pkg/plugins/azure"
	"github.com/solo-io/gloo/pkg/secretwatcher"
	"github.com/solo-io/gloo/pkg/storage/dependencies"
)

// TODO: support both active direcctory and public file directly

const (
	authLevelAdmin     = "admin"
	authLevelFunction  = "function"
	authLevelAnonymous = "anonymous"

	masterKeyName = "_master"
)

func secretRefName(us *v1.Upstream, functionAppName string) string {
	return fmt.Sprintf("%s-%s-azure-keys", us.Name, functionAppName)
}

func functionKeyName(azFn azureFunc) string {
	return azFn.Name
}

func GetFuncsAndSecret(us *v1.Upstream, secrets secretwatcher.SecretMap) ([]*v1.Function, *dependencies.Secret, error) {
	username, password, err := getUserCredentials(us, secrets)
	if err != nil {
		return nil, nil, errors.Wrap(err, "retrieving publish profile credentials from secrets")
	}
	azureSpec, err := azureplugin.DecodeUpstreamSpec(us.Spec)
	if err != nil {
		return nil, nil, errors.Wrap(err, "decoding azure upstream spec")
	}
	functionAppName := azureSpec.FunctionAppName

	azureFuncs, err := listAzureFuncs(functionAppName, username, password)
	if err != nil {
		return nil, nil, errors.Wrap(err, "listing azure funcs")
	}
	jwt, err := getAzureJWT(functionAppName, username, password)
	if err != nil {
		return nil, nil, errors.Wrap(err, "getting azure jwt")
	}
	masterKey, err := getMasterKey(functionAppName, username, password)
	if err != nil {
		return nil, nil, errors.Wrap(err, "getting master key")
	}

	functionKeys, err := getFunctionKeys(functionAppName, azureFuncs, jwt)
	if err != nil {
		return nil, nil, errors.Wrap(err, "getting function keys")
	}

	functionKeys[masterKeyName] = masterKey

	ref := secretRefName(us, functionAppName)

	secret := &dependencies.Secret{
		Ref:  ref,
		Data: functionKeys,
	}

	glooFuncs := convertFunctions(azureFuncs)

	return glooFuncs, secret, nil
}

func listAzureFuncs(functionAppName, username, password string) ([]azureFunc, error) {
	u := fmt.Sprintf("https://%s.scm.azurewebsites.net/api/functions", functionAppName)
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, errors.Wrap(err, "creating http request")
	}
	req.SetBasicAuth(username, password)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "making HTTP GET")
	}
	defer res.Body.Close()
	b, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, errors.Wrap(err, "reading response body")
	}
	if res.StatusCode != 200 {
		return nil, errors.Errorf("request failed with status %d %s: %s", res.StatusCode, res.Status, b)
	}
	var azureFuncs []azureFunc
	if err := json.Unmarshal(b, &azureFuncs); err != nil {
		return nil, errors.Wrapf(err, "parsing response as azure funcs json: %v", err.Error())
	}
	return azureFuncs, nil
}

func getAzureJWT(functionAppName, username, password string) (string, error) {
	u := fmt.Sprintf("https://%s.scm.azurewebsites.net/api/functions/admin/token", functionAppName)
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return "", errors.Wrap(err, "creating http request")
	}
	req.SetBasicAuth(username, password)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", errors.Wrap(err, "making HTTP GET")
	}
	defer res.Body.Close()
	b, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return "", errors.Wrap(err, "reading response body")
	}
	if res.StatusCode != 200 {
		return "", errors.Errorf("request failed with status %d %s: %s", res.StatusCode, res.Status, b)
	}
	return strings.Replace(string(b), "\"", "", -1), nil
}

func getMasterKey(functionAppName, username, password string) (string, error) {
	u := fmt.Sprintf("https://%s.scm.azurewebsites.net/api/functions/admin/masterkey", functionAppName)
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return "", errors.Wrap(err, "creating http request")
	}
	req.SetBasicAuth(username, password)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", errors.Wrap(err, "making HTTP GET")
	}
	defer res.Body.Close()
	b, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return "", errors.Wrap(err, "reading response body")
	}
	if res.StatusCode != 200 {
		return "", errors.Errorf("request failed with status %d %s: %s", res.StatusCode, res.Status, b)
	}
	var masterKeyObj masterKey
	if err := json.Unmarshal(b, &masterKeyObj); err != nil {
		return "", errors.Wrapf(err, "parsing response as master key json: %v", err.Error())
	}
	return masterKeyObj.MasterKey, nil
}

func getFunctionKeys(functionAppName string, azureFuncs []azureFunc, jwt string) (map[string]string, error) {
	functionKeys := make(map[string]string)
	for _, azFn := range azureFuncs {
		switch getAuthLevel(azFn) {
		case authLevelAdmin:
			continue
		case authLevelAnonymous:
			continue
		case authLevelFunction:
			key, err := getFunctionKey(functionAppName, azFn, jwt)
			if err != nil {
				return nil, errors.Wrapf(err, "getting function key for %s", azFn.Name)
			}
			functionKeys[functionKeyName(azFn)] = key
		default:
			return nil, errors.Errorf("unexpected bindings configuration for function %s", azFn.Name)
		}
	}
	return functionKeys, nil
}

func getFunctionKey(functionAppName string, azFn azureFunc, jwt string) (string, error) {
	u := fmt.Sprintf("https://%s.azurewebsites.net/admin/functions/%s/keys", functionAppName, azFn.Name)
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return "", errors.Wrap(err, "creating http request")
	}
	req.Header.Set("Authorization", "Bearer "+jwt)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", errors.Wrap(err, "making HTTP GET")
	}
	defer res.Body.Close()
	b, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return "", errors.Wrap(err, "reading response body")
	}
	if res.StatusCode != 200 {
		return "", errors.Errorf("request failed with status %d %s: %s", res.StatusCode, res.Status, b)
	}
	var functionKeyObj functionKey
	if err := json.Unmarshal(b, &functionKeyObj); err != nil {
		return "", errors.Wrapf(err, "parsing response as function key json: %v", err.Error())
	}
	if len(functionKeyObj.Keys) < 1 {
		return "", errors.Errorf("no function keys found for azure function %s", azFn.Name)
	}
	return functionKeyObj.Keys[0].Value, nil
}

func convertFunctions(azureFunctions []azureFunc) []*v1.Function {
	var funcs []*v1.Function
	for _, azFn := range azureFunctions {
		fn := &v1.Function{
			Name: azFn.Name,
			Spec: azureplugin.EncodeFunctionSpec(azureplugin.FunctionSpec{
				FunctionName: azFn.Name,
				AuthLevel:    getAuthLevel(azFn),
			}),
		}
		funcs = append(funcs, fn)
	}
	return funcs
}

func getAuthLevel(azFn azureFunc) string {
	for _, binding := range azFn.Config.Bindings {
		if binding.Direction != "in" {
			continue
		}
		return binding.AuthLevel
	}
	return "no 'in' binding found"
}

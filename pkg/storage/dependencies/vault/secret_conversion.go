package vault

import (
	vaultapi "github.com/hashicorp/vault/api"
	"github.com/pkg/errors"
	"github.com/solo-io/gloo/pkg/storage/dependencies"
)

func vaultSecretToSecret(ref string, vaultSecret *vaultapi.Secret) (*dependencies.Secret, error) {
	data := make(map[string]string)
	for k, v := range vaultSecret.Data {
		strValue, ok := v.(string)
		if !ok {
			return nil, errors.New("secret data must be encoded as string:string pairs")
		}
		data[k] = strValue
	}
	return &dependencies.Secret{
		Ref:  ref,
		Data: data,
	}, nil
}

func toInterfaceMap(m map[string]string) map[string]interface{} {
	interfaceMap := make(map[string]interface{})
	for k, v := range m {
		interfaceMap[k] = v
	}
	return interfaceMap
}

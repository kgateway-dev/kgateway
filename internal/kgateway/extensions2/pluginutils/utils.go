package pluginutils

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

const (
	// TODO: support capitalized or lower case key names
	AuthKey = "Authorization"
)

func GetAuthToken(in v1alpha1.SingleAuthToken, aiSecrets *ir.Secret) (token string, err error) {
	switch in.Kind {
	case v1alpha1.Inline:
		if in.Inline == nil {
			return "", errors.New("inline auth token must be set if kind is type Inline")
		}
		token = *in.Inline
	case v1alpha1.SecretRef:
		if aiSecrets == nil {
			return "", fmt.Errorf("secret not found for %s", in.SecretRef.Name)
		}
		secret, err := deriveHeaderSecret(aiSecrets)
		if err != nil {
			return "", err
		}
		token = getTokenFromHeaderSecret(secret)
	}
	return token, err
}

type headerSecretDerivation struct {
	authorization string
}

// deriveHeaderSecret from ingest if we are using a kubernetes secretref
// Named returns with the derived string contents or an error due to retrieval or format.
func deriveHeaderSecret(aiSecrets *ir.Secret) (headerSecretDerivation, error) {
	var errs []error
	derived := headerSecretDerivation{
		authorization: string(aiSecrets.Data[AuthKey]),
	}
	if derived.authorization == "" || !utf8.Valid([]byte(derived.authorization)) {
		// err is nil here but this is still safe
		errs = append(errs, errors.New("access_key is not a valid string"))
	}
	return derived, errors.Join(errs...)
}

// `getTokenFromHeaderSecret` retrieves the auth token from the secret reference.
// Currently, this function will return an error if there are more than one header in the secret
// as we do not know which one to select.
// In addition, this function will strip the "Bearer " prefix from the token as it will get conditionally
// added later depending on the provider.
func getTokenFromHeaderSecret(secret headerSecretDerivation) string {
	return strings.TrimPrefix(secret.authorization, "Bearer ")
}
